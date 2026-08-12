package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
)

// writeNDJSONChunk 함수는 테스트 서버에서 NDJSON chunk 하나를 즉시 flush해 내보낸다.
func writeNDJSONChunk(w http.ResponseWriter, chunk string) {
	fmt.Fprintf(w, "%s\n", chunk)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func collectOllamaStreamEvents(t *testing.T, seq func(func(ChatStreamEvent, error) bool)) ([]string, *ChatResponse, error) {
	t.Helper()
	var deltas []string
	var resp *ChatResponse
	var streamErr error
	for event, err := range seq {
		if err != nil {
			streamErr = err
			break
		}
		switch event.Kind {
		case ChatStreamEventTextDelta:
			deltas = append(deltas, event.TextDelta)
		case ChatStreamEventResponse:
			resp = event.Response
		}
	}
	return deltas, resp, streamErr
}

// TestOllamaClientSendsChatRequestAndDecodesResponse 는 Ollama adapter가 내부 요청을 Chat API 형태로 변환하고
// non-streaming 응답의 Tool call을 보존하는지 확인한다.
func TestOllamaClientSendsChatRequestAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Method = %s, want POST", r.Method)
		}
		if r.URL.Path != ollamaChatPath {
			t.Fatalf("Path = %s, want %s", r.URL.Path, ollamaChatPath)
		}
		if got := r.Header.Get("Content-Type"); got != ollamaRequestMediaType {
			t.Fatalf("Content-Type = %q, want %q", got, ollamaRequestMediaType)
		}

		var req ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		if req.Model != "ollama-test" {
			t.Fatalf("Model = %q, want ollama-test", req.Model)
		}
		if req.Stream {
			t.Fatal("Stream = true, want false")
		}
		if len(req.Messages) != 4 {
			t.Fatalf("len(Messages) = %d, want 4", len(req.Messages))
		}
		if len(req.Tools) != 1 {
			t.Fatalf("len(Tools) = %d, want 1", len(req.Tools))
		}
		if req.Tools[0].Type != "function" {
			t.Fatalf("tool type = %q, want function", req.Tools[0].Type)
		}
		if req.Tools[0].Function.Name != "search" || req.Tools[0].Function.Description != "Search documents" {
			t.Fatalf("tool function = %+v, want search schema", req.Tools[0].Function)
		}
		if string(req.Tools[0].Function.Parameters) != `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}` {
			t.Fatalf("tool parameters = %s, want query schema", req.Tools[0].Function.Parameters)
		}
		if req.Messages[0].Role != "system" || req.Messages[0].Content != "runtime rule" {
			t.Fatalf("system message = %+v, want system text", req.Messages[0])
		}
		if req.Messages[1].Role != "user" || req.Messages[1].Content != "hello" {
			t.Fatalf("user message = %+v, want user text", req.Messages[1])
		}
		if req.Messages[2].Role != "assistant" || len(req.Messages[2].ToolCalls) != 1 {
			t.Fatalf("assistant message = %+v, want assistant tool call", req.Messages[2])
		}
		if req.Messages[2].ToolCalls[0].Function.Name != "search" ||
			string(req.Messages[2].ToolCalls[0].Function.Arguments) != `{"query":"before"}` {
			t.Fatalf("assistant tool call = %+v, want search args", req.Messages[2].ToolCalls[0])
		}
		if req.Messages[3].Role != "tool" || req.Messages[3].ToolResultName != "search" || req.Messages[3].Content != "result text" {
			t.Fatalf("tool message = %+v, want tool result", req.Messages[3])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"ollama-test",
			"message":{
				"role":"assistant",
				"content":"checking",
				"tool_calls":[
					{"function":{"name":"search","arguments":{"query":"agent runtime"}}}
				]
			},
			"done_reason":"stop",
			"done":true,
			"prompt_eval_count":5,
			"eval_count":13
		}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{
		Model: "ollama-test",
		Host:  server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Tools: []message.ToolSchema{
			{
				Name:        "search",
				Description: "Search documents",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
			},
		},
		Messages: []message.Message{
			message.System("runtime rule"),
			message.User("hello"),
			message.Assistant("previous", message.ToolCall{
				ID:        "call_previous",
				Name:      "search",
				Arguments: json.RawMessage(`{"query":"before"}`),
			}),
			message.Tool(message.ToolResult{
				ToolCallID: "call_previous",
				Name:       "search",
				Content:    "result text",
			}),
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if resp.Provider != ProviderOllama || resp.Model != "ollama-test" {
		t.Fatalf("response provider/model = %q/%q, want ollama/ollama-test", resp.Provider, resp.Model)
	}
	if resp.Message.Role != message.RoleAssistant || resp.Message.Text != "checking" {
		t.Fatalf("response message = %+v, want assistant text", resp.Message)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.Name != "search" || string(call.Arguments) != `{"query":"agent runtime"}` {
		t.Fatalf("ToolCall = %+v, want search args", call)
	}
	if resp.StopReason != "stop" {
		t.Fatalf("StopReason = %q, want stop", resp.StopReason)
	}
	if resp.FinishReason != FinishReasonToolCall {
		t.Fatalf("FinishReason = %q, want %q", resp.FinishReason, FinishReasonToolCall)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 13 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("Usage = %+v, want 5/13/18", resp.Usage)
	}
}

// TestOllamaFinishReasonNormalization 은 done_reason과 tool call 유무 조합을 공통 완료 사유로 옮기고,
// 잘린 응답은 tool call이 있어도 length limit으로 분류하는지 확인한다.
func TestOllamaFinishReasonNormalization(t *testing.T) {
	tests := []struct {
		name         string
		doneReason   string
		hasToolCalls bool
		want         FinishReason
	}{
		{name: "complete", doneReason: "stop", want: FinishReasonComplete},
		{name: "tool call", doneReason: "stop", hasToolCalls: true, want: FinishReasonToolCall},
		{name: "length", doneReason: "length", want: FinishReasonLengthLimit},
		{name: "length with tool call", doneReason: "length", hasToolCalls: true, want: FinishReasonLengthLimit},
		{name: "load", doneReason: "load", want: FinishReasonUnknown},
		{name: "unload", doneReason: "unload", want: FinishReasonUnknown},
		{name: "empty", want: FinishReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOllamaFinishReason(tt.doneReason, tt.hasToolCalls); got != tt.want {
				t.Fatalf(
					"normalizeOllamaFinishReason(%q, %v) = %q, want %q",
					tt.doneReason,
					tt.hasToolCalls,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestNewOllamaClientRejectsMissingRequiredConfig 는 Ollama 호출 전에 model과 host 누락을 설정 오류로 거절하는지 확인한다.
func TestNewOllamaClientRejectsMissingRequiredConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProviderConfig
	}{
		{name: "model", cfg: ProviderConfig{Host: "http://localhost:11434"}},
		{name: "host", cfg: ProviderConfig{Model: "ollama-test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOllamaClient(tt.cfg)
			if err == nil {
				t.Fatal("NewOllamaClient() error = nil, want config error")
			}
			if !IsKind(err, ErrorKindConfig) {
				t.Fatalf("NewOllamaClient() error kind mismatch: %v", err)
			}
		})
	}
}

// TestOllamaClientHTTPErrorUsesProviderError 는 Ollama 오류 응답을 provider 오류로 분류하고 메시지를 보존하는지 확인한다.
func TestOllamaClientHTTPErrorUsesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{
		Model: "ollama-test",
		Host:  server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{Messages: []message.Message{message.User("hello")}})
	if err == nil {
		t.Fatal("Chat() error = nil, want provider error")
	}
	if !IsKind(err, ErrorKindProvider) {
		t.Fatalf("Chat() error kind mismatch: %v", err)
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("Chat() error = %v, want provider message", err)
	}
}

// TestOllamaClientTimeoutUsesTimeoutErrorKind 는 context deadline 초과가 일반 provider 오류가 아니라 timeout 오류로 분류되는지 확인한다.
func TestOllamaClientTimeoutUsesTimeoutErrorKind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{
		Model: "ollama-test",
		Host:  server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	_, err = client.Chat(ctx, ChatRequest{Messages: []message.Message{message.User("hello")}})
	if err == nil {
		t.Fatal("Chat() error = nil, want timeout error")
	}
	if !IsKind(err, ErrorKindTimeout) {
		t.Fatalf("Chat() error kind mismatch: %v", err)
	}
}

// TestOllamaStreamChatAssemblesTextDeltasAndFinalResponse 는 NDJSON stream이 text delta를 생성 순서대로 내보낸 뒤
// 완성 응답을 정확히 한 번 반환하고, usage와 done reason이 반영되는지 확인한다.
func TestOllamaStreamChatAssemblesTextDeltasAndFinalResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"Hello, "},"done":false}`)
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"world!"},"done":false}`)
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":9}`)
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectOllamaStreamEvents(t, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if streamErr != nil {
		t.Fatalf("StreamChat() error = %v", streamErr)
	}
	if got := strings.Join(deltas, ""); got != "Hello, world!" {
		t.Fatalf("deltas joined = %q, want %q", got, "Hello, world!")
	}
	if len(deltas) != 2 {
		t.Fatalf("len(deltas) = %d, want 2 delta events in order", len(deltas))
	}
	if resp == nil {
		t.Fatal("resp = nil, want completed response")
	}
	if resp.Model != "ollama-stream" || resp.Message.Text != "Hello, world!" {
		t.Fatalf("resp = %+v, want ollama-stream model and joined text", resp)
	}
	if resp.FinishReason != FinishReasonComplete || resp.StopReason != "stop" {
		t.Fatalf("resp finish/stop = %q/%q, want complete/stop", resp.FinishReason, resp.StopReason)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 9 || resp.Usage.TotalTokens != 14 {
		t.Fatalf("resp usage = %+v, want 5/9/14", resp.Usage)
	}
}

// TestOllamaStreamChatAssemblesToolCallsAcrossChunks 는 여러 chunk에 걸쳐 도착한 완성 Tool call을 도착 순서대로
// 모아 완성 응답에 반영하는지 확인한다.
func TestOllamaStreamChatAssemblesToolCallsAcrossChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"search","arguments":{"query":"docs"}}}]},"done":false}`)
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"lookup","arguments":{"id":1}}}]},"done":false}`)
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":2,"eval_count":3}`)
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectOllamaStreamEvents(t, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("search docs")}}))
	if streamErr != nil {
		t.Fatalf("StreamChat() error = %v", streamErr)
	}
	if len(deltas) != 0 {
		t.Fatalf("deltas = %v, want no text delta for tool-only response", deltas)
	}
	if resp == nil {
		t.Fatal("resp = nil, want completed response")
	}
	if len(resp.Message.ToolCalls) != 2 {
		t.Fatalf("len(ToolCalls) = %d, want 2", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].Name != "search" || string(resp.Message.ToolCalls[0].Arguments) != `{"query":"docs"}` {
		t.Fatalf("ToolCalls[0] = %+v, want search {\"query\":\"docs\"}", resp.Message.ToolCalls[0])
	}
	if resp.Message.ToolCalls[1].Name != "lookup" || string(resp.Message.ToolCalls[1].Arguments) != `{"id":1}` {
		t.Fatalf("ToolCalls[1] = %+v, want lookup {\"id\":1}", resp.Message.ToolCalls[1])
	}
	if resp.FinishReason != FinishReasonToolCall {
		t.Fatalf("FinishReason = %q, want %q", resp.FinishReason, FinishReasonToolCall)
	}
}

