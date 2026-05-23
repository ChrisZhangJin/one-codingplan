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
	model         string
	responseID    string
	messageID     string
	reasoningID   string
	started       bool // text message output started
	messageOutIdx int  // output_index assigned to the message item when text starts
	seqNum        int
	accumulated   []byte
	buf           []byte
	// tool call tracking
	toolCalls    map[int]*accumToolCall
	toolIdxToOut map[int]int // maps tool_calls stream index → output_index
	nextOutIdx   int         // monotonically increments per output item (reasoning, message, tool calls)
	closed       bool        // true after emitClosing has run; guards against duplicate finalization
	createdSent  bool        // true once response.created has been emitted
	// reasoning (<think> ... </think>) tracking
	reasoningStarted bool            // true after reasoning output_item.added emitted
	reasoningDone    bool            // true after reasoning output_item.done emitted
	reasoningOutIdx  int             // output_index assigned to the reasoning item
	reasoningText    strings.Builder // accumulated reasoning text
	inThink          bool            // currently inside <think>...</think>
	thinkPending     strings.Builder // bytes that might be the start of a tag boundary; carry across chunks
}

// NewResponsesStreamTranslator creates a translator for converting OpenAI SSE chunks
// to Responses API SSE event sequences.
func NewResponsesStreamTranslator(model string) *ResponsesStreamTranslator {
	return &ResponsesStreamTranslator{
		model:       model,
		responseID:  "resp_proxy",
		messageID:   "msg_proxy",
		reasoningID: "rs_proxy",
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

	// Handle text content
	content := chunk.Choices[0].Delta.Content
	finishReason := chunk.Choices[0].FinishReason

	// response.created must precede any other event in the stream.
	// Emit it on the first chunk that carries either tool calls or text.
	hasToolDelta := len(chunk.Choices[0].Delta.ToolCalls) > 0
	if !t.createdSent && (hasToolDelta || content != "") {
		created, err := t.emitCreated()
		if err != nil {
			return nil, err
		}
		events = append(events, created)
		t.createdSent = true
	}

	// Handle tool call deltas
	for _, tc := range chunk.Choices[0].Delta.ToolCalls {
		tcEvents, err := t.handleToolCallDelta(tc)
		if err != nil {
			return nil, err
		}
		events = append(events, tcEvents...)
	}

	if content != "" {
		contentEvents, err := t.processContent(content)
		if err != nil {
			return nil, err
		}
		events = append(events, contentEvents...)
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
				"call_id":   accum.id,
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

func (t *ResponsesStreamTranslator) emitCreated() ([]byte, error) {
	return formatSSEEvent("response.created", map[string]interface{}{
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
}

func (t *ResponsesStreamTranslator) emitTextOpening() ([][]byte, error) {
	t.messageOutIdx = t.nextOutIdx
	t.nextOutIdx++

	itemAdded, err := formatSSEEvent("response.output_item.added", map[string]interface{}{
		"type":         "response.output_item.added",
		"output_index": t.messageOutIdx,
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
		"output_index":  t.messageOutIdx,
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

	return [][]byte{itemAdded, partAdded}, nil
}

func (t *ResponsesStreamTranslator) emitDelta(text string) ([]byte, error) {
	return formatSSEEvent("response.output_text.delta", map[string]interface{}{
		"type":            "response.output_text.delta",
		"item_id":         t.messageID,
		"output_index":    t.messageOutIdx,
		"content_index":   0,
		"delta":           text,
		"sequence_number": t.nextSeq(),
	})
}

// emitClosing emits the appropriate closing event sequence.
// If tool calls were accumulated, emits tool call completion events.
// Otherwise emits text message completion events.
// Upstreams send both a chunk with finish_reason and a trailing [DONE]; only
// the first one emits — subsequent calls return nothing to avoid duplicate
// response.completed events that confuse SSE state machines on the client.
func (t *ResponsesStreamTranslator) emitClosing() ([][]byte, error) {
	if t.closed {
		return nil, nil
	}
	t.closed = true

	var events [][]byte
	var outputItems []interface{}

	// 1. Close reasoning first if still open (upstream ended mid-think with no </think>).
	if t.reasoningStarted && !t.reasoningDone {
		closeEvts, err := t.emitReasoningClose()
		if err != nil {
			return nil, err
		}
		events = append(events, closeEvts...)
	}
	if t.reasoningStarted {
		outputItems = append(outputItems, t.reasoningOutputItem())
	}

	// 2. Close tool calls or message.
	if len(t.toolCalls) > 0 {
		toolEvts, toolItems, err := t.emitToolCallDoneEvents()
		if err != nil {
			return nil, err
		}
		events = append(events, toolEvts...)
		outputItems = append(outputItems, toolItems...)
	} else if t.started {
		msgEvts, msgItem, err := t.emitMessageDoneEvents()
		if err != nil {
			return nil, err
		}
		events = append(events, msgEvts...)
		outputItems = append(outputItems, msgItem)
	}

	// 3. Emit final response.completed with the assembled output.
	completed, err := t.emitResponseCompleted(outputItems)
	if err != nil {
		return nil, err
	}
	events = append(events, completed)
	return events, nil
}

// emitMessageDoneEvents emits the text/content/item done events for the message
// and returns those events plus the final message output item to include in
// response.completed.
func (t *ResponsesStreamTranslator) emitMessageDoneEvents() ([][]byte, map[string]interface{}, error) {
	fullText := string(t.accumulated)

	textDone, err := formatSSEEvent("response.output_text.done", map[string]interface{}{
		"type":            "response.output_text.done",
		"item_id":         t.messageID,
		"output_index":    t.messageOutIdx,
		"content_index":   0,
		"text":            fullText,
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, nil, err
	}

	partDone, err := formatSSEEvent("response.content_part.done", map[string]interface{}{
		"type":          "response.content_part.done",
		"item_id":       t.messageID,
		"output_index":  t.messageOutIdx,
		"content_index": 0,
		"part": map[string]interface{}{
			"type":        "output_text",
			"text":        fullText,
			"annotations": []interface{}{},
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, nil, err
	}

	msgItem := map[string]interface{}{
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
	}

	itemDone, err := formatSSEEvent("response.output_item.done", map[string]interface{}{
		"type":            "response.output_item.done",
		"output_index":    t.messageOutIdx,
		"item":            msgItem,
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, nil, err
	}

	return [][]byte{textDone, partDone, itemDone}, msgItem, nil
}

// emitToolCallDoneEvents emits arg/item done events for every accumulated tool call
// and returns those events plus the list of function_call output items.
func (t *ResponsesStreamTranslator) emitToolCallDoneEvents() ([][]byte, []interface{}, error) {
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
			return nil, nil, err
		}

		fnItem := map[string]interface{}{
			"type":      "function_call",
			"id":        accum.id,
			"call_id":   accum.id,
			"status":    "completed",
			"name":      accum.name,
			"arguments": args,
		}

		itemDone, err := formatSSEEvent("response.output_item.done", map[string]interface{}{
			"type":            "response.output_item.done",
			"output_index":    outIdx,
			"item":            fnItem,
			"sequence_number": t.nextSeq(),
		})
		if err != nil {
			return nil, nil, err
		}

		events = append(events, argsDone, itemDone)
		outputItems = append(outputItems, fnItem)
	}

	return events, outputItems, nil
}

func (t *ResponsesStreamTranslator) emitResponseCompleted(outputItems []interface{}) ([]byte, error) {
	if outputItems == nil {
		outputItems = []interface{}{}
	}
	return formatSSEEvent("response.completed", map[string]interface{}{
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
}

// --- Inline <think>...</think> handling ---

const (
	thinkOpenTag  = "<think>"
	thinkCloseTag = "</think>"
)

// processContent scans a content delta and dispatches each piece to either the
// message text path or the reasoning path, depending on whether the cursor is
// currently inside a <think>...</think> block. Tag halves split across SSE chunks
// are buffered in t.thinkPending and joined on the next call.
func (t *ResponsesStreamTranslator) processContent(content string) ([][]byte, error) {
	t.thinkPending.WriteString(content)
	buf := t.thinkPending.String()
	t.thinkPending.Reset()

	var events [][]byte
	for {
		if t.inThink {
			if idx := strings.Index(buf, thinkCloseTag); idx >= 0 {
				if idx > 0 {
					events = append(events, t.emitReasoningDelta(buf[:idx]))
				}
				closeEvts, err := t.emitReasoningClose()
				if err != nil {
					return nil, err
				}
				events = append(events, closeEvts...)
				t.inThink = false
				buf = buf[idx+len(thinkCloseTag):]
				continue
			}
			partial := partialTagSuffix(buf, thinkCloseTag)
			safeLen := len(buf) - partial
			if safeLen > 0 {
				events = append(events, t.emitReasoningDelta(buf[:safeLen]))
			}
			if partial > 0 {
				t.thinkPending.WriteString(buf[safeLen:])
			}
			return events, nil
		}

		// Outside a think block.
		if idx := strings.Index(buf, thinkOpenTag); idx >= 0 {
			if idx > 0 {
				textEvts, err := t.emitTextChunk(buf[:idx])
				if err != nil {
					return nil, err
				}
				events = append(events, textEvts...)
			}
			openEvts, err := t.emitReasoningOpen()
			if err != nil {
				return nil, err
			}
			events = append(events, openEvts...)
			t.inThink = true
			buf = buf[idx+len(thinkOpenTag):]
			continue
		}
		partial := partialTagSuffix(buf, thinkOpenTag)
		safeLen := len(buf) - partial
		if safeLen > 0 {
			textEvts, err := t.emitTextChunk(buf[:safeLen])
			if err != nil {
				return nil, err
			}
			events = append(events, textEvts...)
		}
		if partial > 0 {
			t.thinkPending.WriteString(buf[safeLen:])
		}
		return events, nil
	}
}

// partialTagSuffix returns the length of the longest suffix of s that is a strict
// prefix of needle. Used to hold back bytes that may yet complete an opening or
// closing tag across chunk boundaries.
func partialTagSuffix(s, needle string) int {
	maxN := len(needle) - 1
	if maxN > len(s) {
		maxN = len(s)
	}
	for n := maxN; n > 0; n-- {
		if strings.HasPrefix(needle, s[len(s)-n:]) {
			return n
		}
	}
	return 0
}

// emitTextChunk opens the message item if this is the first text, then emits a delta.
func (t *ResponsesStreamTranslator) emitTextChunk(text string) ([][]byte, error) {
	if text == "" {
		return nil, nil
	}
	var events [][]byte
	if !t.started {
		opening, err := t.emitTextOpening()
		if err != nil {
			return nil, err
		}
		events = append(events, opening...)
		t.started = true
	}
	delta, err := t.emitDelta(text)
	if err != nil {
		return nil, err
	}
	events = append(events, delta)
	t.accumulated = append(t.accumulated, []byte(text)...)
	return events, nil
}

func (t *ResponsesStreamTranslator) emitReasoningOpen() ([][]byte, error) {
	t.reasoningOutIdx = t.nextOutIdx
	t.nextOutIdx++
	t.reasoningStarted = true

	itemAdded, err := formatSSEEvent("response.output_item.added", map[string]interface{}{
		"type":         "response.output_item.added",
		"output_index": t.reasoningOutIdx,
		"item": map[string]interface{}{
			"id":      t.reasoningID,
			"type":    "reasoning",
			"summary": []interface{}{},
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	partAdded, err := formatSSEEvent("response.reasoning_summary_part.added", map[string]interface{}{
		"type":          "response.reasoning_summary_part.added",
		"item_id":       t.reasoningID,
		"output_index":  t.reasoningOutIdx,
		"summary_index": 0,
		"part": map[string]interface{}{
			"type": "summary_text",
			"text": "",
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}
	return [][]byte{itemAdded, partAdded}, nil
}

func (t *ResponsesStreamTranslator) emitReasoningDelta(text string) []byte {
	t.reasoningText.WriteString(text)
	ev, _ := formatSSEEvent("response.reasoning_summary_text.delta", map[string]interface{}{
		"type":            "response.reasoning_summary_text.delta",
		"item_id":         t.reasoningID,
		"output_index":    t.reasoningOutIdx,
		"summary_index":   0,
		"delta":           text,
		"sequence_number": t.nextSeq(),
	})
	return ev
}

func (t *ResponsesStreamTranslator) emitReasoningClose() ([][]byte, error) {
	if t.reasoningDone {
		return nil, nil
	}
	t.reasoningDone = true
	fullText := t.reasoningText.String()

	textDone, err := formatSSEEvent("response.reasoning_summary_text.done", map[string]interface{}{
		"type":            "response.reasoning_summary_text.done",
		"item_id":         t.reasoningID,
		"output_index":    t.reasoningOutIdx,
		"summary_index":   0,
		"text":            fullText,
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	partDone, err := formatSSEEvent("response.reasoning_summary_part.done", map[string]interface{}{
		"type":          "response.reasoning_summary_part.done",
		"item_id":       t.reasoningID,
		"output_index":  t.reasoningOutIdx,
		"summary_index": 0,
		"part": map[string]interface{}{
			"type": "summary_text",
			"text": fullText,
		},
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}

	itemDone, err := formatSSEEvent("response.output_item.done", map[string]interface{}{
		"type":            "response.output_item.done",
		"output_index":    t.reasoningOutIdx,
		"item":            t.reasoningOutputItem(),
		"sequence_number": t.nextSeq(),
	})
	if err != nil {
		return nil, err
	}
	return [][]byte{textDone, partDone, itemDone}, nil
}

func (t *ResponsesStreamTranslator) reasoningOutputItem() map[string]interface{} {
	return map[string]interface{}{
		"id":   t.reasoningID,
		"type": "reasoning",
		"summary": []interface{}{
			map[string]interface{}{
				"type": "summary_text",
				"text": t.reasoningText.String(),
			},
		},
	}
}

// Ensure fmt is used (for formatSSEEvent usage pattern matching stream.go).
var _ = fmt.Sprintf
