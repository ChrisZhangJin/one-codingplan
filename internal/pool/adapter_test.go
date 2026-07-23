package pool

import "testing"

func TestDefaultAdapterAnthropicURL(t *testing.T) {
	a := DefaultAdapter{}
	tests := []struct {
		baseURL string
		want    string
	}{
		{"https://api.example.com", "https://api.example.com/v1/messages"},
		{"https://api.example.com/", "https://api.example.com/v1/messages"},
	}
	for _, tt := range tests {
		got := a.AnthropicURL(tt.baseURL)
		if got != tt.want {
			t.Errorf("AnthropicURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
		}
	}
}

func TestDefaultAdapterOpenAIURL(t *testing.T) {
	a := DefaultAdapter{}
	tests := []struct {
		baseURL string
		want    string
	}{
		{"https://api.example.com", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/", "https://api.example.com/v1/chat/completions"},
	}
	for _, tt := range tests {
		got := a.OpenAIURL(tt.baseURL)
		if got != tt.want {
			t.Errorf("OpenAIURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
		}
	}
}

func TestMinimaxAdapterAnthropicURL(t *testing.T) {
	a := MinimaxAdapter{}
	tests := []struct {
		baseURL string
		want    string
	}{
		{"https://api.minimaxi.com", "https://api.minimaxi.com/anthropic/v1/messages"},
		{"https://api.minimaxi.com/", "https://api.minimaxi.com/anthropic/v1/messages"},
	}
	for _, tt := range tests {
		got := a.AnthropicURL(tt.baseURL)
		if got != tt.want {
			t.Errorf("AnthropicURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
		}
	}
}

func TestMinimaxAdapterOpenAIURL(t *testing.T) {
	a := MinimaxAdapter{}
	got := a.OpenAIURL("https://api.minimaxi.com")
	want := "https://api.minimaxi.com/v1/chat/completions"
	if got != want {
		t.Errorf("OpenAIURL = %q, want %q", got, want)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  ", ""},
		{"http://x.com", "http://x.com"},
		{"https://x.com", "https://x.com"},
		{"HTTP://x.com", "HTTP://x.com"},
		{"10.0.3.248:3000/api", "http://10.0.3.248:3000/api"},
		{"example.com/v1", "http://example.com/v1"},
		{" https://x.com ", "https://x.com"},
	}
	for _, tt := range tests {
		if got := NormalizeBaseURL(tt.in); got != tt.want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGetAdapter(t *testing.T) {
	tests := []struct {
		provider string
		wantType string
	}{
		{"minimax", "minimax"},
		{"kimi", "kimi"},
		{"", "default"},
		{"unknown", "default"},
	}
	for _, tt := range tests {
		a := GetAdapter(tt.provider)
		switch tt.wantType {
		case "minimax":
			if _, ok := a.(MinimaxAdapter); !ok {
				t.Errorf("GetAdapter(%q) = %T, want MinimaxAdapter", tt.provider, a)
			}
		case "kimi":
			if _, ok := a.(KimiAdapter); !ok {
				t.Errorf("GetAdapter(%q) = %T, want KimiAdapter", tt.provider, a)
			}
		case "default":
			if _, ok := a.(DefaultAdapter); !ok {
				t.Errorf("GetAdapter(%q) = %T, want DefaultAdapter", tt.provider, a)
			}
		}
	}
}
