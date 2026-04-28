package translator

import (
	"encoding/json"
	"testing"
)

func TestResponsesRequestParseInput_StringRejected(t *testing.T) {
	raw := json.RawMessage(`"hello"`)
	_, err := ParseResponsesInput(raw)
	if err != ErrStringInput {
		t.Errorf("expected ErrStringInput, got %v", err)
	}
}

func TestResponsesRequestParseInput_ArrayParsed(t *testing.T) {
	raw := json.RawMessage(`[{"role":"user","content":"test"}]`)
	msgs, err := ParseResponsesInput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].ContentText() != "test" {
		t.Errorf("unexpected message: role=%q content=%q", msgs[0].Role, msgs[0].ContentText())
	}
}

func TestResponsesRequestParseInput_EmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	msgs, err := ParseResponsesInput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty slice, got %d messages", len(msgs))
	}
}

func TestResponsesRequestToOpenAI_WithInstructions(t *testing.T) {
	req := &ResponsesRequest{
		Model:        "qwen-max",
		Instructions: "You are helpful",
	}
	msgs := []ResponsesInputMessage{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
	}
	out := ResponsesRequestToOpenAI(req, msgs, "")
	if len(out.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(out.Messages))
	}
	if out.Messages[0].Role != "system" || out.Messages[0].Content != "You are helpful" {
		t.Errorf("expected system message with instructions, got %+v", out.Messages[0])
	}
	if out.Messages[1].Role != "user" || out.Messages[1].Content != "hello" {
		t.Errorf("expected user message, got %+v", out.Messages[1])
	}
	if out.Stream {
		t.Error("expected stream=false")
	}
}

func TestResponsesRequestToOpenAI_NoInstructions(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen-max"}
	msgs := []ResponsesInputMessage{
		{Role: "user", Content: json.RawMessage(`"hi"`)},
	}
	out := ResponsesRequestToOpenAI(req, msgs, "")
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	if out.Messages[0].Role != "user" {
		t.Errorf("expected user role, got %q", out.Messages[0].Role)
	}
}

func TestResponsesRequestToOpenAI_StreamPassthrough(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen-max", Stream: true}
	out := ResponsesRequestToOpenAI(req, nil, "")
	if !out.Stream {
		t.Error("expected stream=true to pass through")
	}
}

func TestResponsesRequestToOpenAI_ModelOverride(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen-max"}
	out := ResponsesRequestToOpenAI(req, nil, "gpt-4")
	if out.Model != "gpt-4" {
		t.Errorf("expected model=gpt-4, got %q", out.Model)
	}
}

func TestResponsesRequestToOpenAI_ModelPassthrough(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen-max"}
	out := ResponsesRequestToOpenAI(req, nil, "")
	if out.Model != "qwen-max" {
		t.Errorf("expected model=qwen-max, got %q", out.Model)
	}
}

func TestResponsesRequestToOpenAI_ToolsForwarded(t *testing.T) {
	req := &ResponsesRequest{
		Model: "qwen-max",
		Tools: []ResponsesTool{
			{Type: "function", Name: "bash", Description: "Run a command", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Type: "computer_use_preview"}, // built-in — should be dropped
		},
	}
	out := ResponsesRequestToOpenAI(req, nil, "")
	if len(out.Tools) != 1 {
		t.Fatalf("expected 1 tool (built-in dropped), got %d", len(out.Tools))
	}
	if out.Tools[0].Function.Name != "bash" {
		t.Errorf("expected tool name=bash, got %q", out.Tools[0].Function.Name)
	}
}

func TestResponsesRequestToOpenAI_FunctionCallItem(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen-max"}
	msgs := []ResponsesInputMessage{
		{Type: "function_call", ID: "call_abc", Name: "bash", Arguments: `{"command":"ls"}`},
	}
	out := ResponsesRequestToOpenAI(req, msgs, "")
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	m := out.Messages[0]
	if m.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", m.Role)
	}
	if len(m.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(m.ToolCalls))
	}
	if m.ToolCalls[0].ID != "call_abc" || m.ToolCalls[0].Function.Name != "bash" {
		t.Errorf("unexpected tool call: %+v", m.ToolCalls[0])
	}
}

func TestResponsesRequestToOpenAI_FunctionCallOutputItem(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen-max"}
	msgs := []ResponsesInputMessage{
		{Type: "function_call_output", CallID: "call_abc", Output: "file1.txt\nfile2.txt"},
	}
	out := ResponsesRequestToOpenAI(req, msgs, "")
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	m := out.Messages[0]
	if m.Role != "tool" {
		t.Errorf("expected role=tool, got %q", m.Role)
	}
	if m.ToolCallID != "call_abc" {
		t.Errorf("expected tool_call_id=call_abc, got %q", m.ToolCallID)
	}
	if m.Content != "file1.txt\nfile2.txt" {
		t.Errorf("unexpected content: %q", m.Content)
	}
}

