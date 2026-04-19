package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
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

// handleAnthropicRelay handles POST /v1/messages: forwards raw Anthropic body to upstream /v1/messages.
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

	// Parse Anthropic request to detect stream flag (T-4-07)
	var req translator.AnthropicRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, anthropicError("invalid_request_error", "invalid JSON: "+err.Error()))
		return
	}

	keyID := c.GetString("keyID")
	accessKey := c.MustGet("accessKey").(models.AccessKey)
	allowedUpstreams := parseAllowedUpstreams(accessKey.AllowedUpstreams)
	start := time.Now()

	seen := make(map[uint]bool)

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

		sendBody := bodyBytes
		if up.ModelOverride != "" {
			if rewritten, err := rewriteModel(bodyBytes, up.ModelOverride); err == nil {
				sendBody = rewritten
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		outReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			pool.GetAdapter(up.Name).AnthropicURL(up.BaseURL),
			bytes.NewReader(sendBody))
		if err != nil {
			cancel()
			continue
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
			continue
		}

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			cancel()
			slog.Warn("upstream error response", "name", up.Name, "status", resp.StatusCode, "url", outReq.URL.String(), "body", string(respBody))
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

		// Success path — passthrough
		slog.Info("upstream ok", "name", up.Name, "stream", req.Stream, "url", outReq.URL.String())
		if req.Stream {
			s.proxyStream(c, resp, cancel, keyID, up.ID, up.Name, start)
		} else {
			s.proxyBuffer(c, resp, cancel, keyID, up.ID, up.Name, start)
		}
		return
	}

	// All upstreams exhausted
	c.JSON(http.StatusServiceUnavailable, anthropicErrNoUpstream)
	s.logUsage(keyID, 0, "", false, 0, 0, time.Since(start))
}
