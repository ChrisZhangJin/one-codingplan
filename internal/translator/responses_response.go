package translator

import "time"

// OpenAIToResponsesAPI wraps an OpenAI chat completions response in Responses API format.
// requestModel is the model from the original request (used in the response envelope).
// If the response contains tool calls, they are returned as function_call output items.
func OpenAIToResponsesAPI(resp *OpenAIResponse, requestModel string) *ResponsesResponse {
	model := requestModel
	if model == "" {
		model = resp.Model
	}

	responseID := "resp_" + resp.ID
	messageID := "msg_" + resp.ID

	var output []ResponsesOutput
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				output = append(output, ResponsesOutput{
					Type:      "function_call",
					ID:        tc.ID,
					Status:    "completed",
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
		} else {
			output = []ResponsesOutput{{
				Type:   "message",
				ID:     messageID,
				Status: "completed",
				Role:   "assistant",
				Content: []ResponsesContentPart{{
					Type: "output_text",
					Text: choice.Message.Content,
				}},
			}}
		}
	} else {
		output = []ResponsesOutput{{
			Type:   "message",
			ID:     messageID,
			Status: "completed",
			Role:   "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: "",
			}},
		}}
	}

	return &ResponsesResponse{
		ID:        responseID,
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "completed",
		Model:     model,
		Output:    output,
		Usage: ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		},
	}
}
