package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// accumToolCall holds the accumulated state for a single streaming tool call.
type accumToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

// ResponsesStreamTranslator translates OpenAI SSE chunks to Responses API SSE events.
// It is stateful: tracks whether opening events have been emitted, accumulates text
// for done events, maintains a monotonic sequence_number counter, and accumulates
// tool call arguments across streaming chunks.
type ResponsesStreamTranslator struct {
	model       string
	responseID  string
	messageID   string
	started     bool // text message output started
	seqNum      int
	accumulated []byte
	buf         []byte
	// tool call tracking
	toolCalls    map[int]*accumToolCall
	toolIdxToOut map[int]int // maps tool_calls stream index → output_index
	nextOutIdx   int         // monotonically increments per tool call item
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

	var events [][]byte

	// Handle tool call deltas
	for _, tc := range chunk.Choices[0].Delta.ToolCalls {
		tcEvents, err := t.handleToolCallDelta(tc)
		if err != nil {
			return nil, err
		}
		events = append(events, tcEvents...)
	}

	// Handle text content
	content := chunk.Choices[0].Delta.Content
	finishReason := chunk.Choices[0].FinishReason

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

// handleToolCallDelta processes a single tool call delta entry from an OpenAI streaming chunk.
func (t *ResponsesStreamTranslator) handleToolCallDelta(tc openAIToolCallDelta) ([][]byte, error) {
	if t.toolCalls == nil {
		t.toolCalls = make(map[int]*accumToolCall)
		t.toolIdxToOut = make(map[int]int)
	}

	var events [][]byte
	isNew := false

	accum, exists := t.toolCalls[tc.Index]
	if !exists {
		isNew = true
		accum = &accumToolCall{}
		t.toolCalls[tc.Index] = accum
		t.toolIdxToOut[tc.Index] = t.nextOutIdx
		t.nextOutIdx++
	}
	outIdx := t.toolIdxToOut[tc.Index]

	if tc.ID != "" {
		accum.id = tc.ID
	}
	if tc.Function.Name != "" {
		accum.name = tc.Function.Name
	}

	// Emit output_item.added on first delta (once we have an id)
	if isNew && accum.id != "" {
		itemAdded, err := formatSSEEvent("response.output_item.added", map[string]interface{}{
			"type":         "response.output_item.added",
			"output_index": outIdx,
			"item": map[string]interface{}{
				"type":      "function_call",
				"id":        accum.id,
				"status":    "in_progress",
				"name":      accum.name,
				"arguments": "",
			},
			"sequence_number": t.nextSeq(),
		})
		if err != nil {
			return nil, err
		}
		events = append(events, itemAdded)
	}

	// Emit arguments delta if arguments present
	if tc.Function.Arguments != "" {
		accum.arguments.WriteString(tc.Function.Arguments)

		argsDelta, err := formatSSEEvent("response.function_call_arguments.delta", map[string]interface{}{
			"type":            "response.function_call_arguments.delta",
			"item_id":         accum.id,
			"output_index":    outIdx,
			"delta":           tc.Function.Arguments,
			"sequence_number": t.nextSeq(),
		})
		if err != nil {
			return nil, err
		}
		events = append(events, argsDelta)
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

// emitClosing emits the appropriate closing event sequence.
// If tool calls were accumulated, emits tool call completion events.
// Otherwise emits text message completion events.
func (t *ResponsesStreamTranslator) emitClosing() ([][]byte, error) {
	if len(t.toolCalls) > 0 {
		return t.emitToolCallClosing()
	}
	return t.emitTextClosing()
}

func (t *ResponsesStreamTranslator) emitTextClosing() ([][]byte, error) {
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

// emitToolCallClosing emits done events for all accumulated tool calls plus response.completed.
func (t *ResponsesStreamTranslator) emitToolCallClosing() ([][]byte, error) {
	// Sort tool call indices for deterministic output order
	indices := make([]int, 0, len(t.toolCalls))
	for idx := range t.toolCalls {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	var events [][]byte
	var outputItems []interface{}

	for _, idx := range indices {
		accum := t.toolCalls[idx]
		outIdx := t.toolIdxToOut[idx]
		args := accum.arguments.String()

		argsDone, err := formatSSEEvent("response.function_call_arguments.done", map[string]interface{}{
			"type":            "response.function_call_arguments.done",
			"item_id":         accum.id,
			"output_index":    outIdx,
			"arguments":       args,
			"sequence_number": t.nextSeq(),
		})
		if err != nil {
			return nil, err
		}

		itemDone, err := formatSSEEvent("response.output_item.done", map[string]interface{}{
			"type":         "response.output_item.done",
			"output_index": outIdx,
			"item": map[string]interface{}{
				"type":      "function_call",
				"id":        accum.id,
				"status":    "completed",
				"name":      accum.name,
				"arguments": args,
			},
			"sequence_number": t.nextSeq(),
		})
		if err != nil {
			return nil, err
		}

		events = append(events, argsDone, itemDone)
		outputItems = append(outputItems, map[string]interface{}{
			"type":      "function_call",
			"id":        accum.id,
			"status":    "completed",
			"name":      accum.name,
			"arguments": args,
		})
	}

	completed, err := formatSSEEvent("response.completed", map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id":     t.responseID,
			"object": "response",
			"status": "completed",
			"model":  t.model,
			"output": outputItems,
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
	events = append(events, completed)

	return events, nil
}

// Ensure fmt is used (for formatSSEEvent usage pattern matching stream.go).
var _ = fmt.Sprintf
