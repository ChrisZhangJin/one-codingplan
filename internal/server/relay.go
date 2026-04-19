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
)

var relayClient = &http.Client{Timeout: 30 * time.Second}

// HeartbeatInterval controls how often a `: heartbeat` SSE comment is sent on idle streams.
// Exported so tests can override it.
var HeartbeatInterval = 30 * time.Second

var errNoUpstream = gin.H{
	"error": gin.H{
		"message": "no upstream available",
		"type":    "upstream_error",
		"code":    "no_upstream",
		"param":   nil,
	},
}

// hopByHopHeaders are headers that must not be forwarded to the upstream.
var hopByHopHeaders = []string{
	"Connection", "Keep-Alive", "Transfer-Encoding", "TE", "Trailer", "Upgrade",
	"Accept-Encoding", // let relayClient negotiate compression transparently
}

func cloneHeaders(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, vv := range src {
		dst[k] = append([]string(nil), vv...)
	}
	for _, h := range hopByHopHeaders {
		dst.Del(h)
	}
	return dst
}

func isHopByHop(header string) bool {
	for _, h := range hopByHopHeaders {
		if http.CanonicalHeaderKey(h) == http.CanonicalHeaderKey(header) {
			return true
		}
	}
	return false
}

// authMiddleware validates the bearer token against the access_keys table.
// Rejects with 401 for missing/unknown tokens, 403 for disabled or expired keys.
func (s *Server) authMiddleware(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	token, ok := cutPrefix(auth, "Bearer ")
	if !ok || token == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var key models.AccessKey
	if err := s.db.Where("token = ?", token).First(&key).Error; err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !key.Enabled {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "key disabled"})
		return
	}
	if key.ExpiresAt != nil && time.Now().UTC().After(key.ExpiresAt.UTC()) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "key expired"})
		return
	}
	c.Set("keyID", key.ID)
	c.Set("accessKey", key)
	c.Next()
}

// cutPrefix returns s without the provided leading prefix string and true.
// If s doesn't start with prefix, cutPrefix returns s, false.
func cutPrefix(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return s, false
}

// reqBody is used to detect stream mode from the request body.
type reqBody struct {
	Stream bool `json:"stream"`
}

// chatResponse is used to extract token counts from a non-streaming response.
// Supports both OpenAI format (prompt_tokens/completion_tokens) and
// Anthropic format (input_tokens/output_tokens).
type chatResponse struct {
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
	} `json:"usage"`
}

// rewriteModel replaces the "model" field in a JSON request body.
func rewriteModel(body []byte, model string) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	b, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	m["model"] = b
	return json.Marshal(m)
}

// handleRelay is the main relay handler: reads the body, authenticates (via middleware),
// and forwards the request to an upstream with failover.
func (s *Server) handleRelay(c *gin.Context) {
	// Read body with limit (T-3-03: 10MB cap)
	bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read request body"})
		return
	}
	if len(bodyBytes) > 10*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}

	var rb reqBody
	json.Unmarshal(bodyBytes, &rb) //nolint:errcheck -- malformed JSON treated as non-stream

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
			// Already tried this upstream — exhausted the pool
			break
		}
		seen[up.ID] = true
		current = up

		sendBody := bodyBytes
		if current.ModelOverride != "" {
			if rewritten, err := rewriteModel(bodyBytes, current.ModelOverride); err == nil {
				sendBody = rewritten
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		outReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			pool.GetAdapter(current.Name).OpenAIURL(current.BaseURL),
			bytes.NewReader(sendBody))
		if err != nil {
			cancel()
			continue
		}
		outReq.Header = cloneHeaders(c.Request.Header)
		outReq.Header.Set("Authorization", "Bearer "+current.APIKey)
		outReq.Header.Del("Host")
		pool.GetAdapter(current.Name).InjectHeaders(outReq.Header)

		resp, err := relayClient.Do(outReq)
		if err != nil {
			cancel()
			// Network/timeout error — transient, rotate
			continue
		}

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			cancel()
			class := pool.Classify(current.Name, resp.StatusCode, respBody)
			switch class {
			case pool.ClassCreditsExhausted:
				s.pool.Mark(current.ID, false)
				continue // rotate to next
			case pool.ClassRateLimited:
				delete(seen, current.ID) // allow retrying after backoff
				time.Sleep(s.pool.Backoff())
				continue // retry same upstream
			case pool.ClassModelNotSupported:
				s.pool.Mark(current.ID, false)
				continue // rotate to next
			default: // transient
				continue // rotate to next (do NOT mark unavailable)
			}
		}

		// Success path — cancel is deferred into the proxy functions so the
		// context remains live for the duration of body reads (streaming or buffered).
		slog.Info("upstream ok", "name", current.Name, "stream", rb.Stream, "url", outReq.URL.String())
		if rb.Stream {
			s.proxyStream(c, resp, cancel, keyID, current.ID, current.Name, start)
		} else {
			s.proxyBuffer(c, resp, cancel, keyID, current.ID, current.Name, start)
		}
		return
	}

	// All upstreams exhausted (D-06)
	c.JSON(http.StatusServiceUnavailable, errNoUpstream)
	s.logUsage(keyID, 0, "", false, 0, 0, time.Since(start))
}

