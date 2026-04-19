package pool

import "strings"

// ErrorClass categorizes an upstream error response for routing decisions.
type ErrorClass int

const (
	ClassTransient        ErrorClass = iota
	ClassRateLimited
	ClassCreditsExhausted
	ClassModelNotSupported
)

// String returns a human-readable representation of the error class.
func (c ErrorClass) String() string {
	switch c {
	case ClassRateLimited:
		return "rate-limited"
	case ClassCreditsExhausted:
		return "credits-exhausted"
	case ClassModelNotSupported:
		return "model-not-supported"
	default:
		return "transient"
	}
}

// modelNotSupportedKeywords are body substrings that signal a model/config error.
// Only matched on 5xx responses — 4xx with these strings are treated as transient.
var modelNotSupportedKeywords = []string{
	"not support model",
	"invalid model",
	"model does not exist",
	"model not found",
	"unsupported model",
}

// defaultCreditsKeywords are body substrings that signal credits exhaustion
// for providers not in providerCreditsKeywords.
var defaultCreditsKeywords = []string{
	"insufficient", "quota", "balance", "out of credits",
	"no credit", "token limit", "recharge",
}

// providerCreditsKeywords overrides the default keyword list for specific
// providers whose error bodies require narrower matching (D-11, D-14).
var providerCreditsKeywords = map[string][]string{
	"glm":     {"1113", "insufficient balance"},
	"minimax": {"1008", "insufficient balance"},
}

// Classify returns the ErrorClass for an upstream response.
//
// CRITICAL ORDER (per RESEARCH.md Pitfall 2, 3, 4):
// Body keyword check MUST occur before the HTTP 429 → RateLimited rule.
// GLM and Qwen both return 429 for credits exhaustion; without this ordering
// they would be misclassified as rate-limited rather than credits-exhausted.
func Classify(provider string, status int, body []byte) ErrorClass {
	bodyStr := strings.ToLower(string(body))

	// Model/config errors: 500/501 only — 502/503/504 are gateway errors (ROUT-05)
	if status == 500 || status == 501 {
		for _, kw := range modelNotSupportedKeywords {
			if strings.Contains(bodyStr, kw) {
				return ClassModelNotSupported
			}
		}
	}

	keywords := defaultCreditsKeywords
	if overrides, ok := providerCreditsKeywords[provider]; ok {
		keywords = overrides
	}
	for _, kw := range keywords {
		if strings.Contains(bodyStr, kw) {
			return ClassCreditsExhausted
		}
	}

	if status == 402 || status == 403 {
		return ClassCreditsExhausted
	}
	if status == 404 {
		return ClassModelNotSupported
	}
	if status == 429 {
		return ClassRateLimited
	}
	return ClassTransient
}
