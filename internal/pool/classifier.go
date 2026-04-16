package pool

import "strings"

// ErrorClass categorizes an upstream error response for routing decisions.
type ErrorClass int

const (
	ClassTransient        ErrorClass = iota
	ClassRateLimited
	ClassCreditsExhausted
)

// String returns a human-readable representation of the error class.
func (c ErrorClass) String() string {
	switch c {
	case ClassRateLimited:
		return "rate-limited"
	case ClassCreditsExhausted:
		return "credits-exhausted"
	default:
		return "transient"
	}
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

	keywords := defaultCreditsKeywords
	if overrides, ok := providerCreditsKeywords[provider]; ok {
		keywords = overrides
	}
	for _, kw := range keywords {
		if strings.Contains(bodyStr, kw) {
			return ClassCreditsExhausted
		}
	}

	if status == 402 {
		return ClassCreditsExhausted
	}
	if status == 429 {
		return ClassRateLimited
	}
	return ClassTransient
}
