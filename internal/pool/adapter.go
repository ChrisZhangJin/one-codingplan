package pool

import (
	"net/http"
	"strings"
)

// ProviderAdapter constructs the full upstream URL from a base URL for each protocol.
type ProviderAdapter interface {
	AnthropicURL(baseURL string) string
	OpenAIURL(baseURL string) string
	// InjectHeaders sets any provider-specific headers required on outgoing requests.
	InjectHeaders(h http.Header)
}

// DefaultAdapter handles standard providers using canonical API paths.
type DefaultAdapter struct{}

func (DefaultAdapter) AnthropicURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/v1/messages"
}

func (DefaultAdapter) OpenAIURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
}

func (DefaultAdapter) InjectHeaders(_ http.Header) {}

// MinimaxAdapter handles Minimax, which uses /anthropic/v1/messages for the Anthropic protocol.
type MinimaxAdapter struct {
	DefaultAdapter
}

func (MinimaxAdapter) AnthropicURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/anthropic/v1/messages"
}

// KimiAdapter injects a recognized coding-agent User-Agent required by the Kimi coding endpoint.
type KimiAdapter struct {
	DefaultAdapter
}

func (KimiAdapter) InjectHeaders(h http.Header) {
	h.Set("User-Agent", "claude-code/1.0.0")
}

var adapters = map[string]ProviderAdapter{}

func init() {
	adapters["minimax"] = MinimaxAdapter{}
	adapters["mimo"] = MinimaxAdapter{}
	adapters["kimi"] = KimiAdapter{}
}

// GetAdapter returns the ProviderAdapter for the given provider name.
// Falls back to DefaultAdapter for unknown providers.
func GetAdapter(provider string) ProviderAdapter {
	if a, ok := adapters[provider]; ok {
		return a
	}
	return DefaultAdapter{}
}
