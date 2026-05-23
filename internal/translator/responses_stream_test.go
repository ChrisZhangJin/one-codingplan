package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

// parseSSEEvents parses a sequence of SSE event byte slices into a list of
// (event-type, data-map) pairs for assertion.
func parseSSEEvents(t *testing.T, events [][]byte) []map[string]interface{} {
	t.Helper()
	var result []map[string]interface{}
	for _, ev := range events {
		s := string(ev)
		var eventType, dataLine string
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
			}
		}
		if dataLine == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(dataLine), &m); err != nil {
			t.Fatalf("failed to parse SSE data %q: %v", dataLine, err)
		}
		if eventType != "" {
			m["_event"] = eventType
		}
		result = append(result, m)
	}
	return result
}

func eventTypes(parsed []map[string]interface{}) []string {
	var types []string
	for _, m := range parsed {
		if et, ok := m["type"].(string); ok {
			types = append(types, et)
		}
	}
	return types
}

func seqNum(m map[string]interface{}) int {
	if v, ok := m["sequence_number"].(float64); ok {
		return int(v)
	}
	return -1
}

func TestResponsesStream_FirstChunk(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")
	chunk := []byte(`data: {"choices":[{"delta":{"content":"Hello"}}]}` + "\n\n")
	events, err := tr.Translate(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseSSEEvents(t, events)
	types := eventTypes(parsed)

	// Must include opening events + delta
	wantTypes := []string{
		"response.created",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
	}
	if len(types) != len(wantTypes) {
		t.Fatalf("expected %d events, got %d: %v", len(wantTypes), len(types), types)
	}
	for i, want := range wantTypes {
		if types[i] != want {
			t.Errorf("event[%d]: want %q, got %q", i, want, types[i])
		}
	}

	// delta event must contain "Hello"
	deltaEv := parsed[3]
	if deltaEv["delta"] != "Hello" {
		t.Errorf("expected delta=Hello, got %v", deltaEv["delta"])
	}

	// response.created must be the first event
	if parsed[0]["type"] != "response.created" {
		t.Errorf("expected first event response.created, got %v", parsed[0]["type"])
	}

	// sequence_numbers must be monotonically increasing starting from 0
	for i := 1; i < len(parsed); i++ {
		if seqNum(parsed[i]) <= seqNum(parsed[i-1]) {
			t.Errorf("sequence_number not monotonically increasing at index %d: %d <= %d",
				i, seqNum(parsed[i]), seqNum(parsed[i-1]))
		}
	}
}

func TestResponsesStream_SecondChunk(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	// First chunk — opens stream
	first := []byte(`data: {"choices":[{"delta":{"content":"Hello"}}]}` + "\n\n")
	firstEvents, err := tr.Translate(first)
	if err != nil {
		t.Fatalf("unexpected error on first chunk: %v", err)
	}
	firstParsed := parseSSEEvents(t, firstEvents)

	// Second chunk — should emit only response.output_text.delta
	second := []byte(`data: {"choices":[{"delta":{"content":" World"}}]}` + "\n\n")
	events, err := tr.Translate(second)
	if err != nil {
		t.Fatalf("unexpected error on second chunk: %v", err)
	}
	parsed := parseSSEEvents(t, events)
	types := eventTypes(parsed)

	if len(types) != 1 || types[0] != "response.output_text.delta" {
		t.Fatalf("expected [response.output_text.delta], got %v", types)
	}
	if parsed[0]["delta"] != " World" {
		t.Errorf("expected delta=' World', got %v", parsed[0]["delta"])
	}

	// sequence_number must continue from first chunk
	lastFirstSeq := seqNum(firstParsed[len(firstParsed)-1])
	if seqNum(parsed[0]) <= lastFirstSeq {
		t.Errorf("second chunk sequence_number %d not greater than first chunk last %d",
			seqNum(parsed[0]), lastFirstSeq)
	}
}

func TestResponsesStream_DoneSignal(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	// Feed two content chunks to accumulate text
	tr.Translate([]byte(`data: {"choices":[{"delta":{"content":"Hello"}}]}` + "\n\n")) //nolint:errcheck
	tr.Translate([]byte(`data: {"choices":[{"delta":{"content":" World"}}]}` + "\n\n")) //nolint:errcheck

	// Feed [DONE]
	events, err := tr.Translate([]byte("data: [DONE]\n\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseSSEEvents(t, events)
	types := eventTypes(parsed)

	wantTypes := []string{
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	if len(types) != len(wantTypes) {
		t.Fatalf("expected %d closing events, got %d: %v", len(wantTypes), len(types), types)
	}
	for i, want := range wantTypes {
		if types[i] != want {
			t.Errorf("event[%d]: want %q, got %q", i, want, types[i])
		}
	}

	// response.output_text.done must contain full accumulated text
	doneTxtEv := parsed[0]
	if doneTxtEv["text"] != "Hello World" {
		t.Errorf("expected text=Hello World in output_text.done, got %v", doneTxtEv["text"])
	}

	// response.completed must also contain the accumulated text
	completedEv := parsed[3]
	resp, ok := completedEv["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected response object in response.completed, got %v", completedEv)
	}
	output, ok := resp["output"].([]interface{})
	if !ok || len(output) == 0 {
		t.Fatalf("expected non-empty output in response.completed, got %v", resp["output"])
	}
	outItem, ok := output[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected output[0] to be a map, got %T", output[0])
	}
	content, ok := outItem["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("expected non-empty content in output[0], got %v", outItem["content"])
	}
	contentPart, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected content[0] to be a map, got %T", content[0])
	}
	if contentPart["text"] != "Hello World" {
		t.Errorf("expected content[0].text=Hello World in response.completed, got %v", contentPart["text"])
	}

	// sequence_numbers must be monotonically increasing
	for i := 1; i < len(parsed); i++ {
		if seqNum(parsed[i]) <= seqNum(parsed[i-1]) {
			t.Errorf("sequence_number not monotonically increasing at index %d", i)
		}
	}
}

func TestResponsesStream_FinishReasonStop(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	// First populate with some content
	tr.Translate([]byte(`data: {"choices":[{"delta":{"content":"Hi"}}]}` + "\n\n")) //nolint:errcheck

	// Chunk with finish_reason="stop" (may have no content)
	events, err := tr.Translate([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseSSEEvents(t, events)
	types := eventTypes(parsed)

	// Should emit closing sequence
	wantClosing := []string{
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	if len(types) != len(wantClosing) {
		t.Fatalf("expected closing sequence %v, got %v", wantClosing, types)
	}
	for i, want := range wantClosing {
		if types[i] != want {
			t.Errorf("event[%d]: want %q, got %q", i, want, types[i])
		}
	}
}

func TestResponsesStream_EmptyChoices(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	// Chunk with empty choices (no content, no finish_reason)
	events, err := tr.Translate([]byte(`data: {"choices":[{"delta":{}}]}` + "\n\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected no events for empty choices chunk, got %d", len(events))
	}
}

func TestResponsesStream_ToolCallDelta(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	// First chunk: tool call starts with id and name
	chunk1 := []byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"bash","arguments":""}}]},"finish_reason":null}]}` + "\n\n")
	events1, err := tr.Translate(chunk1)
	if err != nil {
		t.Fatalf("unexpected error on chunk1: %v", err)
	}
	parsed1 := parseSSEEvents(t, events1)
	types1 := eventTypes(parsed1)
	wantTypes1 := []string{"response.created", "response.output_item.added"}
	if len(types1) != len(wantTypes1) {
		t.Fatalf("expected %v, got %v", wantTypes1, types1)
	}
	for i, want := range wantTypes1 {
		if types1[i] != want {
			t.Errorf("event[%d]: want %q, got %q", i, want, types1[i])
		}
	}

	// Second chunk: argument delta
	chunk2 := []byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]},"finish_reason":null}]}` + "\n\n")
	events2, err := tr.Translate(chunk2)
	if err != nil {
		t.Fatalf("unexpected error on chunk2: %v", err)
	}
	parsed2 := parseSSEEvents(t, events2)
	types2 := eventTypes(parsed2)
	if len(types2) != 1 || types2[0] != "response.function_call_arguments.delta" {
		t.Fatalf("expected [response.function_call_arguments.delta], got %v", types2)
	}

	// Third chunk: finish_reason=tool_calls
	chunk3 := []byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n")
	events3, err := tr.Translate(chunk3)
	if err != nil {
		t.Fatalf("unexpected error on chunk3: %v", err)
	}
	parsed3 := parseSSEEvents(t, events3)
	types3 := eventTypes(parsed3)
	wantTypes3 := []string{
		"response.function_call_arguments.done",
		"response.output_item.done",
		"response.completed",
	}
	if len(types3) != len(wantTypes3) {
		t.Fatalf("expected %v, got %v", wantTypes3, types3)
	}
	for i, want := range wantTypes3 {
		if types3[i] != want {
			t.Errorf("event[%d]: want %q, got %q", i, want, types3[i])
		}
	}

	// response.completed output must be function_call type
	completedEv := parsed3[2]
	resp, ok := completedEv["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected response object in response.completed")
	}
	output, ok := resp["output"].([]interface{})
	if !ok || len(output) == 0 {
		t.Fatalf("expected non-empty output in response.completed")
	}
	outItem, ok := output[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected output[0] to be a map")
	}
	if outItem["type"] != "function_call" {
		t.Errorf("expected output type=function_call, got %v", outItem["type"])
	}
	if outItem["name"] != "bash" {
		t.Errorf("expected name=bash, got %v", outItem["name"])
	}

	// Sequence numbers must be strictly increasing across all chunks
	allParsed := append(append(parsed1, parsed2...), parsed3...)
	for i := 1; i < len(allParsed); i++ {
		if seqNum(allParsed[i]) <= seqNum(allParsed[i-1]) {
			t.Errorf("sequence_number not monotonically increasing at index %d: %d <= %d",
				i, seqNum(allParsed[i]), seqNum(allParsed[i-1]))
		}
	}
}

