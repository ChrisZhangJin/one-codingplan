package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// anthropicReqBody builds a minimal valid AnthropicRequest JSON.
func anthropicReqBody(stream bool) []byte {
	b := map[string]interface{}{
		"model": "claude-opus-4-5",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "hello"},
		},
		"max_tokens": 1024,
		"stream":     stream,
	}
	bs, _ := json.Marshal(b)
	return bs
}

// fakeOpenAIUpstream returns a test server that responds with 200 + the given JSON body.
func fakeOpenAIUpstream(responseBody []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(responseBody)
	}))
}

// fakeOpenAIStreamingUpstream returns a test server that sends text/event-stream
// with the given frames as "data: <frame>\n\n" lines.
func fakeOpenAIStreamingUpstream(frames []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher := w.(http.Flusher)
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			flusher.Flush()
		}
	}))
}

// openAIResponseBody builds a minimal OpenAI response JSON with a text message.
func openAIResponseBody(content, finishReason string, promptTokens, completionTokens int) []byte {
	r := map[string]interface{}{
		"id":    "chatcmpl-test",
		"model": "qwen-max",
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
		},
	}
	bs, _ := json.Marshal(r)
	return bs
}

// --- Tests ---

func TestAnthropicRelay_NonStream(t *testing.T) {
	db := setupTestDB(t)
	upstream := fakeOpenAIUpstream(openAIResponseBody("Hello back!", "stop", 10, 5))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["type"] != "message" {
		t.Errorf("expected type=message, got %v", resp["type"])
	}
	if resp["role"] != "assistant" {
		t.Errorf("expected role=assistant, got %v", resp["role"])
	}
	// D-03: model echo
	if resp["model"] != "claude-opus-4-5" {
		t.Errorf("expected model=claude-opus-4-5, got %v", resp["model"])
	}
	if resp["stop_reason"] != "end_turn" {
		t.Errorf("expected stop_reason=end_turn, got %v", resp["stop_reason"])
	}
	content, ok := resp["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("expected non-empty content array, got %v", resp["content"])
	}
	block, ok := content[0].(map[string]interface{})
	if !ok || block["type"] != "text" {
		t.Errorf("expected content[0].type=text, got %v", content[0])
	}
}

func TestAnthropicRelay_Stream(t *testing.T) {
	db := setupTestDB(t)
	frames := []string{
		`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"content":" World"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"content":"!"},"finish_reason":"stop"}]}`,
		`[DONE]`,
	}
	upstream := fakeOpenAIStreamingUpstream(frames)
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(true)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream content-type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: message_start") {
		t.Errorf("expected 'event: message_start' in response, got: %s", body)
	}
	if !strings.Contains(body, "event: content_block_delta") {
		t.Errorf("expected 'event: content_block_delta' in response, got: %s", body)
	}
	if !strings.Contains(body, "event: message_stop") {
		t.Errorf("expected 'event: message_stop' in response, got: %s", body)
	}
}

func TestAnthropicRelay_Auth(t *testing.T) {
	db := setupTestDB(t)
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 1, 1))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	// No Authorization header (T-4-09)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAnthropicRelay_Failover(t *testing.T) {
	db := setupTestDB(t)
	calls500 := int32(0)
	calls200 := int32(0)

	fake500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls500, 1)
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer fake500.Close()

	fake200 := fakeOpenAIUpstream(openAIResponseBody("ok", "stop", 5, 3))
	defer fake200.Close()
	// Wrap to count calls
	calls200Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls200, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("ok", "stop", 5, 3))
	}))
	defer calls200Server.Close()
	fake200.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	// Seed 200-server first (lower ID) so pool selects 500-server first
	seedUpstream(t, db, "up-ok", calls200Server.URL)
	seedUpstream(t, db, "up-500", fake500.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 after failover, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnthropicRelay_AllFail(t *testing.T) {
	db := setupTestDB(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 503 {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["type"] != "error" {
		t.Errorf("expected type=error, got %v", m["type"])
	}
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", m)
	}
	if errObj["type"] != "overloaded_error" {
		t.Errorf("expected error.type=overloaded_error, got %v", errObj["type"])
	}
}

func TestAnthropicRelay_Usage(t *testing.T) {
	db := setupTestDB(t)
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 10, 5))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	u := seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	rec := waitForUsageRecord(t, db)
	if !rec.Success {
		t.Error("expected Success=true")
	}
	if rec.InputTokens != 10 {
		t.Errorf("expected InputTokens=10, got %d", rec.InputTokens)
	}
	if rec.OutputTokens != 5 {
		t.Errorf("expected OutputTokens=5, got %d", rec.OutputTokens)
	}
	if rec.KeyID != "key-1" {
		t.Errorf("expected KeyID=key-1, got %q", rec.KeyID)
	}
	if rec.UpstreamID != u.ID {
		t.Errorf("expected UpstreamID=%d, got %d", u.ID, rec.UpstreamID)
	}
}

