package translator

import (
	"encoding/json"
	"errors"
	"strings"
)

var ErrStringInput = errors.New("input must be an array, not a string")

// ParseResponsesInput validates and parses the raw JSON input field.
// Returns ErrStringInput if the input is a JSON string (per D-02).
func ParseResponsesInput(raw json.RawMessage) ([]ResponsesInputMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	trimmed := raw
	for i, b := range trimmed {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		trimmed = trimmed[i:]
		break
	}
	if len(trimmed) > 0 && trimmed[0] == '"' {
		return nil, ErrStringInput
	}
	var msgs []ResponsesInputMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// ResponsesRequestToOpenAI translates a parsed Responses API request to an OpenAI chat completions request.
// If modelOverride is non-empty, it replaces the model field.
// The instructions field is prepended as a system message (per D-03).
// Function-type tools are forwarded; built-in tool types are silently dropped.
func ResponsesRequestToOpenAI(req *ResponsesRequest, msgs []ResponsesInputMessage, modelOverride string) OpenAIRequest {
	model := req.Model
	if modelOverride != "" {
		model = modelOverride
	}

	var messages []OpenAIMessage

	if req.Instructions != "" {
		messages = append(messages, OpenAIMessage{
			Role:    "system",
			Content: req.Instructions,
		})
	}

	for _, m := range msgs {
		messages = append(messages, convertResponsesItem(m)...)
	}

	var tools []OpenAITool
	for _, t := range req.Tools {
		if t.Type == "function" {
			tools = append(tools, OpenAITool{
				Type: "function",
				Function: OpenAIFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			})
		}
		// Skip non-function built-in tools (computer_use_preview, web_search_preview, etc.)
	}

	return OpenAIRequest{
		Model:    model,
		Messages: mergeSystemMessages(messages),
		Stream:   req.Stream,
		Tools:    tools,
	}
}

// convertResponsesItem converts a single Responses API input item to one or more OpenAI messages.
// Items may be role-based messages (user/assistant/system) or typed items
// (function_call for assistant tool calls, function_call_output for tool results).
func convertResponsesItem(m ResponsesInputMessage) []OpenAIMessage {
	switch m.Type {
	case "function_call":
		// Assistant's tool call stored in conversation history
		return []OpenAIMessage{{
			Role: "assistant",
			ToolCalls: []OpenAIToolCall{{
				ID:   m.ID,
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      m.Name,
					Arguments: m.Arguments,
				},
			}},
		}}
	case "function_call_output":
		// Tool execution result
		return []OpenAIMessage{{
			Role:       "tool",
			ToolCallID: m.CallID,
			Content:    m.Output,
		}}
	}

	// Role-based message
	role := m.Role
	switch role {
	case "", "developer":
		role = "system"
	}

	if len(m.Content) == 0 {
		return nil
	}

	// String content: "hello world"
	if m.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil && s != "" {
			return []OpenAIMessage{{Role: role, Content: s}}
		}
	}

	// Array content: may be plain text parts or tool_result parts
	if m.Content[0] == '[' {
		var parts []ResponsesContentPart
		if err := json.Unmarshal(m.Content, &parts); err == nil {
			return convertContentParts(role, parts)
		}
	}

	return nil
}

// convertContentParts converts an array of content parts to OpenAI messages.
// tool_result parts become separate "tool" role messages; text parts are merged
// into a single message under the original role.
func convertContentParts(role string, parts []ResponsesContentPart) []OpenAIMessage {
	var msgs []OpenAIMessage
	var textBuf []string

	flushText := func() {
		if len(textBuf) > 0 {
			msgs = append(msgs, OpenAIMessage{Role: role, Content: strings.Join(textBuf, "")})
			textBuf = nil
		}
	}

	for _, p := range parts {
		switch p.Type {
		case "tool_result":
			flushText()
			callID := p.ToolUseID
			if callID == "" {
				callID = p.CallID
			}
			msgs = append(msgs, OpenAIMessage{
				Role:       "tool",
				ToolCallID: callID,
				Content:    p.ContentText(),
			})
		case "input_text", "text", "output_text":
			textBuf = append(textBuf, p.Text)
		}
	}
	flushText()

	return msgs
}

// mergeSystemMessages collapses consecutive system messages into one,
// joining their content with newlines. Providers like Minimax reject multiple system messages.
func mergeSystemMessages(msgs []OpenAIMessage) []OpenAIMessage {
	var out []OpenAIMessage
	for _, m := range msgs {
		if m.Role == "system" && len(out) > 0 && out[len(out)-1].Role == "system" {
			out[len(out)-1].Content = strings.Join([]string{out[len(out)-1].Content, m.Content}, "\n\n")
		} else {
			out = append(out, m)
		}
	}
	return out
}
