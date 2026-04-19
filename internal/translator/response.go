package translator

import (
	"encoding/json"
	"fmt"
)

// OpenAIToAnthropic converts a non-streaming OpenAI response to Anthropic format.
// originalModel is echoed back as the response model field (D-03).
func OpenAIToAnthropic(resp *OpenAIResponse, originalModel string) (*AnthropicResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in upstream response")
	}
	ch := resp.Choices[0]

	out := &AnthropicResponse{
		ID:    resp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: originalModel, // D-03: echo client's requested model name
	}

	var content []AnthropicContentBlock
	if ch.Message.Content != "" {
		content = append(content, AnthropicContentBlock{
			Type: "text",
			Text: ch.Message.Content,
		})
	}
	for _, tc := range ch.Message.ToolCalls {
		content = append(content, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    tc.ID, // D-07: preserve tool_call_id
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}
	out.Content = content
	out.StopReason = translateFinishReason(ch.FinishReason)
	out.Usage = AnthropicUsage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}
	return out, nil
}

func translateFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
	}
}