// TestOllamaStreamChatReturnsProviderErrorOnDecodeFailure 는 NDJSON chunk 하나가 유효한 JSON이 아니면 이미
// 전달된 delta와 별개로 provider 오류로 종료되고 완성 응답을 만들지 않는지 확인한다.
func TestOllamaStreamChatReturnsProviderErrorOnDecodeFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"partial"},"done":false}`)
		writeNDJSONChunk(w, `{not valid json`)
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectOllamaStreamEvents(t, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if len(deltas) != 1 || deltas[0] != "partial" {
		t.Fatalf("deltas = %v, want [partial] delivered before decode failure", deltas)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil after decode failure", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestOllamaStreamChatReturnsProviderErrorOnAbnormalEOF 는 done:true chunk 없이 연결이 끝나는 불완전 stream이
// 성공 응답으로 숨겨지지 않는지 확인한다.
func TestOllamaStreamChatReturnsProviderErrorOnAbnormalEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"partial"},"done":false}`)
		// 연결을 여기서 그냥 끝내 done:true 없는 EOF를 재현한다.
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	_, resp, streamErr := collectOllamaStreamEvents(t, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for abnormal EOF", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestOllamaStreamChatHTTPErrorBeforeBodyStreaming 는 streaming 요청도 non-2xx status를 body streaming 전에
// 기존 HTTP 오류 처리로 반환하는지 확인한다.
func TestOllamaStreamChatHTTPErrorBeforeBodyStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectOllamaStreamEvents(t, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if len(deltas) != 0 || resp != nil {
		t.Fatalf("deltas/resp = %v/%v, want nothing on HTTP error", deltas, resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
	if !strings.Contains(streamErr.Error(), "model not found") {
		t.Fatalf("streamErr = %v, want message to contain provider error text", streamErr)
	}
}

// TestOllamaStreamChatTimeoutUsesTimeoutErrorKind 는 stream body 수신 도중 context deadline이 지나면
// timeout 오류로 분류되는지 확인한다.
func TestOllamaStreamChatTimeoutUsesTimeoutErrorKind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"partial"},"done":false}`)
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, resp, streamErr := collectOllamaStreamEvents(t, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if resp != nil {
		t.Fatalf("resp = %+v, want nil on timeout", resp)
	}
	if streamErr == nil {
		t.Fatal("streamErr = nil, want timeout error")
	}
	if !IsKind(streamErr, ErrorKindTimeout) {
		t.Fatalf("streamErr kind mismatch: %v", streamErr)
	}
}

// TestOllamaStreamChatContextCancellationEndsWithProviderError 는 명시적으로 취소한 context가 stream 수신
// 도중 provider 오류로 종료되고 성공 응답을 만들지 않는지 확인한다(SPEC §5.11).
func TestOllamaStreamChatContextCancellationEndsWithProviderError(t *testing.T) {
	firstChunkSent := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"partial"},"done":false}`)
		close(firstChunkSent)
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-firstChunkSent
		cancel()
	}()

	_, resp, streamErr := collectOllamaStreamEvents(t, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if resp != nil {
		t.Fatalf("resp = %+v, want nil after context cancellation", resp)
	}
	if streamErr == nil {
		t.Fatal("streamErr = nil, want error after context cancellation")
	}
	if !IsKind(streamErr, ErrorKindProvider) && !IsKind(streamErr, ErrorKindTimeout) {
		t.Fatalf("streamErr kind mismatch: %v, want provider or timeout error", streamErr)
	}
}