func TestAnthropicRelay_ModelOverride(t *testing.T) {
	db := setupTestDB(t)

	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("hi", "stop", 5, 3))
	}))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	// D-01: apply model override
	p.SetModelOverride("up1", "qwen-max")
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// D-01: verify upstream received the override model
	var forwarded map[string]interface{}
	if err := json.Unmarshal(capturedBody, &forwarded); err != nil {
		t.Fatalf("failed to parse forwarded body: %v", err)
	}
	if forwarded["model"] != "qwen-max" {
		t.Errorf("expected forwarded model=qwen-max, got %v", forwarded["model"])
	}

	// D-03: verify response echoes original model name
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["model"] != "claude-opus-4-5" {
		t.Errorf("expected response model=claude-opus-4-5, got %v", resp["model"])
	}
}

func TestAnthropicRelay_ModelStrip(t *testing.T) {
	db := setupTestDB(t)

	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("hi", "stop", 5, 3))
	}))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	// No model override set — D-02: model field should be absent/empty
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// D-02: verify upstream did NOT receive a claude- model name
	var forwarded map[string]interface{}
	if err := json.Unmarshal(capturedBody, &forwarded); err != nil {
		t.Fatalf("failed to parse forwarded body: %v", err)
	}
	model, hasModel := forwarded["model"]
	if hasModel && strings.HasPrefix(fmt.Sprintf("%v", model), "claude-") {
		t.Errorf("expected model not to start with 'claude-', got %v", model)
	}
}

func TestAnthropicRelay_InvalidBody(t *testing.T) {
	db := setupTestDB(t)
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 1, 1))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader([]byte(`{not valid json`)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["type"] != "error" {
		t.Errorf("expected type=error, got %v", m["type"])
	}
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", m)
	}
	if errObj["type"] != "invalid_request_error" {
		t.Errorf("expected error.type=invalid_request_error, got %v", errObj["type"])
	}
}

func TestAnthropicRelay_ToolRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	var capturedBody []byte
	// Upstream returns a tool_calls response
	toolCallResp := map[string]interface{}{
		"id":    "chatcmpl-tool",
		"model": "qwen-max",
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "",
					"tool_calls": []map[string]interface{}{
						{
							"id":   "call_abc123",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"location":"Beijing"}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     15,
			"completion_tokens": 8,
		},
	}
	toolCallBody, _ := json.Marshal(toolCallResp)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(toolCallBody)
	}))
	defer upstream.Close()

	// Build request with tool definition + tool_use in assistant + tool_result in user
	anthropicReq := map[string]interface{}{
		"model": "claude-opus-4-5",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "What's the weather in Beijing?",
			},
			{
				"role": "assistant",
				"content": []map[string]interface{}{
					{
						"type":  "tool_use",
						"id":    "call_abc123",
						"name":  "get_weather",
						"input": map[string]interface{}{"location": "Beijing"},
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": "call_abc123",
						"content":     "Sunny, 25°C",
					},
				},
			},
		},
		"max_tokens": 1024,
		"tools": []map[string]interface{}{
			{
				"name":        "get_weather",
				"description": "Get current weather",
				"input_schema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"location": map[string]interface{}{"type": "string"}},
				},
			},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify forwarded request has tool_calls (D-06) and role:tool message (D-07)
	var forwarded map[string]interface{}
	if err := json.Unmarshal(capturedBody, &forwarded); err != nil {
		t.Fatalf("failed to parse forwarded body: %v", err)
	}
	messages, ok := forwarded["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		t.Fatalf("expected messages in forwarded request, got %v", forwarded["messages"])
	}
	// Find the assistant message with tool_calls
	foundToolCalls := false
	foundToolRole := false
	for _, msg := range messages {
		m, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		if tcs, ok := m["tool_calls"]; ok && tcs != nil {
			foundToolCalls = true
		}
		if m["role"] == "tool" {
			foundToolRole = true
			if m["tool_call_id"] != "call_abc123" {
				t.Errorf("expected tool_call_id=call_abc123, got %v", m["tool_call_id"])
			}
		}
	}
	if !foundToolCalls {
		t.Error("expected forwarded request to contain tool_calls (D-06)")
	}
	if !foundToolRole {
		t.Error("expected forwarded request to contain role:tool message (D-07)")
	}

	// Verify response has tool_use content block with correct ID
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	content, ok := resp["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("expected content array in response, got %v", resp["content"])
	}
	foundToolUse := false
	for _, block := range content {
		b, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		if b["type"] == "tool_use" {
			foundToolUse = true
			if b["id"] != "call_abc123" {
				t.Errorf("expected tool_use id=call_abc123, got %v", b["id"])
			}
			if b["name"] != "get_weather" {
				t.Errorf("expected tool_use name=get_weather, got %v", b["name"])
			}
		}
	}
	if !foundToolUse {
		t.Error("expected tool_use content block in Anthropic response")
	}
	if resp["stop_reason"] != "tool_use" {
		t.Errorf("expected stop_reason=tool_use, got %v", resp["stop_reason"])
	}
}

