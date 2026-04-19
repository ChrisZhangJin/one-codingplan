package translator

import (
	"encoding/json"
	"fmt"
)

// AnthropicToOpenAI converts an Anthropic Messages API request to an OpenAI
// Chat Completions request. modelOverride sets the model field (D-01); if
// empty, the model field is omitted from the output (D-02).
func AnthropicToOpenAI(req *AnthropicRequest, modelOverride string) (*OpenAIRequest, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	out := &OpenAIRequest{
		MaxTokens: maxTokens,
		Stream:    req.Stream,
	}

	if modelOverride != "" {
		out.Model = modelOverride
	}

	msgs := make([]OpenAIMessage, 0, len(req.Messages)+1)
	if systemText := extractSystemText(req.System); systemText != "" {
		msgs = append(msgs, OpenAIMessage{Role: "system", Content: systemText})
	}

	for _, m := range req.Messages {
		translated, err := translateMessage(m)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, translated...)
	}
	out.Messages = msgs

	if len(req.Tools) > 0 {
		out.Tools = translateTools(req.Tools)
	}

	return out, nil
}

// translateMessage converts one AnthropicMessage to zero or more OpenAIMessages.
// A single Anthropic message can expand to multiple OpenAI messages when it
// contains tool_result blocks (each becomes a separate role:tool message).
func translateMessage(m AnthropicMessage) ([]OpenAIMessage, error) {
	switch v := m.Content.(type) {
	case string:
		return []OpenAIMessage{{Role: m.Role, Content: v}}, nil
	case []interface{}:
		return translateContentBlocks(m.Role, v)
	default:
		// Re-marshal and try to decode as blocks array.
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("translator: cannot marshal message content: %w", err)
		}
		var blocks []interface{}
		if err := json.Unmarshal(raw, &blocks); err != nil {
			// Fallback: treat as string.
			return []OpenAIMessage{{Role: m.Role, Content: string(raw)}}, nil
		}
		return translateContentBlocks(m.Role, blocks)
	}
}

// translateContentBlocks converts a content block array from an AnthropicMessage
// into the appropriate OpenAI message(s).
func translateContentBlocks(role string, blocks []interface{}) ([]OpenAIMessage, error) {
	var msgs []OpenAIMessage
	var textParts []string
	var toolCalls []OpenAIToolCall

	for _, raw := range blocks {
		blockJSON, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("translator: marshal content block: %w", err)
		}
		var block AnthropicContentBlock
		if err := json.Unmarshal(blockJSON, &block); err != nil {
			return nil, fmt.Errorf("translator: unmarshal content block: %w", err)
		}

		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			arguments := string(block.Input)
			if arguments == "" {
				arguments = "{}"
			}
			toolCalls = append(toolCalls, OpenAIToolCall{
				ID:   block.ID,
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      block.Name,
					Arguments: arguments,
				},
			})
		case "tool_result":
			// Each tool_result becomes a separate role:tool message (D-07).
			content := extractToolResultContent(block)
			msgs = append(msgs, OpenAIMessage{
				Role:       "tool",
				ToolCallID: block.ToolUseID,
				Content:    content,
			})
		}
	}

	// Flush accumulated text and tool_calls into an assistant/user message.
	if len(textParts) > 0 || len(toolCalls) > 0 {
		var combined string
		for _, t := range textParts {
			combined += t
		}
		msg := OpenAIMessage{Role: role, Content: combined}
		if len(toolCalls) > 0 {
			msg.ToolCalls = toolCalls
		}
		msgs = append([]OpenAIMessage{msg}, msgs...)
	} else if len(msgs) == 0 {
		// Empty content array — emit an empty message so the messages array is not missing.
		msgs = append(msgs, OpenAIMessage{Role: role})
	}

	return msgs, nil
}

// extractToolResultContent extracts the string content from a tool_result block.
// The content field can be a string or an array of text blocks.
func extractToolResultContent(block AnthropicContentBlock) string {
	if block.Content == nil {
		return ""
	}
	switch v := block.Content.(type) {
	case string:
		return v
	case []interface{}:
		var result string
		for _, raw := range v {
			blockJSON, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var tb AnthropicContentBlock
			if err := json.Unmarshal(blockJSON, &tb); err != nil {
				continue
			}
			if tb.Type == "text" {
				result += tb.Text
			}
		}
		return result
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

// extractSystemText handles the Anthropic system field which can be either a
// plain string or an array of content blocks (e.g. [{"type":"text","text":"..."}]).
func extractSystemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of content blocks.
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var result string
	for _, b := range blocks {
		if b.Type == "text" {
			result += b.Text
		}
	}
	return result
}

// translateTools converts Anthropic tool definitions to OpenAI tool definitions.
func translateTools(tools []AnthropicTool) []OpenAITool {
	out := make([]OpenAITool, len(tools))
	for i, t := range tools {
		out[i] = OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}
	return out
}
