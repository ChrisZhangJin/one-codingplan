package translator

import (
	"encoding/json"
	"testing"
)

func TestAnthropicToOpenAI_SimpleText(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello"},
		},
	}
	out, err := AnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Model != "" {
		t.Errorf("expected empty model (D-02), got %q", out.Model)
	}
	if out.MaxTokens != 1024 {
		t.Errorf("expected max_tokens=1024, got %d", out.MaxTokens)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	if out.Messages[0].Role != "user" {
		t.Errorf("expected role=user, got %q", out.Messages[0].Role)
	}
	if out.Messages[0].Content != "hello" {
		t.Errorf("expected content=hello, got %q", out.Messages[0].Content)
	}
}

func TestAnthropicToOpenAI_WithModelOverride(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello"},
		},
	}
	out, err := AnthropicToOpenAI(req, "qwen-max")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Model != "qwen-max" {
		t.Errorf("expected model=qwen-max (D-01), got %q", out.Model)
	}
}

func TestAnthropicToOpenAI_SystemPrompt(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 1024,
		System:    "You are helpful",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hi"},
		},
	}
	out, err := AnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(out.Messages))
	}
	if out.Messages[0].Role != "system" {
		t.Errorf("expected first message role=system, got %q", out.Messages[0].Role)
	}
	if out.Messages[0].Content != "You are helpful" {
		t.Errorf("expected system content, got %q", out.Messages[0].Content)
	}
	if out.Messages[1].Role != "user" {
		t.Errorf("expected second message role=user, got %q", out.Messages[1].Role)
	}
}

func TestAnthropicToOpenAI_Tools(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)
	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 1024,
		Messages:  []AnthropicMessage{{Role: "user", Content: "weather?"}},
		Tools: []AnthropicTool{
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: schema,
			},
		},
	}
	out, err := AnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(out.Tools))
	}
	tool := out.Tools[0]
	if tool.Type != "function" {
		t.Errorf("expected tool type=function, got %q", tool.Type)
	}
	if tool.Function.Name != "get_weather" {
		t.Errorf("expected function name=get_weather, got %q", tool.Function.Name)
	}
	if tool.Function.Description != "Get weather" {
		t.Errorf("expected description, got %q", tool.Function.Description)
	}
	if string(tool.Function.Parameters) != string(schema) {
		t.Errorf("expected parameters=%s, got %s", schema, tool.Function.Parameters)
	}
}

func TestAnthropicToOpenAI_ToolResult(t *testing.T) {
	// User message with tool_result block (D-07)
	blocks := []interface{}{
		map[string]interface{}{
			"type":        "tool_result",
			"tool_use_id": "call_123",
			"content":     "sunny",
		},
	}
	blocksJSON, _ := json.Marshal(blocks)
	var content interface{}
	json.Unmarshal(blocksJSON, &content)

	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: content},
		},
	}
	out, err := AnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message (tool result), got %d", len(out.Messages))
	}
	msg := out.Messages[0]
	if msg.Role != "tool" {
		t.Errorf("expected role=tool, got %q", msg.Role)
	}
	if msg.ToolCallID != "call_123" {
		t.Errorf("expected tool_call_id=call_123, got %q", msg.ToolCallID)
	}
	if msg.Content != "sunny" {
		t.Errorf("expected content=sunny, got %q", msg.Content)
	}
}

func TestAnthropicToOpenAI_AssistantToolUse(t *testing.T) {
	input := json.RawMessage(`{"city":"NYC"}`)
	blocks := []interface{}{
		map[string]interface{}{
			"type":  "tool_use",
			"id":    "call_123",
			"name":  "get_weather",
			"input": json.RawMessage(`{"city":"NYC"}`),
		},
	}
	blocksJSON, _ := json.Marshal(blocks)
	var content interface{}
	json.Unmarshal(blocksJSON, &content)

	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "assistant", Content: content},
		},
	}
	out, err := AnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 assistant message, got %d", len(out.Messages))
	}
	msg := out.Messages[0]
	if msg.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", msg.Role)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_123" {
		t.Errorf("expected tool call id=call_123, got %q", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("expected tool call type=function, got %q", tc.Type)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected function name=get_weather, got %q", tc.Function.Name)
	}
	// Arguments should be the JSON-marshaled input
	var gotArgs map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &gotArgs); err != nil {
		t.Errorf("arguments not valid JSON: %v", err)
	}
	_ = input // used as reference
}

func TestAnthropicToOpenAI_ContentArray(t *testing.T) {
	blocks := []interface{}{
		map[string]interface{}{"type": "text", "text": "part1"},
		map[string]interface{}{"type": "text", "text": "part2"},
	}
	blocksJSON, _ := json.Marshal(blocks)
	var content interface{}
	json.Unmarshal(blocksJSON, &content)

	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: content},
		},
	}
	out, err := AnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	if out.Messages[0].Content != "part1part2" {
		t.Errorf("expected concatenated content 'part1part2', got %q", out.Messages[0].Content)
	}
}

func TestAnthropicToOpenAI_MaxTokensDefault(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 0,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello"},
		},
	}
	out, err := AnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.MaxTokens != 4096 {
		t.Errorf("expected default max_tokens=4096, got %d", out.MaxTokens)
	}
}

func TestAnthropicToOpenAI_Stream(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-opus-4-5",
		MaxTokens: 1024,
		Stream:    true,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello"},
		},
	}
	out, err := AnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Stream {
		t.Error("expected stream=true")
	}
}

