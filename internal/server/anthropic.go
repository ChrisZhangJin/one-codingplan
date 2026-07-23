package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"one-codingplan/internal/models"
	"one-codingplan/internal/pool"
	"one-codingplan/internal/translator"
)

// anthropicExtensionFields are fields in the Anthropic Messages API that are
// Claude-specific and not understood by third-party Anthropic-compatible upstreams.
// Forwarding them causes undefined behavior (e.g. Kimi never terminates its SSE stream
// when it receives "thinking").
var anthropicExtensionFields = []string{"thinking", "betas"}

// stripAnthropicExtensions removes Claude-specific fields from a raw Anthropic
// request body before forwarding to third-party upstreams.
func stripAnthropicExtensions(body []byte) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	for _, field := range anthropicExtensionFields {
		delete(m, field)
	}
	return json.Marshal(m)
}

var anthropicErrNoUpstream = gin.H{
	"type": "error",
	"error": gin.H{
		"type":    "overloaded_error",
		"message": "no upstream available",
	},
}

func anthropicError(errType, message string) gin.H {
	return gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	}
}

// anthropicAttemptResult signals what handleAnthropicRelay should do after a single attempt.
type anthropicAttemptResult int

const (
	anthropicAttemptDone   anthropicAttemptResult = iota // response delivered to client; return
	anthropicAttemptRotate                               // try next upstream
)

// handleAnthropicRelay handles POST /v1/messages.
// First pass: route to Anthropic-capable upstreams (passthrough).
// Second pass: route to OpenAI-only upstreams, translating Anthropic <-> OpenAI Chat.
func (s *Server) handleAnthropicRelay(c *gin.Context) {
	bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, anthropicError("api_error", "failed to read request body"))
		return
	}
	if len(bodyBytes) > 10*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, anthropicError("invalid_request_error", "request body too large"))
		return
	}

	var req translator.AnthropicRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "invalid JSON: "+err.Error()))
		return
	}

	keyID := c.GetString("keyID")
	accessKey := c.MustGet("accessKey").(models.AccessKey)
	allowedUpstreams := parseAllowedUpstreams(accessKey.AllowedUpstreams)
	start := time.Now()

	sanitizedBody, err := stripAnthropicExtensions(bodyBytes)
	if err != nil {
		sanitizedBody = bodyBytes
	}

	// Two-pass routing: try native Anthropic upstreams first, then fall back to
	// OpenAI upstreams with translation. Each pass has its own seen-set so
	// "both"-protocol upstreams are only attempted in the native pass.
	for _, wantProto := range []string{models.ProtocolAnthropic, models.ProtocolOpenAI} {
		seen := make(map[uint]bool)
		for {
			up, selErr := s.pool.Select(allowedUpstreams, wantProto)
			if errors.Is(selErr, pool.ErrNoUpstreams) {
				break
			}
			if selErr != nil {
				break
			}
			if seen[up.ID] {
				break
			}
			seen[up.ID] = true

			// In the OpenAI pass, skip "both"-protocol upstreams — they were already
			// attempted via native Anthropic in the first pass.
			if wantProto == models.ProtocolOpenAI && up.Protocol != models.ProtocolOpenAI {
				continue
			}

			var result anthropicAttemptResult
			if up.Protocol == models.ProtocolOpenAI {
				result = s.relayAnthropicTranslated(c, &req, up, keyID, start, seen)
			} else {
				result = s.relayAnthropicPassthrough(c, bodyBytes, sanitizedBody, &req, up, keyID, start, seen)
			}
			if result == anthropicAttemptDone {
				return
			}
		}
	}

	// All upstreams exhausted
	c.JSON(http.StatusServiceUnavailable, anthropicErrNoUpstream)
	s.logUsage(keyID, 0, "", false, 0, 0, time.Since(start))
}