// proxyBuffer reads the upstream response body, forwards headers and status,
// and writes the body to the client. Extracts token counts for usage logging (D-08, D-13).
func (s *Server) proxyBuffer(c *gin.Context, resp *http.Response, cancel context.CancelFunc, keyID string, upstreamID uint, upstreamName string, start time.Time) {
	defer cancel()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read upstream response"})
		s.logUsage(keyID, upstreamID, upstreamName, false, 0, 0, time.Since(start))
		return
	}

	// Forward upstream response headers, excluding hop-by-hop headers (T-3-07)
	for k, vv := range resp.Header {
		if isHopByHop(k) {
			continue
		}
		for _, v := range vv {
			c.Header(k, v)
		}
	}

	contentType := resp.Header.Get("Content-Type")
	c.Data(resp.StatusCode, contentType, body)

	// Extract token counts — support both OpenAI and Anthropic response formats.
	var cr chatResponse
	json.Unmarshal(body, &cr) //nolint:errcheck
	inTokens := cr.Usage.PromptTokens
	if inTokens == 0 {
		inTokens = cr.Usage.InputTokens
	}
	outTokens := cr.Usage.CompletionTokens
	if outTokens == 0 {
		outTokens = cr.Usage.OutputTokens
	}
	s.logUsage(keyID, upstreamID, upstreamName, true, inTokens, outTokens, time.Since(start))
}

// logUsage enqueues a UsageRecord for serial write by the usageWriter goroutine.
func (s *Server) logUsage(keyID string, upstreamID uint, upstreamName string, success bool, in, out int, latency time.Duration) {
	select {
	case s.usageCh <- models.UsageRecord{
		KeyID:        keyID,
		UpstreamID:   upstreamID,
		UpstreamName: upstreamName,
		InputTokens:  in,
		OutputTokens: out,
		LatencyMs:    latency.Milliseconds(),
		Success:      success,
	}:
	default: // drop if channel full (backpressure)
	}
}

// proxyStream copies an SSE upstream response to the client with per-chunk flushing,
// a heartbeat goroutine, and async usage logging on completion.
func (s *Server) proxyStream(c *gin.Context, resp *http.Response, cancel context.CancelFunc, keyID string, upstreamID uint, upstreamName string, start time.Time) {
	defer cancel()
	defer resp.Body.Close()

	// Set streaming headers before any body write (D-09)
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

	// mu serializes writes from the heartbeat goroutine and the main read loop.
	// Both write to c.Writer (not concurrency-safe) so all writes must be guarded.
	var mu sync.Mutex
	writeAndFlush := func(p []byte) {
		mu.Lock()
		c.Writer.Write(p) //nolint:errcheck
		flusher.Flush()
		mu.Unlock()
	}

	// Heartbeat goroutine — exits when handler returns via defer close(done) (T-3-08)
	// Capture interval before goroutine launch to avoid data race with test overrides.
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

	// Copy upstream SSE stream to client; flush after each read so frames arrive immediately
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			writeAndFlush(buf[:n])
		}
		if err != nil {
			break // EOF or mid-stream failure (D-10): close connection, no retry
		}
	}

	// Log usage after stream completes; token counts are 0 for streaming per D-13 fallback
	s.logUsage(keyID, upstreamID, upstreamName, true, 0, 0, time.Since(start))
}
