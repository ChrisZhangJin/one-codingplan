package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func responsesReqBody(stream bool, input interface{}) []byte {
	b := map[string]interface{}{
		"model":  "qwen-max",
		"input":  input,
		"stream": stream,
	}
	bs, _ := json.Marshal(b)
	return bs
}

func responsesReqBodyWithInstructions(stream bool, input interface{}, instructions string) []byte {
	b := map[string]interface{}{
		"model":        "qwen-max",
		"input":        input,
		"stream":       stream,
		"instructions": instructions,
	}
	bs, _ := json.Marshal(b)
	return bs
}

func defaultInput() interface{} {
	return []map[string]interface{}{
		{"role": "user", "content": "hello"},
	}
}

func TestResponsesRelay_NonStream(t *testing.T) {
	db := setupTestDB(t)
	upstream := fakeOpenAIUpstream(openAIResponseBody("Hello from Codex", "stop", 10, 5))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader(responsesReqBody(false, defaultInput())))
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
	if resp["object"] != "response" {
		t.Errorf("expected object=response, got %v", resp["object"])
	}
	if resp["status"] != "completed" {
		t.Errorf("expected status=completed, got %v", resp["status"])
	}

	output, ok := resp["output"].([]interface{})
	if !ok || len(output) == 0 {
		t.Fatalf("expected non-empty output array, got %v", resp["output"])
	}
	outItem, ok := output[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected output[0] to be a map, got %T", output[0])
	}
	content, ok := outItem["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("expected non-empty content array in output[0], got %v", outItem["content"])
	}
	part, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected content[0] to be a map, got %T", content[0])
	}
	if part["text"] != "Hello from Codex" {
		t.Errorf("expected text=Hello from Codex, got %v", part["text"])
	}

	usage, ok := resp["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected usage object, got %v", resp["usage"])
	}
	if usage["input_tokens"] != float64(10) {
		t.Errorf("expected input_tokens=10, got %v", usage["input_tokens"])
	}
	if usage["output_tokens"] != float64(5) {
		t.Errorf("expected output_tokens=5, got %v", usage["output_tokens"])
	}
}

func TestResponsesRelay_NonStream_WithInstructions(t *testing.T) {
	db := setupTestDB(t)

	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("ok", "stop", 5, 3))
	}))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader(responsesReqBodyWithInstructions(false, defaultInput(), "Be helpful")))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Parse the body sent to upstream
	var sentReq map[string]interface{}
	if err := json.Unmarshal(capturedBody, &sentReq); err != nil {
		t.Fatalf("failed to parse captured upstream body: %v", err)
	}
	messages, ok := sentReq["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		t.Fatalf("expected messages array at upstream, got %v", sentReq["messages"])
	}
	firstMsg, ok := messages[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected messages[0] to be a map, got %T", messages[0])
	}
	if firstMsg["role"] != "system" {
		t.Errorf("expected messages[0].role=system, got %v", firstMsg["role"])
	}
	if firstMsg["content"] != "Be helpful" {
		t.Errorf("expected messages[0].content=Be helpful, got %v", firstMsg["content"])
	}
}

func TestResponsesRelay_Auth_Missing(t *testing.T) {
	db := setupTestDB(t)
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 1, 1))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader(responsesReqBody(false, defaultInput())))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestResponsesRelay_Auth_Invalid(t *testing.T) {
	db := setupTestDB(t)
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 1, 1))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader(responsesReqBody(false, defaultInput())))
	req.Header.Set("Authorization", "Bearer wrong-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestResponsesRelay_StringInput(t *testing.T) {
	db := setupTestDB(t)
	upstream := fakeOpenAIUpstream(openAIResponseBody("hi", "stop", 1, 1))
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	// Use raw string input (not array) to trigger ErrStringInput
	body := []byte(`{"model":"qwen-max","input":"hello","stream":false}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", m)
	}
	if errObj["code"] != "invalid_request_error" {
		t.Errorf("expected error.code=invalid_request_error, got %v", errObj["code"])
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "array") {
		t.Errorf("expected error message to contain 'array', got %q", msg)
	}
}

func TestResponsesRelay_Stream(t *testing.T) {
	db := setupTestDB(t)
	frames := []string{
		`{"choices":[{"delta":{"content":"Hi"}}]}`,
		`{"choices":[{"delta":{"content":"!"}}]}`,
		`[DONE]`,
	}
	upstream := fakeOpenAIStreamingUpstream(frames)
	defer upstream.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	seedUpstream(t, db, "up1", upstream.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	body := []byte(`{"model":"qwen-max","input":[{"role":"user","content":"hello"}],"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
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
	respBody := w.Body.String()
	if !strings.Contains(respBody, "event: response.created") {
		t.Errorf("expected 'event: response.created' in response body, got: %s", respBody)
	}
	if !strings.Contains(respBody, "event: response.output_text.delta") {
		t.Errorf("expected 'event: response.output_text.delta' in response body, got: %s", respBody)
	}
	if !strings.Contains(respBody, "event: response.completed") {
		t.Errorf("expected 'event: response.completed' in response body, got: %s", respBody)
	}
}

func TestResponsesRelay_Failover(t *testing.T) {
	db := setupTestDB(t)
	calls500 := int32(0)
	calls200 := int32(0)

	fake500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls500, 1)
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer fake500.Close()

	fake200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls200, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(openAIResponseBody("ok", "stop", 5, 3))
	}))
	defer fake200.Close()

	seedAccessKey(t, db, "test-token-abc", true)
	// Seed 200 first (lower ID) so pool selects 500 first
	seedUpstream(t, db, "up-ok", fake200.URL)
	seedUpstream(t, db, "up-500", fake500.URL)
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader(responsesReqBody(false, defaultInput())))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 after failover, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["object"] != "response" {
		t.Errorf("expected object=response in Responses API format, got %v", resp["object"])
	}
}

func TestResponsesRelay_AllFail(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader(responsesReqBody(false, defaultInput())))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 503 {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", m)
	}
	if errObj["code"] != "server_error" {
		t.Errorf("expected error.code=server_error, got %v", errObj["code"])
	}
}

func TestResponsesRelay_NoUpstream(t *testing.T) {
	db := setupTestDB(t)

	seedAccessKey(t, db, "test-token-abc", true)
	// No upstreams seeded
	p := buildPool(t, db, 10*time.Millisecond)
	engine := buildServer(db, p).Engine()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader(responsesReqBody(false, defaultInput())))
	req.Header.Set("Authorization", "Bearer test-token-abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 503 {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
