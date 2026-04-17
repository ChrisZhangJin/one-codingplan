package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
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

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		outReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			up.BaseURL+"/v1/messages",
			bytes.NewReader(bodyBytes))
		if err != nil {
			cancel()
			continue
		}
		outReq.Header = cloneHeaders(c.Request.Header)
		outReq.Header.Set("Authorization", "Bearer "+up.APIKey)
		outReq.Header.Set("Content-Type", "application/json")
		outReq.Header.Del("Host")
		resp, reqErr := relayClient.Do(outReq)
		if reqErr != nil {
			cancel()
			log.Printf("[upstream] %s network error: %v", up.Name, reqErr)
			continue
		}

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			cancel()
			log.Printf("[upstream] %s status %d: %s", up.Name, resp.StatusCode, respBody)
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
		if req.Stream {
			s.proxyStream(c, resp, cancel, keyID, up.ID, start)
		} else {
			s.proxyBuffer(c, resp, cancel, keyID, up.ID, start)
		}
		return
	}

	// All upstreams exhausted
	c.JSON(http.StatusServiceUnavailable, anthropicErrNoUpstream)
	s.logUsage(keyID, 0, false, 0, 0, time.Since(start))
}
