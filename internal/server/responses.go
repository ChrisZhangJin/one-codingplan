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

	"one-codingplan/internal/logging"
	"one-codingplan/internal/models"
	"one-codingplan/internal/pool"
	"one-codingplan/internal/translator"
)

var responsesErrNoUpstream = gin.H{
	"error": gin.H{
		"code":    "server_error",
		"message": "no upstream available",
	},
}

func responsesError(code, message string) gin.H {
	return gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	}
}

// handleResponsesRelay handles POST /v1/responses: translates Responses API requests to
// OpenAI chat completions, runs the failover loop, and returns Responses API format.
func (s *Server) handleResponsesRelay(c *gin.Context) {
	bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, responsesError("server_error", "failed to read request body"))
		return
	}
	if len(bodyBytes) > 10*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, responsesError("invalid_request_error", "request body too large"))
		return
	}

	var req translator.ResponsesRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, responsesError("invalid_request_error", "invalid JSON: "+err.Error()))
		return
	}

	msgs, err := translator.ParseResponsesInput(req.Input)
	if err != nil {
		if errors.Is(err, translator.ErrStringInput) {
			c.JSON(http.StatusBadRequest, responsesError("invalid_request_error", "input must be an array"))
			return
		}
		c.JSON(http.StatusBadRequest, responsesError("invalid_request_error", "invalid input: "+err.Error()))
		return
	}

	keyID := c.GetString("keyID")
	accessKey := c.MustGet("accessKey").(models.AccessKey)
	allowedUpstreams := parseAllowedUpstreams(accessKey.AllowedUpstreams)
	start := time.Now()

	slog.Debug("responses handler invoked", "model", req.Model, "stream", req.Stream, "key_id", keyID)

	seen := make(map[uint]bool)

	for {
		up, err := s.pool.Select(allowedUpstreams)
		if errors.Is(err, pool.ErrNoUpstreams) {
			slog.Debug("responses no upstream available", "seen", len(seen))
			break
		}
		if err != nil {
			slog.Debug("responses pool select error", "err", err, "seen", len(seen))
			break
		}
		if seen[up.ID] {
			slog.Debug("responses all upstreams exhausted", "upstream", up.Name, "seen", len(seen))
			break
		}
		seen[up.ID] = true
		slog.Debug("responses upstream selected", "name", up.Name, "id", up.ID)

		modelOverride := ""
		if up.ModelOverride != "" {
			modelOverride = up.ModelOverride
		}
		openAIReq := translator.ResponsesRequestToOpenAI(&req, msgs, modelOverride)
		sendBody, err := json.Marshal(openAIReq)
		if err != nil {
			continue
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
		outReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			pool.GetAdapter(up.Name).OpenAIURL(up.BaseURL),
			bytes.NewReader(sendBody))
		if err != nil {
			cancel()
			continue
		}
		outReq.Header = cloneHeaders(c.Request.Header)
		outReq.Header.Set("Authorization", "Bearer "+up.APIKey)
		outReq.Header.Set("Content-Type", "application/json")
		outReq.Header.Del("Host")
		pool.GetAdapter(up.Name).InjectHeaders(outReq.Header)
		slog.Debug("responses upstream request", "name", up.Name, "url", outReq.URL.String(),
			"model_override", up.ModelOverride)
		slog.Log(nil, logging.LevelVerbose, "responses upstream body", "body", string(sendBody))

		resp, reqErr := relayClient.Do(outReq)
		if reqErr != nil {
			cancel()
			slog.Warn("responses upstream network error", "name", up.Name, "err", reqErr)
			continue
		}

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			cancel()
			slog.Warn("responses upstream error", "name", up.Name, "status", resp.StatusCode, "body", string(respBody))
			class := pool.Classify(up.Name, resp.StatusCode, respBody)
			switch class {
			case pool.ClassCreditsExhausted:
				s.pool.Mark(up.ID, false)
				continue
			case pool.ClassRateLimited:
				delete(seen, up.ID)
				time.Sleep(s.pool.Backoff())
				continue
			case pool.ClassModelNotSupported:
				s.pool.Mark(up.ID, false)
				continue
			default:
				continue
			}
		}

		slog.Info("responses upstream ok", "name", up.Name, "stream", req.Stream, "url", outReq.URL.String())
		if req.Stream {
			s.proxyResponsesStream(c, resp, cancel, keyID, up.ID, up.Name, start, req.Model)
		} else {
			s.proxyResponsesBuffer(c, resp, cancel, keyID, up.ID, up.Name, start, req.Model)
		}
		return
	}

	c.JSON(http.StatusServiceUnavailable, responsesErrNoUpstream)
	s.logUsage(keyID, 0, "", false, 0, 0, time.Since(start))
}