func TestResponsesStream_FinishReasonThenDoneEmitsCloseOnce(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	tr.Translate([]byte(`data: {"choices":[{"delta":{"content":"Hi"}}]}` + "\n\n")) //nolint:errcheck

	finishEvents, err := tr.Translate([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
	if err != nil {
		t.Fatalf("unexpected error on finish chunk: %v", err)
	}
	finishParsed := parseSSEEvents(t, finishEvents)
	if got := countCompleted(finishParsed); got != 1 {
		t.Fatalf("expected exactly 1 response.completed on finish_reason, got %d", got)
	}

	doneEvents, err := tr.Translate([]byte("data: [DONE]\n\n"))
	if err != nil {
		t.Fatalf("unexpected error on [DONE]: %v", err)
	}
	if len(doneEvents) != 0 {
		t.Fatalf("expected [DONE] after finish_reason to emit no events, got %d: %v",
			len(doneEvents), eventTypes(parseSSEEvents(t, doneEvents)))
	}
}

func TestResponsesStream_ToolCallFinishThenDoneEmitsCloseOnce(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	tr.Translate([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"bash","arguments":""}}]}}]}` + "\n\n"))                  //nolint:errcheck
	tr.Translate([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cmd\":\"ls\"}"}}]}}]}` + "\n\n"))                                              //nolint:errcheck
	finishEvents, err := tr.Translate([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
	if err != nil {
		t.Fatalf("unexpected error on finish chunk: %v", err)
	}
	finishParsed := parseSSEEvents(t, finishEvents)
	if got := countCompleted(finishParsed); got != 1 {
		t.Fatalf("expected exactly 1 response.completed on tool_calls finish, got %d", got)
	}

	doneEvents, err := tr.Translate([]byte("data: [DONE]\n\n"))
	if err != nil {
		t.Fatalf("unexpected error on [DONE]: %v", err)
	}
	if len(doneEvents) != 0 {
		t.Fatalf("expected [DONE] after tool_calls finish to emit no events, got %d: %v",
			len(doneEvents), eventTypes(parseSSEEvents(t, doneEvents)))
	}
}

func TestResponsesStream_ToolCallEventsCarryCallID(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	tr.Translate([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc123","type":"function","function":{"name":"bash","arguments":""}}]}}]}` + "\n\n")) //nolint:errcheck
	tr.Translate([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cmd\":\"ls\"}"}}]}}]}` + "\n\n"))                                //nolint:errcheck
	closingEvents, err := tr.Translate([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr2 := NewResponsesStreamTranslator("gpt-4")
	openEvents, _ := tr2.Translate([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc123","type":"function","function":{"name":"bash","arguments":""}}]}}]}` + "\n\n"))
	openParsed := parseSSEEvents(t, openEvents)
	var added map[string]interface{}
	for _, ev := range openParsed {
		if ev["type"] == "response.output_item.added" {
			added = ev
			break
		}
	}
	if added == nil {
		t.Fatalf("missing response.output_item.added in %v", eventTypes(openParsed))
	}
	addedItem := added["item"].(map[string]interface{})
	if addedItem["call_id"] != "call_abc123" {
		t.Errorf("output_item.added: expected call_id=call_abc123, got %v", addedItem["call_id"])
	}

	closingParsed := parseSSEEvents(t, closingEvents)
	var done, completed map[string]interface{}
	for _, ev := range closingParsed {
		switch ev["type"] {
		case "response.output_item.done":
			done = ev
		case "response.completed":
			completed = ev
		}
	}
	if done == nil || completed == nil {
		t.Fatalf("missing output_item.done or response.completed in %v", eventTypes(closingParsed))
	}

	doneItem := done["item"].(map[string]interface{})
	if doneItem["call_id"] != "call_abc123" {
		t.Errorf("output_item.done: expected call_id=call_abc123, got %v", doneItem["call_id"])
	}

	resp := completed["response"].(map[string]interface{})
	output := resp["output"].([]interface{})
	completedItem := output[0].(map[string]interface{})
	if completedItem["call_id"] != "call_abc123" {
		t.Errorf("response.completed output[0]: expected call_id=call_abc123, got %v", completedItem["call_id"])
	}
}

