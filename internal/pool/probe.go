package pool

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"one-codingplan/internal/models"
)

// probeClient is the HTTP client used for all probe requests.
// Not DefaultClient — has explicit timeout.
var probeClient = &http.Client{Timeout: 10 * time.Second}

// probeModels maps provider name to a known-cheap model for probe requests.
var probeModels = map[string]string{
	"kimi":    "moonshot-v1-8k",
	"qwen":    "qwen-turbo",
	"glm":     "glm-4-flash",
	"minimax": "MiniMax-Text-01",
}

// defaultAnthropicProbeModel is the fallback Anthropic model used by probes
// when an upstream has no ModelOverride set. Cheap, broadly available.
const defaultAnthropicProbeModel = "claude-3-5-haiku-latest"

func probeModel(provider string) string {
	if m, ok := probeModels[provider]; ok {
		return m
	}
	return "gpt-3.5-turbo"
}

// probeAnthropicModel picks a model name for Anthropic probes: the upstream's
// ModelOverride if set, otherwise a sensible default. Self-hosted proxies
// often passthrough model names unchanged, so any string usually works.
func probeAnthropicModel(e entry) string {
	if e.ModelOverride != "" {
		return e.ModelOverride
	}
	return defaultAnthropicProbeModel
}

// runProbeLoop runs on a goroutine started by StartProbeLoop.
// It probes unavailable upstreams on an hourly ticker and stops when stopCh is closed.
func (p *Pool) runProbeLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.probeAll()
		case <-p.stopCh:
			return
		}
	}
}

// probeAll probes all currently unavailable upstreams and marks them available on success.
func (p *Pool) probeAll() {
	p.mu.RLock()
	var unavailable []entry
	for _, e := range p.entries {
		if !e.available {
			unavailable = append(unavailable, e)
		}
	}
	p.mu.RUnlock()

	for _, e := range unavailable {
		if sendProbe(e) {
			p.Mark(e.ID, true)
		}
	}
}

// sendProbe dispatches to the right probe shape based on the upstream protocol.
// For 'both' and unknown, prefer the OpenAI probe because every legacy upstream
// supports it and that keeps current observable behaviour unchanged.
func sendProbe(e entry) bool {
	if e.Protocol == models.ProtocolAnthropic {
		return sendAnthropicProbe(e)
	}
	return sendOpenAIProbe(e)
}

// sendOpenAIProbe sends a minimal chat completion request to the upstream and
// returns true if the upstream responded with a 2xx status that is not
// credits-exhausted.
func sendOpenAIProbe(e entry) bool {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type requestBody struct {
		Model     string    `json:"model"`
		Messages  []message `json:"messages"`
		MaxTokens int       `json:"max_tokens"`
	}

	body := requestBody{
		Model:     probeModel(e.Name),
		Messages:  []message{{Role: "user", Content: "hi"}},
		MaxTokens: 1,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := GetAdapter(e.Name).OpenAIURL(e.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)

	resp, err := probeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	if Classify(e.Name, resp.StatusCode, respBody) == ClassCreditsExhausted {
		return false
	}
	return true
}

// sendAnthropicProbe sends a minimal Anthropic Messages request to the upstream
// and returns true on 2xx (non-credits-exhausted). Used for upstreams that
// only speak the Anthropic protocol — typically self-hosted Claude proxies.
func sendAnthropicProbe(e entry) bool {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type requestBody struct {
		Model     string    `json:"model"`
		MaxTokens int       `json:"max_tokens"`
		Messages  []message `json:"messages"`
	}

	body := requestBody{
		Model:     probeAnthropicModel(e),
		MaxTokens: 1,
		Messages:  []message{{Role: "user", Content: "hi"}},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := GetAdapter(e.Name).AnthropicURL(e.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", e.APIKey)
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	GetAdapter(e.Name).InjectHeaders(req.Header)

	resp, err := probeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	if Classify(e.Name, resp.StatusCode, respBody) == ClassCreditsExhausted {
		return false
	}
	return true
}
