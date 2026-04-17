package pool

import "strings"

// ProviderAdapter constructs the full upstream URL from a base URL for each protocol.
type ProviderAdapter interface {
	AnthropicURL(baseURL string) string
	OpenAIURL(baseURL string) string
}

// DefaultAdapter handles standard providers using canonical API paths.
type DefaultAdapter struct{}

func (DefaultAdapter) AnthropicURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/v1/messages"
}

func (DefaultAdapter) OpenAIURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
}

// MinimaxAdapter handles Minimax, which uses /anthropic/v1/messages for the Anthropic protocol.
type MinimaxAdapter struct {
	DefaultAdapter
}

func (MinimaxAdapter) AnthropicURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/anthropic/v1/messages"
}

var adapters = map[string]ProviderAdapter{}

func init() {
	adapters["minimax"] = MinimaxAdapter{}
}

// GetAdapter returns the ProviderAdapter for the given provider name.
// Falls back to DefaultAdapter for unknown providers.
func GetAdapter(provider string) ProviderAdapter {
	if a, ok := adapters[provider]; ok {
		return a
	}
	return DefaultAdapter{}
}
