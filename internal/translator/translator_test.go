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