func TestOpenAIToAnthropic_Text(t *testing.T) {
	resp := &OpenAIResponse{
		ID:    "chatcmpl-123",
		Model: "qwen-max",
		Choices: []OpenAIChoice{
			{
				Message:      OpenAIMessage{Role: "assistant", Content: "Hello"},
				FinishReason: "stop",
			},
		},
		Usage: OpenAIUsage{PromptTokens: 10, CompletionTokens: 5},
	}
	out, err := OpenAIToAnthropic(resp, "claude-opus-4-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Type != "message" {
		t.Errorf("expected type=message, got %q", out.Type)
	}
	if out.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", out.Role)
	}
	if out.Model != "claude-opus-4-5" {
		t.Errorf("expected model=claude-opus-4-5 (D-03), got %q", out.Model)
	}
	if out.StopReason != "end_turn" {
		t.Errorf("expected stop_reason=end_turn, got %q", out.StopReason)
	}
	if out.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens=10, got %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens=5, got %d", out.Usage.OutputTokens)
	}
	if len(out.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(out.Content))
	}
	if out.Content[0].Type != "text" {
		t.Errorf("expected content[0].type=text, got %q", out.Content[0].Type)
	}
	if out.Content[0].Text != "Hello" {
		t.Errorf("expected content[0].text=Hello, got %q", out.Content[0].Text)
	}
}

func TestOpenAIToAnthropic_ToolUse(t *testing.T) {
	resp := &OpenAIResponse{
		ID:    "chatcmpl-456",
		Model: "qwen-max",
		Choices: []OpenAIChoice{
			{
				Message: OpenAIMessage{
					Role: "assistant",
					ToolCalls: []OpenAIToolCall{
						{
							ID:   "call_abc",
							Type: "function",
							Function: OpenAIFunctionCall{
								Name:      "get_weather",
								Arguments: `{"city":"NYC"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
	out, err := OpenAIToAnthropic(resp, "claude-opus-4-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.StopReason != "tool_use" {
		t.Errorf("expected stop_reason=tool_use, got %q", out.StopReason)
	}
	if len(out.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(out.Content))
	}
	block := out.Content[0]
	if block.Type != "tool_use" {
		t.Errorf("expected type=tool_use, got %q", block.Type)
	}
	if block.ID != "call_abc" {
		t.Errorf("expected id=call_abc (D-07), got %q", block.ID)
	}
	if block.Name != "get_weather" {
		t.Errorf("expected name=get_weather, got %q", block.Name)
	}
	var input map[string]interface{}
	if err := json.Unmarshal(block.Input, &input); err != nil {
		t.Fatalf("input not valid JSON: %v", err)
	}
	if input["city"] != "NYC" {
		t.Errorf("expected city=NYC, got %v", input["city"])
	}
}

func TestOpenAIToAnthropic_TextAndToolUse(t *testing.T) {
	resp := &OpenAIResponse{
		ID:    "chatcmpl-789",
		Model: "qwen-max",
		Choices: []OpenAIChoice{
			{
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: "Let me check",
					ToolCalls: []OpenAIToolCall{
						{
							ID:   "call_xyz",
							Type: "function",
							Function: OpenAIFunctionCall{
								Name:      "get_weather",
								Arguments: `{"city":"LA"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
	out, err := OpenAIToAnthropic(resp, "claude-opus-4-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Content) != 2 {
		t.Fatalf("expected 2 content blocks (text+tool_use), got %d", len(out.Content))
	}
	if out.Content[0].Type != "text" {
		t.Errorf("expected content[0].type=text, got %q", out.Content[0].Type)
	}
	if out.Content[1].Type != "tool_use" {
		t.Errorf("expected content[1].type=tool_use, got %q", out.Content[1].Type)
	}
}

func TestModelEcho(t *testing.T) {
	resp := &OpenAIResponse{
		ID:    "chatcmpl-abc",
		Model: "gpt-4",
		Choices: []OpenAIChoice{
			{
				Message:      OpenAIMessage{Role: "assistant", Content: "hi"},
				FinishReason: "stop",
			},
		},
	}
	out, err := OpenAIToAnthropic(resp, "claude-3-haiku")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Model != "claude-3-haiku" {
		t.Errorf("expected model=claude-3-haiku (D-03), got %q", out.Model)
	}
}

func TestOpenAIToAnthropic_FinishReasons(t *testing.T) {
	cases := []struct {
		reason   string
		expected string
	}{
		{"stop", "end_turn"},
		{"tool_calls", "tool_use"},
		{"length", "max_tokens"},
		{"content_filter", "end_turn"},
		{"", "end_turn"},
	}
	for _, tc := range cases {
		resp := &OpenAIResponse{
			Choices: []OpenAIChoice{
				{
					Message:      OpenAIMessage{Role: "assistant", Content: "x"},
					FinishReason: tc.reason,
				},
			},
		}
		out, err := OpenAIToAnthropic(resp, "claude-opus-4-5")
		if err != nil {
			t.Fatalf("finish_reason=%q: unexpected error: %v", tc.reason, err)
		}
		if out.StopReason != tc.expected {
			t.Errorf("finish_reason=%q: expected stop_reason=%q, got %q", tc.reason, tc.expected, out.StopReason)
		}
	}
}

func TestOpenAIToAnthropic_NoChoices(t *testing.T) {
	resp := &OpenAIResponse{
		ID:      "chatcmpl-empty",
		Choices: []OpenAIChoice{},
	}
	_, err := OpenAIToAnthropic(resp, "claude-opus-4-5")
	if err == nil {
		t.Fatal("expected error for empty choices, got nil")
	}
}
