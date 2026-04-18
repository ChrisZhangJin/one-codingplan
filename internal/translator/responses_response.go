package translator

import "time"

// OpenAIToResponsesAPI wraps an OpenAI chat completions response in Responses API format.
// requestModel is the model from the original request (used in the response envelope).
func OpenAIToResponsesAPI(resp *OpenAIResponse, requestModel string) *ResponsesResponse {
	model := requestModel
	if model == "" {
		model = resp.Model
	}

	text := ""
	if len(resp.Choices) > 0 {
		text = resp.Choices[0].Message.Content
	}

	responseID := "resp_" + resp.ID
	messageID := "msg_" + resp.ID

	return &ResponsesResponse{
		ID:        responseID,
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "completed",
		Model:     model,
		Output: []ResponsesOutput{
			{
				Type:   "message",
				ID:     messageID,
				Status: "completed",
				Role:   "assistant",
				Content: []ResponsesContentPart{
					{
						Type: "output_text",
						Text: text,
					},
				},
			},
		},
		Usage: ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		},
	}
}