func TestResponsesRequestToOpenAI_ToolResultContentPart(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen-max"}
	msgs := []ResponsesInputMessage{
		{
			Role: "user",
			Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_abc","content":"ok"}]`),
		},
	}
	out := ResponsesRequestToOpenAI(req, msgs, "")
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 tool message, got %d", len(out.Messages))
	}
	m := out.Messages[0]
	if m.Role != "tool" {
		t.Errorf("expected role=tool, got %q", m.Role)
	}
	if m.ToolCallID != "call_abc" {
		t.Errorf("expected tool_call_id=call_abc, got %q", m.ToolCallID)
	}
	if m.Content != "ok" {
		t.Errorf("expected content=ok, got %q", m.Content)
	}
}

func TestOpenAIToResponsesAPI_Basic(t *testing.T) {
	resp := &OpenAIResponse{
		ID:    "chatcmpl-123",
		Model: "qwen-max",
		Choices: []OpenAIChoice{
			{Message: OpenAIMessage{Role: "assistant", Content: "Hello world"}},
		},
		Usage: OpenAIUsage{PromptTokens: 10, CompletionTokens: 5},
	}
	out := OpenAIToResponsesAPI(resp, "qwen-max")
	if out.ID != "resp_chatcmpl-123" {
		t.Errorf("expected id=resp_chatcmpl-123, got %q", out.ID)
	}
	if out.Object != "response" {
		t.Errorf("expected object=response, got %q", out.Object)
	}
	if out.Status != "completed" {
		t.Errorf("expected status=completed, got %q", out.Status)
	}
	if len(out.Output) != 1 {
		t.Fatalf("expected 1 output, got %d", len(out.Output))
	}
	if out.Output[0].Type != "message" {
		t.Errorf("expected output type=message, got %q", out.Output[0].Type)
	}
	if len(out.Output[0].Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(out.Output[0].Content))
	}
	if out.Output[0].Content[0].Type != "output_text" {
		t.Errorf("expected content type=output_text, got %q", out.Output[0].Content[0].Type)
	}
	if out.Output[0].Content[0].Text != "Hello world" {
		t.Errorf("expected text=Hello world, got %q", out.Output[0].Content[0].Text)
	}
	if out.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens=10, got %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens=5, got %d", out.Usage.OutputTokens)
	}
	if out.Usage.TotalTokens != 15 {
		t.Errorf("expected total_tokens=15, got %d", out.Usage.TotalTokens)
	}
}

func TestOpenAIToResponsesAPI_EmptyChoices(t *testing.T) {
	resp := &OpenAIResponse{
		ID:    "chatcmpl-456",
		Model: "qwen-max",
	}
	out := OpenAIToResponsesAPI(resp, "qwen-max")
	if len(out.Output) != 1 {
		t.Fatalf("expected 1 output, got %d", len(out.Output))
	}
	if out.Output[0].Content[0].Text != "" {
		t.Errorf("expected empty text, got %q", out.Output[0].Content[0].Text)
	}
}

func TestOpenAIToResponsesAPI_RequestModel(t *testing.T) {
	resp := &OpenAIResponse{
		ID:    "chatcmpl-789",
		Model: "upstream-model",
		Choices: []OpenAIChoice{
			{Message: OpenAIMessage{Content: "ok"}},
		},
	}
	out := OpenAIToResponsesAPI(resp, "request-model")
	if out.Model != "request-model" {
		t.Errorf("expected model=request-model, got %q", out.Model)
	}
}

func TestOpenAIToResponsesAPI_ToolCalls(t *testing.T) {
	resp := &OpenAIResponse{
		ID:    "chatcmpl-tc1",
		Model: "qwen-max",
		Choices: []OpenAIChoice{
			{
				Message: OpenAIMessage{
					Role: "assistant",
					ToolCalls: []OpenAIToolCall{
						{ID: "call_abc", Type: "function", Function: OpenAIFunctionCall{Name: "bash", Arguments: `{"command":"ls"}`}},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: OpenAIUsage{PromptTokens: 20, CompletionTokens: 8},
	}
	out := OpenAIToResponsesAPI(resp, "qwen-max")
	if len(out.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(out.Output))
	}
	item := out.Output[0]
	if item.Type != "function_call" {
		t.Errorf("expected type=function_call, got %q", item.Type)
	}
	if item.ID != "call_abc" {
		t.Errorf("expected id=call_abc, got %q", item.ID)
	}
	if item.Name != "bash" {
		t.Errorf("expected name=bash, got %q", item.Name)
	}
	if item.Arguments != `{"command":"ls"}` {
		t.Errorf("unexpected arguments: %q", item.Arguments)
	}
}
