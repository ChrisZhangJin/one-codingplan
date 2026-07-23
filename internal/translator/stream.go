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
	closed  bool
	buf     []byte
	// textIndex is the Anthropic content_block index used for the assistant text
	// block, or -1 when no text block has been opened yet.
	textIndex int
	textOpen  bool
	// toolIndexes maps an OpenAI tool_calls[].index to the Anthropic content_block
	// index that was opened for that tool. Entries persist until message close.
	toolIndexes map[int]int
	// toolOpen tracks whether the Anthropic tool_use block for an OpenAI tool_call
	// index is currently open (between content_block_start and content_block_stop).
	toolOpen map[int]bool
	// nextIndex is the next Anthropic content_block index to assign.
	nextIndex int
}

// NewStreamTranslator creates a StreamTranslator that echoes originalModel in message_start (D-03).
func NewStreamTranslator(originalModel string) *StreamTranslator {
	return &StreamTranslator{
		model:       originalModel,
		textIndex:   -1,
		toolIndexes: make(map[int]int),
		toolOpen:    make(map[int]bool),
	}
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

	var chunk openAIStreamChunk
	if err := json.Unmarshal(line, &chunk); err != nil {
		return nil, nil
	}
	if len(chunk.Choices) == 0 {
		return nil, nil
	}

	delta := chunk.Choices[0].Delta
	finishReason := chunk.Choices[0].FinishReason

	var events [][]byte

	// message_start — emitted lazily on the first chunk that carries either
	// text content or a tool_call. message_start MUST precede every other event.
	if !st.started && (delta.Content != "" || len(delta.ToolCalls) > 0) {
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
		events = append(events, msgStart)
		st.started = true
	}

	// Text delta: open text block on first non-empty content, then emit deltas.
	if delta.Content != "" {
		if !st.textOpen {
			st.textIndex = st.nextIndex
			st.nextIndex++
			blockStart, err := formatSSEEvent("content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": st.textIndex,
				"content_block": map[string]interface{}{
					"type": "text",
					"text": "",
				},
			})
			if err != nil {
				return nil, err
			}
			events = append(events, blockStart)
			st.textOpen = true
		}
		blockDelta, err := formatSSEEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": st.textIndex,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": delta.Content,
			},
		})
		if err != nil {
			return nil, err
		}
		events = append(events, blockDelta)
	}

	// Tool-call deltas. Each tool_call has its own .index in the OpenAI stream;
	// we map that to an Anthropic content_block index.
	for _, tc := range delta.ToolCalls {
		// Close the text block (if open) before opening the first tool_use block
		// at a higher index — Anthropic streams one block at a time per index.
		if st.textOpen {
			stopEv, err := formatSSEEvent("content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": st.textIndex,
			})
			if err != nil {
				return nil, err
			}
			events = append(events, stopEv)
			st.textOpen = false
		}

		anthIdx, exists := st.toolIndexes[tc.Index]
		if !exists {
			anthIdx = st.nextIndex
			st.nextIndex++
			st.toolIndexes[tc.Index] = anthIdx
		}
		if !st.toolOpen[tc.Index] {
			blockStart, err := formatSSEEvent("content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": anthIdx,
				"content_block": map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": map[string]interface{}{},
				},
			})
			if err != nil {
				return nil, err
			}
			events = append(events, blockStart)
			st.toolOpen[tc.Index] = true
		}
		if tc.Function.Arguments != "" {
			blockDelta, err := formatSSEEvent("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": anthIdx,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": tc.Function.Arguments,
				},
			})
			if err != nil {
				return nil, err
			}
			events = append(events, blockDelta)
		}
	}

	// Close on finish_reason.
	if finishReason != nil && *finishReason != "" {
		closing, err := st.emitClosing(translateFinishReason(*finishReason))
		if err != nil {
			return nil, err
		}
		events = append(events, closing...)
	}

	return events, nil
}

func (st *StreamTranslator) emitClosing(stopReason string) ([][]byte, error) {
	if st.closed {
		return nil, nil
	}
	st.closed = true

	var events [][]byte

	// Close any still-open text block.
	if st.textOpen {
		stopEv, err := formatSSEEvent("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": st.textIndex,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, stopEv)
		st.textOpen = false
	}
	// Close any still-open tool_use blocks, in OpenAI-tool-index order.
	for openAIIdx, anthIdx := range st.toolIndexes {
		if !st.toolOpen[openAIIdx] {
			continue
		}
		stopEv, err := formatSSEEvent("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": anthIdx,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, stopEv)
		st.toolOpen[openAIIdx] = false
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
	events = append(events, msgDelta, msgStop)
	return events, nil
}

func formatSSEEvent(eventType string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}
