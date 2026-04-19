package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ResponsesStreamTranslator translates OpenAI SSE chunks to Responses API SSE events.
// It is stateful: tracks whether opening events have been emitted, accumulates text
// for done events, and maintains a monotonic sequence_number counter.
type ResponsesStreamTranslator struct {
	model      string
	responseID string
	messageID  string
	started    bool
	seqNum     int
	accumulated []byte
	buf        []byte
}

// NewResponsesStreamTranslator creates a translator for converting OpenAI SSE chunks
// to Responses API SSE event sequences.
func NewResponsesStreamTranslator(model string) *ResponsesStreamTranslator {
	return &ResponsesStreamTranslator{
		model:      model,
		responseID: "resp_proxy",
		messageID:  "msg_proxy",
	}
}

// Translate accepts raw upstream bytes and returns 0+ complete Responses API SSE event byte slices.
func (t *ResponsesStreamTranslator) Translate(chunk []byte) ([][]byte, error) {
	t.buf = append(t.buf, chunk...)
	var events [][]byte
	for {
		idx := bytes.Index(t.buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		frame := t.buf[:idx]
		t.buf = t.buf[idx+2:]
		emitted, err := t.translateFrame(frame)
		if err != nil {
			return nil, err
		}
		events = append(events, emitted...)
	}
	return events, nil
}

func (t *ResponsesStreamTranslator) nextSeq() int {
	n := t.seqNum
	t.seqNum++
	return n
}

func (t *ResponsesStreamTranslator) translateFrame(frame []byte) ([][]byte, error) {
	line := bytes.TrimSpace(frame)
	for _, l := range bytes.Split(line, []byte("\n")) {
		l = bytes.TrimSpace(l)
		if bytes.HasPrefix(l, []byte("data:")) {
			line = bytes.TrimSpace(l[5:])
			break
		}
	}

	if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
		return t.emitClosing()
	}

	var chunk openAIStreamChunk
	if err := json.Unmarshal(line, &chunk); err != nil {
		return nil, nil
	}
	if len(chunk.Choices) == 0 {
		return nil, nil
	}

	content := chunk.Choices[0].Delta.Content
	finishReason := chunk.Choices[0].FinishReason

	var events [][]byte

	if content != "" && !t.started {
		opening, err := t.emitOpening()
		if err != nil {
			return nil, err
		}
		events = append(events, opening...)
		t.started = true
	}

	if content != "" {
		delta, err := t.emitDelta(content)
		if err != nil {
			return nil, err
		}
		events = append(events, delta)
		t.accumulated = append(t.accumulated, []byte(content)...)
	}

	if finishReason != nil && *finishReason != "" {
		closing, err := t.emitClosing()
		if err != nil {
			return nil, err
		}
		events = append(events, closing...)
	}

	return events, nil
}

func (t *ResponsesStreamTranslator) emitOpening() ([][]byte, error) {
	created, err := formatSSEEvent("response.created", map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id":     t.responseID,
			"object": "response",
			"status": "in_progress",
			"model":  t.model,
			"output": []interface{}{},
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	itemAdded, err := formatSSEEvent("response.output_item.added", map[string]interface{}{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item": map[string]interface{}{
			"id":      t.messageID,
			"type":    "message",
			"status":  "in_progress",
			"role":    "assistant",
			"content": []interface{}{},
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	partAdded, err := formatSSEEvent("response.content_part.added", map[string]interface{}{
		"type":          "response.content_part.added",
		"item_id":       t.messageID,
		"output_index":  0,
		"content_index": 0,
		"part": map[string]interface{}{
			"type":        "output_text",
			"text":        "",
			"annotations": []interface{}{},
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	return [][]byte{created, itemAdded, partAdded}, nil
}

func (t *ResponsesStreamTranslator) emitDelta(text string) ([]byte, error) {
	return formatSSEEvent("response.output_text.delta", map[string]interface{}{
		"type":            "response.output_text.delta",
		"item_id":         t.messageID,
		"output_index":    0,
		"content_index":   0,
		"delta":           text,
		"sequence_number": t.nextSeq(),
	})
}

func (t *ResponsesStreamTranslator) emitClosing() ([][]byte, error) {
	fullText := string(t.accumulated)

	textDone, err := formatSSEEvent("response.output_text.done", map[string]interface{}{
		"type":            "response.output_text.done",
		"item_id":         t.messageID,
		"output_index":    0,
		"content_index":   0,
		"text":            fullText,
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	partDone, err := formatSSEEvent("response.content_part.done", map[string]interface{}{
		"type":          "response.content_part.done",
		"item_id":       t.messageID,
		"output_index":  0,
		"content_index": 0,
		"part": map[string]interface{}{
			"type":        "output_text",
			"text":        fullText,
			"annotations": []interface{}{},
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	itemDone, err := formatSSEEvent("response.output_item.done", map[string]interface{}{
		"type":         "response.output_item.done",
		"output_index": 0,
		"item": map[string]interface{}{
			"id":     t.messageID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type":        "output_text",
					"text":        fullText,
					"annotations": []interface{}{},
				},
			},
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	completed, err := formatSSEEvent("response.completed", map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id":     t.responseID,
			"object": "response",
			"status": "completed",
			"model":  t.model,
			"output": []interface{}{
				map[string]interface{}{
					"type":   "message",
					"id":     t.messageID,
					"status": "completed",
					"role":   "assistant",
					"content": []interface{}{
						map[string]interface{}{
							"type":        "output_text",
							"text":        fullText,
							"annotations": []interface{}{},
						},
					},
				},
			},
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
				"total_tokens":  0,
			},
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	return [][]byte{textDone, partDone, itemDone, completed}, nil
}

// Ensure fmt is used (for formatSSEEvent usage pattern matching stream.go).
var _ = fmt.Sprintf