// TestOllamaStreamChatStopsWithoutFurtherEventsWhenConsumerBreaks 는 consumer가 순회를 중단하면 추가 event 없이
// 요청이 정리되어 handler의 request context가 취소되는지 확인한다(SPEC §5.11).
func TestOllamaStreamChatStopsWithoutFurtherEventsWhenConsumerBreaks(t *testing.T) {
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":"first"},"done":false}`)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	seq := streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}})

	var deltaCount int
	for event, err := range seq {
		if err != nil {
			t.Fatalf("StreamChat() error = %v, want no error before break", err)
		}
		if event.Kind == ChatStreamEventTextDelta {
			deltaCount++
		}
		break
	}
	if deltaCount != 1 {
		t.Fatalf("deltaCount = %d, want 1 delta before break", deltaCount)
	}

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("server did not observe request context cancellation after consumer break")
	}
}

// TestOllamaStreamChatRequestBodyCarriesStreamTrue 는 streaming 요청 본문에 stream:true가 실려 나가는지 확인한다.
func TestOllamaStreamChatRequestBodyCarriesStreamTrue(t *testing.T) {
	var gotStream bool
	var decodeErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaChatRequest
		decodeErr = json.NewDecoder(r.Body).Decode(&req)
		gotStream = req.Stream
		w.Header().Set("Content-Type", "application/json")
		writeNDJSONChunk(w, `{"model":"ollama-stream","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}`)
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{Model: "ollama-test", Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	_, _, streamErr := collectOllamaStreamEvents(t, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if streamErr != nil {
		t.Fatalf("StreamChat() error = %v", streamErr)
	}
	if decodeErr != nil {
		t.Fatalf("decode request body: %v", decodeErr)
	}
	if !gotStream {
		t.Fatal("request body Stream = false, want true for StreamChat")
	}
}
