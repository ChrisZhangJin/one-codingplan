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
		content := m.ContentText()
		if content == "" {
			continue
		}
		role := m.Role
		switch role {
		case "", "developer":
			role = "system"
		}
		messages = append(messages, OpenAIMessage{
			Role:    role,
			Content: content,
		})
	}

	return OpenAIRequest{
		Model:    model,
		Messages: mergeSystemMessages(messages),
		Stream:   req.Stream,
	}
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