// proxyResponsesBuffer reads the full upstream response, translates it to Responses API format,
// and returns it as JSON.
func (s *Server) proxyResponsesBuffer(c *gin.Context, resp *http.Response, cancel context.CancelFunc, keyID string, upstreamID uint, upstreamName string, start time.Time, requestModel string) {
	defer cancel()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		c.JSON(http.StatusBadGateway, responsesError("server_error", "failed to read upstream response"))
		s.logUsage(keyID, upstreamID, upstreamName, false, 0, 0, time.Since(start))
		return
	}

	var openAIResp translator.OpenAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		c.JSON(http.StatusBadGateway, responsesError("server_error", "failed to parse upstream response"))
		s.logUsage(keyID, upstreamID, upstreamName, false, 0, 0, time.Since(start))
		return
	}

	responsesResp := translator.OpenAIToResponsesAPI(&openAIResp, requestModel)
	c.JSON(http.StatusOK, responsesResp)
	s.logUsage(keyID, upstreamID, upstreamName, true, openAIResp.Usage.PromptTokens, openAIResp.Usage.CompletionTokens, time.Since(start))
}

// proxyResponsesStream streams the upstream response, translating OpenAI SSE chunks to
// Responses API SSE events. Token counts are extracted from the final SSE chunk's usage field.
func (s *Server) proxyResponsesStream(c *gin.Context, resp *http.Response, cancel context.CancelFunc, keyID string, upstreamID uint, upstreamName string, start time.Time, model string) {
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

	tr := translator.NewResponsesStreamTranslator(model)
	buf := make([]byte, 4096)
	var inTokens, outTokens int

	// openAIStreamChunk mirrors the shape used by translator/stream.go
	type openAIStreamChunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
			Usage        struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		} `json:"choices"`
	}

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			// Try to extract token counts from this chunk's usage field.
			// The final chunk (before [DONE]) contains the complete usage.
			raw := bytes.TrimSpace(buf[:n])
			for _, line := range bytes.Split(raw, []byte("\n")) {
				line = bytes.TrimSpace(line)
				if !bytes.HasPrefix(line, []byte("data:")) {
					continue
				}
				line = bytes.TrimSpace(line[5:])
				if bytes.Equal(line, []byte("[DONE]")) {
					continue
				}
				var chunk openAIStreamChunk
				if json.Unmarshal(line, &chunk) == nil && len(chunk.Choices) > 0 {
					if u := chunk.Choices[0].Usage; u.PromptTokens > 0 || u.CompletionTokens > 0 {
						inTokens = u.PromptTokens
						outTokens = u.CompletionTokens
					}
				}
			}

			slog.Log(nil, logging.LevelVerbose, "responses stream chunk", "raw", string(buf[:n]))
			events, translateErr := tr.Translate(buf[:n])
			if translateErr != nil {
				slog.Debug("responses stream translate error", "err", translateErr)
			} else {
				for _, ev := range events {
					slog.Log(nil, logging.LevelVerbose, "responses stream event", "event", string(ev))
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