// relayAnthropicPassthrough forwards the request to an Anthropic-capable upstream
// (raw passthrough). Returns anthropicAttemptDone when the response has been written.
func (s *Server) relayAnthropicPassthrough(c *gin.Context, originalBody, sanitizedBody []byte, req *translator.AnthropicRequest, up *pool.UpstreamEntry, keyID string, start time.Time, seen map[uint]bool) anthropicAttemptResult {
	sendBody := sanitizedBody
	if up.PassthroughExtensions {
		sendBody = originalBody
	}
	if up.ModelOverride != "" {
		if rewritten, err := rewriteModel(sendBody, up.ModelOverride); err == nil {
			sendBody = rewritten
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
	outURL := pool.GetAdapter(up.Name).AnthropicURL(up.BaseURL)
	outReq, err := http.NewRequestWithContext(ctx, http.MethodPost, outURL, bytes.NewReader(sendBody))
	if err != nil {
		cancel()
		slog.Warn("anthropic upstream request build failed", "name", up.Name, "url", outURL, "err", err)
		return anthropicAttemptRotate
	}
	outReq.Header = cloneHeaders(c.Request.Header)
	outReq.Header.Set("Authorization", "Bearer "+up.APIKey)
	outReq.Header.Set("x-api-key", up.APIKey)
	outReq.Header.Set("Content-Type", "application/json")
	outReq.Header.Del("Host")
	pool.GetAdapter(up.Name).InjectHeaders(outReq.Header)
	slog.Debug("upstream request", "name", up.Name, "url", outReq.URL.String(),
		"model_override", up.ModelOverride, "key_prefix", up.APIKey[:min(8, len(up.APIKey))],
		"body", string(sendBody))

	resp, reqErr := relayClient.Do(outReq)
	if reqErr != nil {
		cancel()
		slog.Warn("upstream network error", "name", up.Name, "err", reqErr)
		return anthropicAttemptRotate
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		cancel()
		slog.Warn("upstream error response", "name", up.Name, "status", resp.StatusCode, "url", outReq.URL.String(), "body", string(respBody))
		s.handleUpstreamError(up, resp.StatusCode, respBody, seen)
		return anthropicAttemptRotate
	}

	slog.Info("upstream ok", "name", up.Name, "stream", req.Stream, "url", outReq.URL.String())
	if req.Stream {
		s.proxyStream(c, resp, cancel, keyID, up.ID, up.Name, start)
	} else {
		s.proxyBuffer(c, resp, cancel, keyID, up.ID, up.Name, start)
	}
	return anthropicAttemptDone
}

// relayAnthropicTranslated forwards an Anthropic request to an OpenAI-only upstream
// by translating to/from the OpenAI Chat Completions API.
func (s *Server) relayAnthropicTranslated(c *gin.Context, req *translator.AnthropicRequest, up *pool.UpstreamEntry, keyID string, start time.Time, seen map[uint]bool) anthropicAttemptResult {
	openAIReq, err := translator.AnthropicToOpenAI(req, up.ModelOverride)
	if err != nil {
		slog.Warn("anthropic->openai translate failed", "name", up.Name, "err", err)
		return anthropicAttemptRotate
	}
	if openAIReq.Model == "" {
		openAIReq.Model = req.Model
	}
	sendBody, err := json.Marshal(openAIReq)
	if err != nil {
		return anthropicAttemptRotate
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
	outURL := pool.GetAdapter(up.Name).OpenAIURL(up.BaseURL)
	outReq, err := http.NewRequestWithContext(ctx, http.MethodPost, outURL, bytes.NewReader(sendBody))
	if err != nil {
		cancel()
		slog.Warn("anthropic->openai upstream request build failed", "name", up.Name, "url", outURL, "err", err)
		return anthropicAttemptRotate
	}
	outReq.Header = cloneHeaders(c.Request.Header)
	outReq.Header.Set("Authorization", "Bearer "+up.APIKey)
	outReq.Header.Set("Content-Type", "application/json")
	// Strip Anthropic-only headers; the OpenAI upstream doesn't understand them.
	outReq.Header.Del("anthropic-version")
	outReq.Header.Del("anthropic-beta")
	outReq.Header.Del("x-api-key")
	outReq.Header.Del("Host")
	pool.GetAdapter(up.Name).InjectHeaders(outReq.Header)
	slog.Debug("upstream request (translated)", "name", up.Name, "url", outReq.URL.String(),
		"model", openAIReq.Model, "stream", openAIReq.Stream, "body", string(sendBody))

	resp, reqErr := relayClient.Do(outReq)
	if reqErr != nil {
		cancel()
		slog.Warn("upstream network error (translated)", "name", up.Name, "err", reqErr)
		return anthropicAttemptRotate
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		cancel()
		slog.Warn("upstream error response (translated)", "name", up.Name, "status", resp.StatusCode, "url", outReq.URL.String(), "body", string(respBody))
		s.handleUpstreamError(up, resp.StatusCode, respBody, seen)
		return anthropicAttemptRotate
	}

	slog.Info("upstream ok (translated)", "name", up.Name, "stream", req.Stream, "url", outReq.URL.String())
	if req.Stream {
		s.proxyAnthropicTranslatedStream(c, resp, cancel, keyID, up.ID, up.Name, start, req.Model)
	} else {
		s.proxyAnthropicTranslatedBuffer(c, resp, cancel, keyID, up.ID, up.Name, start, req.Model)
	}
	return anthropicAttemptDone
}

// handleUpstreamError applies the classifier-driven side effects (mark unavailable,
// backoff for retry, etc.). The caller is responsible for rotating to the next upstream.
func (s *Server) handleUpstreamError(up *pool.UpstreamEntry, status int, body []byte, seen map[uint]bool) {
	switch pool.Classify(up.Name, status, body) {
	case pool.ClassCreditsExhausted:
		s.pool.Mark(up.ID, false)
	case pool.ClassRateLimited:
		delete(seen, up.ID)
		time.Sleep(s.pool.Backoff())
	case pool.ClassModelNotSupported:
		s.pool.Mark(up.ID, false)
	}
}

// proxyAnthropicTranslatedBuffer reads a non-streaming OpenAI Chat response,
// translates it to Anthropic format, and writes the JSON to the client.
func (s *Server) proxyAnthropicTranslatedBuffer(c *gin.Context, resp *http.Response, cancel context.CancelFunc, keyID string, upstreamID uint, upstreamName string, start time.Time, requestModel string) {
	defer cancel()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		c.JSON(http.StatusBadGateway, anthropicError("api_error", "failed to read upstream response"))
		s.logUsage(keyID, upstreamID, upstreamName, false, 0, 0, time.Since(start))
		return
	}

	var openAIResp translator.OpenAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		c.JSON(http.StatusBadGateway, anthropicError("api_error", "failed to parse upstream response"))
		s.logUsage(keyID, upstreamID, upstreamName, false, 0, 0, time.Since(start))
		return
	}

	anthResp, err := translator.OpenAIToAnthropic(&openAIResp, requestModel)
	if err != nil {
		c.JSON(http.StatusBadGateway, anthropicError("api_error", "translation failed: "+err.Error()))
		s.logUsage(keyID, upstreamID, upstreamName, false, 0, 0, time.Since(start))
		return
	}
	c.JSON(http.StatusOK, anthResp)
	s.logUsage(keyID, upstreamID, upstreamName, true, openAIResp.Usage.PromptTokens, openAIResp.Usage.CompletionTokens, time.Since(start))
}

// proxyAnthropicTranslatedStream streams an OpenAI SSE response, translating
// each chunk to Anthropic-format SSE events via translator.StreamTranslator.
func (s *Server) proxyAnthropicTranslatedStream(c *gin.Context, resp *http.Response, cancel context.CancelFunc, keyID string, upstreamID uint, upstreamName string, start time.Time, requestModel string) {
	defer cancel()
	defer resp.Body.Close()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		s.logUsage(keyID, upstreamID, upstreamName, false, 0, 0, time.Since(start))
		return
	}

	var mu sync.Mutex
	writeAndFlush := func(p []byte) {
		mu.Lock()
		c.Writer.Write(p) //nolint:errcheck
		flusher.Flush()
		mu.Unlock()
	}

	hbInterval := HeartbeatInterval
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(hbInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				writeAndFlush([]byte(": heartbeat\n\n"))
			case <-done:
				return
			}
		}
	}()

	tr := translator.NewStreamTranslator(requestModel)
	buf := make([]byte, 4096)
	var inTokens, outTokens int

	// usagePeek extracts token counts from the trailing chunk's usage field.
	// OpenAI puts usage at the top level of the final chunk (with stream_options.include_usage),
	// DeepSeek emits it that way by default.
	type usagePeek struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			raw := buf[:n]
			for _, line := range bytes.Split(raw, []byte("\n")) {
				line = bytes.TrimSpace(line)
				if !bytes.HasPrefix(line, []byte("data:")) {
					continue
				}
				line = bytes.TrimSpace(line[5:])
				if bytes.Equal(line, []byte("[DONE]")) {
					continue
				}
				var u usagePeek
				if json.Unmarshal(line, &u) == nil {
					if u.Usage.PromptTokens > 0 || u.Usage.CompletionTokens > 0 {
						inTokens = u.Usage.PromptTokens
						outTokens = u.Usage.CompletionTokens
					}
				}
			}
			events, translateErr := tr.Translate(raw)
			if translateErr != nil {
				slog.Debug("anthropic stream translate error", "err", translateErr)
			} else {
				for _, ev := range events {
					writeAndFlush(ev)
				}
			}
		}
		if err != nil {
			break
		}
	}

	s.logUsage(keyID, upstreamID, upstreamName, true, inTokens, outTokens, time.Since(start))
}
