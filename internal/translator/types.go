package translator

import (
	"encoding/json"
	"strings"
)

// --- Anthropic types ---

type AnthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []AnthropicMessage `json:"messages"`
	System    json.RawMessage    `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream,omitempty"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string OR []AnthropicContentBlock
}

type AnthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   interface{}     `json:"content,omitempty"`
}

type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        AnthropicUsage          `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// --- OpenAI types ---

type OpenAIRequest struct {
	Model     string          `json:"model,omitempty"`
	Messages  []OpenAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
	Tools     []OpenAITool    `json:"tools,omitempty"`
}

type OpenAIMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
	Model   string         `json:"model"`
}

type OpenAIChoice struct {
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// --- Responses API types (OpenAI Responses API for Codex) ---

type ResponsesRequest struct {
	Model        string          `json:"model"`
	Input        json.RawMessage `json:"input"`
	Instructions string          `json:"instructions,omitempty"`
	Stream       bool            `json:"stream,omitempty"`
	Tools        []ResponsesTool `json:"tools,omitempty"`
}

// ResponsesTool is a tool definition in Responses API format (flat, not nested like chat completions).
// Only "function" type tools are forwarded to upstreams; built-in types are silently dropped.
type ResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ResponsesInputMessage represents a single item in the Responses API input array.
// Items can be role-based messages OR typed items (function_call, function_call_output).
type ResponsesInputMessage struct {
	// Role-based message fields
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
	// function_call / function_call_output item fields
	Type      string `json:"type,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Output    string `json:"output,omitempty"`
}

// ContentText extracts the plain text from Content, which may be a JSON string
// or an array of content parts (e.g. [{"type":"input_text","text":"..."}]).
func (m *ResponsesInputMessage) ContentText() string {
	if len(m.Content) == 0 {
		return ""
	}
	// String form: "hello"
	if m.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			return s
		}
	}
	// Array form: [{"type":"input_text","text":"..."}]
	if m.Content[0] == '[' {
		var parts []ResponsesContentPart
		if err := json.Unmarshal(m.Content, &parts); err == nil {
			var sb strings.Builder
			for _, p := range parts {
				sb.WriteString(p.Text)
			}
			return sb.String()
		}
	}
	return ""
}

type ResponsesResponse struct {
	ID        string            `json:"id"`
	Object    string            `json:"object"`
	CreatedAt int64             `json:"created_at"`
	Status    string            `json:"status"`
	Model     string            `json:"model"`
	Output    []ResponsesOutput `json:"output"`
	Usage     ResponsesUsage    `json:"usage"`
}

type ResponsesOutput struct {
	Type    string                 `json:"type"`
	ID      string                 `json:"id,omitempty"`
	Status  string                 `json:"status,omitempty"`
	// For "message" type
	Role    string                 `json:"role,omitempty"`
	Content []ResponsesContentPart `json:"content,omitempty"`
	// For "function_call" type
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type ResponsesContentPart struct {
	Type        string          `json:"type"`
	Text        string          `json:"text,omitempty"`
	// For tool_result input parts
	ToolUseID   string          `json:"tool_use_id,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	Content     json.RawMessage `json:"content,omitempty"`
	Annotations []string        `json:"annotations,omitempty"`
}

// ContentText extracts plain text from a ResponsesContentPart, used for tool_result content
// which may be a JSON string or array of text parts.
func (p *ResponsesContentPart) ContentText() string {
	if len(p.Content) == 0 {
		return p.Text
	}
	if p.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(p.Content, &s); err == nil {
			return s
		}
	}
	if p.Content[0] == '[' {
		var parts []ResponsesContentPart
		if err := json.Unmarshal(p.Content, &parts); err == nil {
			var sb strings.Builder
			for _, part := range parts {
				sb.WriteString(part.Text)
			}
			return sb.String()
		}
	}
	return p.Text
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
