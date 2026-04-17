package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"one-codingplan/internal/models"
	"one-codingplan/internal/pool"
	"one-codingplan/internal/translator"
)

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

// handleAnthropicRelay handles POST /v1/messages: parses Anthropic-format requests,
// translates to OpenAI format, forwards to upstream with failover, and translates back.
func (s *Server) handleAnthropicRelay(c *gin.Context) {
	// Read body with 10MB limit (T-4-08)
	bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, anthropicError("api_error", "failed to read request body"))
		return
	}
	if len(bodyBytes) > 10*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, anthropicError("invalid_request_error", "request body too large"))
		return
	}

	// Parse Anthropic request (T-4-07)
	var req translator.AnthropicRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "invalid JSON: "+err.Error()))
		return
	}

	originalModel := req.Model
	keyID := c.GetString("keyID")
	accessKey := c.MustGet("accessKey").(models.AccessKey)
	allowedUpstreams := parseAllowedUpstreams(accessKey.AllowedUpstreams)
	start := time.Now()

	seen := make(map[uint]bool)
	var current *pool.UpstreamEntry

	for {
		up, err := s.pool.Select(allowedUpstreams)
		if errors.Is(err, pool.ErrNoUpstreams) {
			break
		}
		if err != nil {
			break
		}
		if seen[up.ID] {
			break
		}
		seen[up.ID] = true
		current = up

		var resp *http.Response
		var cancel context.CancelFunc

		if current.Format == "anthropic" {
			// Direct passthrough — forward raw Anthropic body to /v1/messages
			ctx, ctxCancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
			cancel = ctxCancel
			outReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
				current.BaseURL+"/v1/messages",
				bytes.NewReader(bodyBytes))
			if err != nil {
				cancel()
				continue
			}
			outReq.Header = cloneHeaders(c.Request.Header)
			outReq.Header.Set("Authorization", "Bearer "+current.APIKey)
			outReq.Header.Set("Content-Type", "application/json")
			outReq.Header.Del("Host")
			var reqErr error
			resp, reqErr = relayClient.Do(outReq)
			if reqErr != nil {
				cancel()
				log.Printf("[upstream] %s network error: %v", current.Name, reqErr)
				continue
			}
		} else {
			// Translate Anthropic request to OpenAI format (D-01/D-02)
			oaiReq, err := translator.AnthropicToOpenAI(&req, current.ModelOverride)
			if err != nil {
				// Translation failure is a client error
				c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "request translation failed: "+err.Error()))
				return
			}

			translatedBody, err := json.Marshal(oaiReq)
			if err != nil {
				continue
			}

			ctx, ctxCancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
			cancel = ctxCancel
			outReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
				current.BaseURL+"/v1/chat/completions",
				bytes.NewReader(translatedBody))
			if err != nil {
				cancel()
				continue
			}
			outReq.Header = cloneHeaders(c.Request.Header)
			outReq.Header.Set("Authorization", "Bearer "+current.APIKey)
			outReq.Header.Set("Content-Type", "application/json")
			outReq.Header.Del("Host")
			var reqErr error
			resp, reqErr = relayClient.Do(outReq)
			if reqErr != nil {
				cancel()
				log.Printf("[upstream] %s network error: %v", current.Name, reqErr)
				continue
			}
		}

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			cancel()
			log.Printf("[upstream] %s status %d: %s", current.Name, resp.StatusCode, respBody)
			class := pool.Classify(current.Name, resp.StatusCode, respBody)
			switch class {
			case pool.ClassCreditsExhausted:
				s.pool.Mark(current.ID, false)
				continue
			case pool.ClassRateLimited:
				delete(seen, current.ID) // allow retrying after backoff
				time.Sleep(s.pool.Backoff())
				continue
			case pool.ClassModelNotSupported:
				s.pool.Mark(current.ID, false)
				continue
			default:
				continue
			}
		}

		// Success path
		if req.Stream {
			if current.Format == "anthropic" {
				s.proxyStream(c, resp, cancel, keyID, current.ID, start)
			} else {
				s.proxyAnthropicStream(c, resp, cancel, keyID, current.ID, start, originalModel)
			}
		} else {
			if current.Format == "anthropic" {
				s.proxyBuffer(c, resp, cancel, keyID, current.ID, start)
			} else {
				s.proxyAnthropicBuffer(c, resp, cancel, keyID, current.ID, start, originalModel)
			}
		}
		return
	}

	// All upstreams exhausted
	c.JSON(http.StatusServiceUnavailable, anthropicErrNoUpstream)
	s.logUsage(keyID, 0, false, 0, 0, time.Since(start))
}

// proxyAnthropicBuffer reads the upstream OpenAI-format response, translates it to
// Anthropic format, and writes the result to the client.
func (s *Server) proxyAnthropicBuffer(c *gin.Context, resp *http.Response, cancel context.CancelFunc, keyID string, upstreamID uint, start time.Time, originalModel string) {
	defer cancel()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		c.JSON(http.StatusBadGateway, anthropicError("api_error", "failed to read upstream response"))
		s.logUsage(keyID, upstreamID, false, 0, 0, time.Since(start))
		return
	}

	var oaiResp translator.OpenAIResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		log.Printf("[upstream] id=%d parse error: %v — body: %.200s", upstreamID, err, body)
		c.JSON(http.StatusBadGateway, anthropicError("api_error", "failed to parse upstream response"))
		s.logUsage(keyID, upstreamID, false, 0, 0, time.Since(start))
		return
	}

	anthropicResp, err := translator.OpenAIToAnthropic(&oaiResp, originalModel)
	if err != nil {
		log.Printf("[upstream] id=%d translation error: %v", upstreamID, err)
		c.JSON(http.StatusBadGateway, anthropicError("api_error", "response translation failed: "+err.Error()))
		s.logUsage(keyID, upstreamID, false, 0, 0, time.Since(start))
		return
	}

	c.JSON(http.StatusOK, anthropicResp)
	s.logUsage(keyID, upstreamID, true,
		anthropicResp.Usage.InputTokens, anthropicResp.Usage.OutputTokens,
		time.Since(start))
}

// proxyAnthropicStream copies an upstream OpenAI SSE stream to the client as Anthropic SSE events.
func (s *Server) proxyAnthropicStream(c *gin.Context, resp *http.Response, cancel context.CancelFunc, keyID string, upstreamID uint, start time.Time, originalModel string) {
	defer cancel()
	defer resp.Body.Close()

	// Set streaming headers (D-09, T-4-11)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		s.logUsage(keyID, upstreamID, false, 0, 0, time.Since(start))
		return
	}

	// mu serializes writes from heartbeat goroutine and main read loop (T-4-11)
	var mu sync.Mutex
	writeAndFlush := func(p []byte) {
		mu.Lock()
		c.Writer.Write(p) //nolint:errcheck
		flusher.Flush()
		mu.Unlock()
	}

	// Heartbeat goroutine
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

	st := translator.NewStreamTranslator(originalModel)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			events, translateErr := st.Translate(buf[:n])
			if translateErr == nil {
				for _, event := range events {
					writeAndFlush(event)
				}
			}
		}
		if err != nil {
			break
		}
	}

	s.logUsage(keyID, upstreamID, true, 0, 0, time.Since(start))
}
