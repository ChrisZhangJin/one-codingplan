package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// StreamTranslator is a stateful SSE translator that accepts raw OpenAI upstream bytes
// and emits complete Anthropic SSE event byte slices, handling partial frame buffering.
type StreamTranslator struct {
	model   string
	started bool
	buf     []byte
}

// NewStreamTranslator creates a StreamTranslator that echoes originalModel in message_start (D-03).
func NewStreamTranslator(originalModel string) *StreamTranslator {
	return &StreamTranslator{model: originalModel}
}

// Translate accepts raw upstream bytes and returns 0+ complete Anthropic SSE event byte slices.
func (st *StreamTranslator) Translate(chunk []byte) ([][]byte, error) {
	st.buf = append(st.buf, chunk...)
	var events [][]byte
	for {
		idx := bytes.Index(st.buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		frame := st.buf[:idx]
		st.buf = st.buf[idx+2:]
		emitted, err := st.translateFrame(frame)
		if err != nil {
			return nil, err
		}
		events = append(events, emitted...)
	}
	return events, nil
}

// openAIToolCallDelta is a single tool call entry within a streaming delta.
type openAIToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openAIStreamChunk is the minimal shape of an OpenAI streaming chunk.
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string                `json:"content"`
			ToolCalls []openAIToolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (st *StreamTranslator) translateFrame(frame []byte) ([][]byte, error) {
	// Strip leading/trailing whitespace lines and extract the data line.
	line := bytes.TrimSpace(frame)
	// Handle multi-line frames: find the "data: " line.
	for _, l := range bytes.Split(line, []byte("\n")) {
		l = bytes.TrimSpace(l)
		if bytes.HasPrefix(l, []byte("data:")) {
			line = bytes.TrimSpace(l[5:])
			break
		}
	}

	// Handle [DONE] sentinel.
	if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
		return st.emitClosing("end_turn")
	}

	// Parse the OpenAI streaming chunk.
	var chunk openAIStreamChunk
	if err := json.Unmarshal(line, &chunk); err != nil {
		// Skip unparseable frames (T-4-06: malformed SSE frames).
		return nil, nil
	}
	if len(chunk.Choices) == 0 {
		return nil, nil
	}

	content := chunk.Choices[0].Delta.Content
	finishReason := chunk.Choices[0].FinishReason

	var events [][]byte

	// If content is non-empty and we haven't started yet, emit opening events.
	if content != "" && !st.started {
		msgStart, err := formatSSEEvent("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":            "msg_proxy",
				"type":          "message",
				"role":          "assistant",
				"content":       []interface{}{},
				"model":         st.model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage":         map[string]int{"input_tokens": 0, "output_tokens": 0},
			},
		})
		if err != nil {
			return nil, err
		}
		blockStart, err := formatSSEEvent("content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": 0,
			"content_block": map[string]interface{}{
				"type": "text",
				"text": "",
			},
		})
		if err != nil {
			return nil, err
		}
		events = append(events, msgStart, blockStart)
		st.started = true
	}

	// Emit content_block_delta if content is non-empty.
	if content != "" {
		delta, err := formatSSEEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": content,
			},
		})
		if err != nil {
			return nil, err
		}
		events = append(events, delta)
	}

	// If finish_reason is present and non-empty, emit closing sequence.
	if finishReason != nil && *finishReason != "" {
		stopReason := translateFinishReason(*finishReason)
		closing, err := st.emitClosing(stopReason)
		if err != nil {
			return nil, err
		}
		events = append(events, closing...)
	}

	return events, nil
}

func (st *StreamTranslator) emitClosing(stopReason string) ([][]byte, error) {
	blockStop, err := formatSSEEvent("content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})
	if err != nil {
		return nil, err
	}
	msgDelta, err := formatSSEEvent("message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{"output_tokens": 0},
	})
	if err != nil {
		return nil, err
	}
	msgStop, err := formatSSEEvent("message_stop", map[string]interface{}{
		"type": "message_stop",
	})
	if err != nil {
		return nil, err
	}
	return [][]byte{blockStop, msgDelta, msgStop}, nil
}

func formatSSEEvent(eventType string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}