func TestResponsesStream_ToolCallFirstEmitsCreated(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	chunk := []byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"bash","arguments":""}}]}}]}` + "\n\n")
	events, err := tr.Translate(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseSSEEvents(t, events)
	if len(parsed) == 0 || parsed[0]["type"] != "response.created" {
		t.Fatalf("expected first event response.created on tool-call-first stream, got %v", eventTypes(parsed))
	}
	if seqNum(parsed[0]) != 0 {
		t.Errorf("expected response.created sequence_number=0, got %d", seqNum(parsed[0]))
	}
}

func countCompleted(parsed []map[string]interface{}) int {
	n := 0
	for _, m := range parsed {
		if m["type"] == "response.completed" {
			n++
		}
	}
	return n
}

func TestResponsesStream_ThinkBlockBecomesReasoningSummary(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")
	chunks := [][]byte{
		[]byte(`data: {"choices":[{"delta":{"content":"<think>"}}]}` + "\n\n"),
		[]byte(`data: {"choices":[{"delta":{"content":"weighing options"}}]}` + "\n\n"),
		[]byte(`data: {"choices":[{"delta":{"content":"</think>"}}]}` + "\n\n"),
		[]byte(`data: {"choices":[{"delta":{"content":"Hello"}}]}` + "\n\n"),
		[]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"),
	}
	var all []map[string]interface{}
	for _, c := range chunks {
		evs, err := tr.Translate(c)
		if err != nil {
			t.Fatalf("translate err: %v", err)
		}
		all = append(all, parseSSEEvents(t, evs)...)
	}

	gotTypes := eventTypes(all)
	wantOrder := []string{
		"response.created",
		"response.output_item.added",            // reasoning
		"response.reasoning_summary_part.added", //
		"response.reasoning_summary_text.delta", // "weighing options"
		"response.reasoning_summary_text.done",
		"response.reasoning_summary_part.done",
		"response.output_item.done",       // reasoning
		"response.output_item.added",      // message
		"response.content_part.added",     //
		"response.output_text.delta",      // "Hello"
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done", // message
		"response.completed",
	}
	if len(gotTypes) != len(wantOrder) {
		t.Fatalf("event count: want %d, got %d (%v)", len(wantOrder), len(gotTypes), gotTypes)
	}
	for i, want := range wantOrder {
		if gotTypes[i] != want {
			t.Errorf("event[%d]: want %q, got %q (full: %v)", i, want, gotTypes[i], gotTypes)
		}
	}

	// Reasoning delta payload
	for _, ev := range all {
		if ev["type"] == "response.reasoning_summary_text.delta" {
			if ev["delta"] != "weighing options" {
				t.Errorf("reasoning delta: want 'weighing options', got %v", ev["delta"])
			}
		}
	}

	// Message text must NOT contain the literal <think> tags
	for _, ev := range all {
		if ev["type"] == "response.output_text.delta" {
			if s, _ := ev["delta"].(string); strings.Contains(s, "<think>") || strings.Contains(s, "</think>") {
				t.Errorf("message delta leaked think tags: %q", s)
			}
		}
	}

	// response.completed output[] must include reasoning then message
	var completed map[string]interface{}
	for _, ev := range all {
		if ev["type"] == "response.completed" {
			completed = ev
		}
	}
	if completed == nil {
		t.Fatal("missing response.completed")
	}
	resp := completed["response"].(map[string]interface{})
	output := resp["output"].([]interface{})
	if len(output) != 2 {
		t.Fatalf("expected 2 output items (reasoning, message), got %d: %+v", len(output), output)
	}
	if output[0].(map[string]interface{})["type"] != "reasoning" {
		t.Errorf("output[0] type: want reasoning, got %v", output[0])
	}
	if output[1].(map[string]interface{})["type"] != "message" {
		t.Errorf("output[1] type: want message, got %v", output[1])
	}
}

func TestResponsesStream_ThinkTagSplitAcrossChunks(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")
	chunks := [][]byte{
		[]byte(`data: {"choices":[{"delta":{"content":"<thi"}}]}` + "\n\n"),
		[]byte(`data: {"choices":[{"delta":{"content":"nk>plan</thi"}}]}` + "\n\n"),
		[]byte(`data: {"choices":[{"delta":{"content":"nk>done"}}]}` + "\n\n"),
		[]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"),
	}
	var all []map[string]interface{}
	for _, c := range chunks {
		evs, _ := tr.Translate(c)
		all = append(all, parseSSEEvents(t, evs)...)
	}

	var reasoningText strings.Builder
	var messageText strings.Builder
	for _, ev := range all {
		switch ev["type"] {
		case "response.reasoning_summary_text.delta":
			reasoningText.WriteString(ev["delta"].(string))
		case "response.output_text.delta":
			messageText.WriteString(ev["delta"].(string))
		}
	}
	if reasoningText.String() != "plan" {
		t.Errorf("reasoning text: want 'plan', got %q", reasoningText.String())
	}
	if messageText.String() != "done" {
		t.Errorf("message text: want 'done', got %q", messageText.String())
	}
	if strings.Contains(messageText.String(), "<") || strings.Contains(reasoningText.String(), "<") {
		t.Errorf("tag leakage: reasoning=%q message=%q", reasoningText.String(), messageText.String())
	}
}

func TestResponsesStream_ThinkBeforeToolCall(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")
	tr.Translate([]byte(`data: {"choices":[{"delta":{"content":"<think>route to bash</think>"}}]}` + "\n\n")) //nolint:errcheck
	tr.Translate([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_x","type":"function","function":{"name":"bash","arguments":""}}]}}]}` + "\n\n")) //nolint:errcheck
	finishEvts, _ := tr.Translate([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
	parsed := parseSSEEvents(t, finishEvts)

	var completed map[string]interface{}
	for _, ev := range parsed {
		if ev["type"] == "response.completed" {
			completed = ev
		}
	}
	if completed == nil {
		t.Fatal("missing response.completed in finish events")
	}
	resp := completed["response"].(map[string]interface{})
	output := resp["output"].([]interface{})
	if len(output) != 2 {
		t.Fatalf("expected reasoning + function_call (2 items), got %d", len(output))
	}
	if output[0].(map[string]interface{})["type"] != "reasoning" {
		t.Errorf("output[0]: want reasoning, got %v", output[0])
	}
	if output[1].(map[string]interface{})["type"] != "function_call" {
		t.Errorf("output[1]: want function_call, got %v", output[1])
	}
}

func TestResponsesStream_UnclosedThinkClosesOnFinish(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")
	tr.Translate([]byte(`data: {"choices":[{"delta":{"content":"<think>cut off"}}]}` + "\n\n")) //nolint:errcheck
	finishEvts, _ := tr.Translate([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
	parsed := parseSSEEvents(t, finishEvts)
	types := eventTypes(parsed)

	// Must emit reasoning_summary_text.done before response.completed
	doneIdx, completedIdx := -1, -1
	for i, ty := range types {
		if ty == "response.reasoning_summary_text.done" {
			doneIdx = i
		}
		if ty == "response.completed" {
			completedIdx = i
		}
	}
	if doneIdx < 0 {
		t.Fatalf("expected reasoning_summary_text.done in finish events, got %v", types)
	}
	if completedIdx < 0 || completedIdx < doneIdx {
		t.Fatalf("expected response.completed after reasoning close, got %v", types)
	}
}

func TestResponsesStream_SequenceOrderAfterFull(t *testing.T) {
	tr := NewResponsesStreamTranslator("gpt-4")

	// Accumulate a full stream
	tr.Translate([]byte(`data: {"choices":[{"delta":{"content":"A"}}]}` + "\n\n")) //nolint:errcheck
	deltaEvents, _ := tr.Translate([]byte(`data: {"choices":[{"delta":{"content":"B"}}]}` + "\n\n"))
	doneEvents, _ := tr.Translate([]byte("data: [DONE]\n\n"))

	deltaParsed := parseSSEEvents(t, deltaEvents)
	doneParsed := parseSSEEvents(t, doneEvents)

	if len(deltaParsed) == 0 || len(doneParsed) == 0 {
		t.Fatal("expected events from both delta and done chunks")
	}

	lastDeltaSeq := seqNum(deltaParsed[len(deltaParsed)-1])
	completedSeq := seqNum(doneParsed[len(doneParsed)-1])

	if completedSeq <= lastDeltaSeq {
		t.Errorf("response.completed sequence_number %d should be greater than last delta sequence_number %d",
			completedSeq, lastDeltaSeq)
	}
}
