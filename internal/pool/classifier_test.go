package pool_test

import (
	"testing"

	"one-codingplan/internal/pool"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		status   int
		body     string
		want     pool.ErrorClass
	}{
		{
			name:     "TestClassify_Kimi_CreditsExhausted",
			provider: "kimi",
			status:   403,
			body:     `{"error":{"message":"Your account's current quota has been exhausted","type":"exceeded_current_quota_error"}}`,
			want:     pool.ClassCreditsExhausted,
		},
		{
			name:     "TestClassify_GLM_1113",
			provider: "glm",
			status:   429,
			body:     `{"error":{"code":1113,"message":"Insufficient balance or no resource package"}}`,
			want:     pool.ClassCreditsExhausted,
		},
		{
			name:     "TestClassify_Minimax_1008",
			provider: "minimax",
			status:   500,
			body:     `{"error":{"type":"api_error","message":"insufficient balance (1008)"}}`,
			want:     pool.ClassCreditsExhausted,
		},
		{
			name:     "TestClassify_Qwen_InsufficientQuota",
			provider: "qwen",
			status:   429,
			body:     `{"error":{"code":"DataInspectionFailed","message":"insufficient_quota","request_id":"xxx"}}`,
			want:     pool.ClassCreditsExhausted,
		},
		{
			name:     "TestClassify_RateLimit_429",
			provider: "kimi",
			status:   429,
			body:     `{"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`,
			want:     pool.ClassRateLimited,
		},
		{
			name:     "TestClassify_Transient_503",
			provider: "unknown",
			status:   503,
			body:     `{"error":{"message":"service unavailable"}}`,
			want:     pool.ClassTransient,
		},
		{
			name:     "TestClassify_402_CreditsExhausted",
			provider: "unknown",
			status:   402,
			body:     "",
			want:     pool.ClassCreditsExhausted,
		},
		{
			name:     "TestClassify_Unknown_500",
			provider: "unknown",
			status:   500,
			body:     `{"error":{"message":"internal server error"}}`,
			want:     pool.ClassTransient,
		},
		{
			name:     "TestClassify_GLM_PureRateLimit",
			provider: "glm",
			status:   429,
			body:     `{"error":{"code":1301,"message":"rate limit exceeded"}}`,
			want:     pool.ClassRateLimited,
		},
		// Model/config error cases (ROUT-05)
		{
			name:     "TestClassify_Minimax_ModelNotSupported_500",
			provider: "minimax",
			status:   500,
			body:     `{"code":1000,"message":"your current token plan not support model, MiniMax-Text-01 (2061)"}`,
			want:     pool.ClassModelNotSupported,
		},
		{
			name:     "TestClassify_Kimi_InvalidModel_501",
			provider: "kimi",
			status:   501,
			body:     `{"error":"invalid model specified"}`,
			want:     pool.ClassModelNotSupported,
		},
		{
			// 503 is a gateway error — treat as transient even if body contains model keyword
			name:     "TestClassify_GLM_ModelDoesNotExist_503",
			provider: "glm",
			status:   503,
			body:     `{"error":"model does not exist"}`,
			want:     pool.ClassTransient,
		},
		// 4xx with model keyword → transient (not 5xx)
		{
			name:     "TestClassify_ModelKeyword_400_Transient",
			provider: "any",
			status:   400,
			body:     `{"error":"model does not exist"}`,
			want:     pool.ClassTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pool.Classify(tt.provider, tt.status, []byte(tt.body))
			if got != tt.want {
				t.Errorf("Classify(%q, %d, %q) = %v, want %v",
					tt.provider, tt.status, tt.body, got, tt.want)
			}
		})
	}
}

// Individual named test functions for plan acceptance criteria.

func TestClassify_Kimi_CreditsExhausted(t *testing.T) {
	body := `{"error":{"message":"Your account's current quota has been exhausted","type":"exceeded_current_quota_error"}}`
	got := pool.Classify("kimi", 403, []byte(body))
	if got != pool.ClassCreditsExhausted {
		t.Errorf("got %v, want ClassCreditsExhausted", got)
	}
}

func TestClassify_GLM_1113(t *testing.T) {
	body := `{"error":{"code":1113,"message":"Insufficient balance or no resource package"}}`
	got := pool.Classify("glm", 429, []byte(body))
	if got != pool.ClassCreditsExhausted {
		t.Errorf("got %v, want ClassCreditsExhausted (not ClassRateLimited)", got)
	}
}

func TestClassify_Minimax_1008(t *testing.T) {
	body := `{"error":{"type":"api_error","message":"insufficient balance (1008)"}}`
	got := pool.Classify("minimax", 500, []byte(body))
	if got != pool.ClassCreditsExhausted {
		t.Errorf("got %v, want ClassCreditsExhausted (not ClassTransient)", got)
	}
}

func TestClassify_Qwen_InsufficientQuota(t *testing.T) {
	body := `{"error":{"code":"DataInspectionFailed","message":"insufficient_quota","request_id":"xxx"}}`
	got := pool.Classify("qwen", 429, []byte(body))
	if got != pool.ClassCreditsExhausted {
		t.Errorf("got %v, want ClassCreditsExhausted", got)
	}
}

func TestClassify_GLM_PureRateLimit(t *testing.T) {
	body := `{"error":{"code":1301,"message":"rate limit exceeded"}}`
	got := pool.Classify("glm", 429, []byte(body))
	if got != pool.ClassRateLimited {
		t.Errorf("got %v, want ClassRateLimited", got)
	}
}
