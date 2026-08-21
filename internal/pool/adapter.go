package pool

import (
	"net/http"
	"strings"
)

// NormalizeBaseURL ensures the base URL has an http(s) scheme. Bare
// host:port values like "10.0.3.248:3000/api" fail url.Parse with
// "first path segment in URL cannot contain colon", so we default to
// http:// when no scheme is present.
func NormalizeBaseURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return s
	}
	return "http://" + s
}

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

// glmBasePathSuffixes are path suffixes users commonly paste as the GLM base URL.
// Zhipu documents "https://open.bigmodel.cn/api/paas/v4" as the OpenAI base URL,
// so that value ends up in the portal a lot — but neither protocol path hangs off
// it, so GLMAdapter trims back to the host root before appending its own paths.
var glmBasePathSuffixes = []string{"/api/anthropic", "/api/paas/v4", "/api/paas"}

// GLMAdapter handles Zhipu GLM, which serves each protocol from a different
// path and neither of them under the canonical /v1 prefix:
//
//	Anthropic: /api/anthropic/v1/messages
//	OpenAI:    /api/paas/v4/chat/completions
type GLMAdapter struct {
	DefaultAdapter
}

// glmRoot strips any known GLM API path suffix so the caller can append its own.
func glmRoot(baseURL string) string {
	s := strings.TrimRight(baseURL, "/")
	for _, suffix := range glmBasePathSuffixes {
		if strings.HasSuffix(s, suffix) {
			return strings.TrimSuffix(s, suffix)
		}
	}
	return s
}

func (GLMAdapter) AnthropicURL(baseURL string) string {
	return glmRoot(baseURL) + "/api/anthropic/v1/messages"
}

func (GLMAdapter) OpenAIURL(baseURL string) string {
	return glmRoot(baseURL) + "/api/paas/v4/chat/completions"
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
	adapters["glm"] = GLMAdapter{}
}

// GetAdapter returns the ProviderAdapter for the given provider name.
// Falls back to DefaultAdapter for unknown providers.
func GetAdapter(provider string) ProviderAdapter {
	if a, ok := adapters[provider]; ok {
		return a
	}
	return DefaultAdapter{}
}