// TestAnthropicPassthrough_NonStream verifies that an upstream with Format=="anthropic"
// receives the raw Anthropic body verbatim at /v1/messages, and the response is forwarded verbatim.
func TestAnthropicPassthrough_NonStream(t *testing.T) {
	db := setupTestDB(t)

	var capturedPath string
	var capturedBody []byte
	var capturedContentType string

	passthroughResp := map[string]interface{}{
		"id":      "msg_pass_01",
		"type":    "message",
		"role":    "assistant",
		"model":   "claude-opus-4-5",
		"content": []map[string]interface{}{{"type": "text", "text": "passthrough response"}},
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{"input_tokens": 7, "output_tokens": 3},
	}
	passthroughBody, _ := json.Marshal(passthroughResp)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		capturedContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(passthroughBody)
	}))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "mimo", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	p.SetFormat("mimo", "anthropic")
	engine := buildServer(db, p).Engine()

	reqBody := anthropicReqBody(false)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Upstream must have received request at /v1/messages (passthrough path)
	if capturedPath != "/v1/messages" {
		t.Errorf("expected upstream path=/v1/messages, got %q", capturedPath)
	}
	// Upstream must have received verbatim Anthropic body
	if !bytes.Equal(capturedBody, reqBody) {
		t.Errorf("upstream body mismatch: got %s, want %s", capturedBody, reqBody)
	}
	if capturedContentType != "application/json" {
		t.Errorf("expected Content-Type=application/json at upstream, got %q", capturedContentType)
	}
	// Response must be forwarded verbatim (contains passthrough JSON)
	if !bytes.Contains(w.Body.Bytes(), []byte("passthrough response")) {
		t.Errorf("expected verbatim passthrough response in body, got: %s", w.Body.String())
	}
}

// TestAnthropicTranslate_NonStream verifies that an upstream with Format=="" receives
// an OpenAI-translated body at /v1/chat/completions.
func TestAnthropicTranslate_NonStream(t *testing.T) {
	db := setupTestDB(t)

	var capturedPath string
	var capturedBody []byte

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("translated response", "stop", 10, 5))
	}))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	// No SetFormat call — default empty format means translate path
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Upstream must have received request at /v1/chat/completions (translate path)
	if capturedPath != "/v1/chat/completions" {
		t.Errorf("expected upstream path=/v1/chat/completions, got %q", capturedPath)
	}
	// Forwarded body must contain "messages" key (OpenAI format)
	var forwarded map[string]interface{}
	if err := json.Unmarshal(capturedBody, &forwarded); err != nil {
		t.Fatalf("failed to parse forwarded body: %v", err)
	}
	if _, ok := forwarded["messages"]; !ok {
		t.Errorf("expected OpenAI 'messages' key in forwarded body, got %v", forwarded)
	}
}

// TestAnthropicPassthrough_UpstreamError verifies that a passthrough upstream returning 500
// causes failover to the next upstream (format=openai).
func TestAnthropicPassthrough_UpstreamError(t *testing.T) {
	db := setupTestDB(t)

	fake500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer fake500.Close()

	fake200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("ok from openai upstream", "stop", 5, 3))
	}))
	defer fake200.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	// Seed 200 upstream first (lower ID) so pool selects 500 (anthropic) first in round-robin
	seedUpstream(t, db, "up-ok", fake200.URL)
	seedUpstream(t, db, "up-500-anthropic", fake500.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	p.SetFormat("up-500-anthropic", "anthropic")
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader(anthropicReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 after failover, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRelay_OpenAI_Regression(t *testing.T) {
	db := setupTestDB(t)
	fake := fakeOpenAIUpstream(upstreamResponse(10, 5))
	defer fake.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", fake.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader(chatReqBody(false)))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 on OpenAI path (regression), got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
}
