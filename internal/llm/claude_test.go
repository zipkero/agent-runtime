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

// writeSSEEvent 함수는 테스트 서버에서 SSE event 하나를 즉시 flush해 내보낸다.
func writeSSEEvent(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// newSSEServer 함수는 handler가 자유롭게 SSE event를 쓸 수 있도록 streaming 응답 header를 미리 설정한 test server를 만든다.
func newSSEServer(handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		handler(w, r)
	}))
}

func collectStreamEvents(t *testing.T, ctx context.Context, seq func(func(ChatStreamEvent, error) bool)) ([]string, *ChatResponse, error) {
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

// TestClaudeStreamChatAssemblesTextDeltasAndFinalResponse 는 Claude SSE stream이 text delta를 생성 순서대로 내보낸 뒤
// 완성 응답을 정확히 한 번 반환하고, usage와 stop reason이 정규화되는지 확인한다.
func TestClaudeStreamChatAssemblesTextDeltasAndFinalResponse(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":5}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, "ping", `{"type":"ping"}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello, "}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world!"}}`)
		writeSSEEvent(w, "future_event", `{"type":"future_event","foo":"bar"}`)
		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`)
		writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
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
	if resp.Model != "claude-stream" || resp.Message.Text != "Hello, world!" {
		t.Fatalf("resp = %+v, want claude-stream model and joined text", resp)
	}
	if resp.FinishReason != FinishReasonComplete || resp.StopReason != "end_turn" {
		t.Fatalf("resp finish/stop = %q/%q, want complete/end_turn", resp.FinishReason, resp.StopReason)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 9 || resp.Usage.TotalTokens != 14 {
		t.Fatalf("resp usage = %+v, want 5/9/14", resp.Usage)
	}
}

// TestClaudeStreamChatAssemblesToolCallAfterContentBlockStop 는 tool_use content block의 부분 JSON이
// content_block_stop 이후에만 완성된 ToolCall이 되는지 확인한다.
func TestClaudeStreamChatAssemblesToolCallAfterContentBlockStop(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":3}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"search","input":{}}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"que"}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"ry\":\"docs\"}"}}`)
		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`)
		writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("search docs")}}))
	if streamErr != nil {
		t.Fatalf("StreamChat() error = %v", streamErr)
	}
	if len(deltas) != 0 {
		t.Fatalf("deltas = %v, want no text delta for tool-only response", deltas)
	}
	if resp == nil {
		t.Fatal("resp = nil, want completed response")
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.ID != "toolu_1" || call.Name != "search" || string(call.Arguments) != `{"query":"docs"}` {
		t.Fatalf("ToolCall = %+v, want toolu_1 search {\"query\":\"docs\"}", call)
	}
	if resp.FinishReason != FinishReasonToolCall {
		t.Fatalf("FinishReason = %q, want %q", resp.FinishReason, FinishReasonToolCall)
	}
}

// TestClaudeStreamChatHandlesMultiLineDataField 는 SSE data 필드가 여러 줄로 나뉘어도 개행으로 합쳐 하나의 JSON으로 해석하는지 확인한다.
func TestClaudeStreamChatHandlesMultiLineDataField(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\n")
		fmt.Fprint(w, "data: \"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
		writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if streamErr != nil {
		t.Fatalf("StreamChat() error = %v", streamErr)
	}
	if got := strings.Join(deltas, ""); got != "hi" {
		t.Fatalf("deltas joined = %q, want %q", got, "hi")
	}
	if resp == nil || resp.Message.Text != "hi" {
		t.Fatalf("resp = %+v, want text hi", resp)
	}
}

// TestClaudeStreamChatReturnsProviderErrorOnStreamErrorEvent 는 SSE error event가 이미 전달된 delta와 별개로
// provider 오류로 종료되고 완성 응답을 만들지 않는지 확인한다.
func TestClaudeStreamChatReturnsProviderErrorOnStreamErrorEvent(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`)
		writeSSEEvent(w, "error", `{"type":"error","error":{"type":"overloaded_error","message":"boom"}}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if len(deltas) != 1 || deltas[0] != "partial" {
		t.Fatalf("deltas = %v, want [partial] delivered before error", deltas)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil after stream error", resp)
	}
	if streamErr == nil {
		t.Fatal("streamErr = nil, want provider error")
	}
	if !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr kind mismatch: %v", streamErr)
	}
	if !strings.Contains(streamErr.Error(), "boom") {
		t.Fatalf("streamErr = %v, want message to contain boom", streamErr)
	}
}

// TestClaudeStreamChatRejectsInvalidToolJSON 는 content_block_stop 시점에 부분 Tool JSON이 완성되지 않으면
// provider 오류로 종료하는지 확인한다.
func TestClaudeStreamChatRejectsInvalidToolJSON(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"search","input":{}}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}`)
		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	_, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for invalid tool json", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestClaudeStreamChatReturnsProviderErrorOnAbnormalEOF 는 message_stop 없이 연결이 끝나는 불완전 stream이
// 성공 응답으로 숨겨지지 않는지 확인한다.
func TestClaudeStreamChatReturnsProviderErrorOnAbnormalEOF(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`)
		// 연결을 여기서 그냥 끝내 message_stop 없는 EOF를 재현한다.
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	_, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for abnormal EOF", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestClaudeStreamChatTimeoutUsesTimeoutErrorKind 는 stream body 수신 도중 context deadline이 지나면
// timeout 오류로 분류되는지 확인한다.
func TestClaudeStreamChatTimeoutUsesTimeoutErrorKind(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		<-r.Context().Done()
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
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

// TestClaudeStreamChatRejectsDeltaBeforeContentBlockStart 는 content_block_start 없이 도착한
// content_block_delta가 무음으로 버려지지 않고 provider 오류로 종료되는지 확인한다.
func TestClaudeStreamChatRejectsDeltaBeforeContentBlockStart(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lost"}}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if len(deltas) != 0 {
		t.Fatalf("deltas = %v, want no delta delivered for out-of-order content_block_delta", deltas)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for out-of-order delta", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestClaudeStreamChatRejectsDeltaTypeMismatch 는 tool_use block에 도착한 text_delta와 text block에 도착한
// input_json_delta처럼 delta type이 block 종류와 맞지 않는 경우를 provider 오류로 종료하는지 확인한다.
// 각 경우는 type 검사가 없었다면 정상 종료됐을 완전한 SSE sequence를 이어 보내, EOF 경로가 아니라 이 검사 자체가
// 오류를 냈는지 오류 메시지로 확인한다.
func TestClaudeStreamChatRejectsDeltaTypeMismatch(t *testing.T) {
	tests := []struct {
		name          string
		contentBlock  string
		delta         string
		wantErrSubstr string
	}{
		{
			name:          "text_delta on tool_use block",
			contentBlock:  `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"search","input":{}}}`,
			delta:         `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"oops"}}`,
			wantErrSubstr: "text_delta",
		},
		{
			name:          "input_json_delta on text block",
			contentBlock:  `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			delta:         `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
			wantErrSubstr: "input_json_delta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
				writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
				writeSSEEvent(w, "content_block_start", tt.contentBlock)
				writeSSEEvent(w, "content_block_delta", tt.delta)
				// type 검사가 없다면 이 나머지 sequence만으로 정상 완료된다.
				writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
				writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
				writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
			})
			defer server.Close()

			client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
			if err != nil {
				t.Fatalf("newClaudeClient() error = %v", err)
			}
			streamer := client.(StreamingLLMClient)

			ctx := context.Background()
			deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
			if len(deltas) != 0 {
				t.Fatalf("deltas = %v, want no delta delivered for mismatched delta type", deltas)
			}
			if resp != nil {
				t.Fatalf("resp = %+v, want nil for mismatched delta type", resp)
			}
			if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
				t.Fatalf("streamErr = %v, want provider error", streamErr)
			}
			if !strings.Contains(streamErr.Error(), tt.wantErrSubstr) {
				t.Fatalf("streamErr = %v, want message to contain %q", streamErr, tt.wantErrSubstr)
			}
		})
	}
}

// TestClaudeStreamChatRejectsDuplicateContentBlockStart 는 이미 등록된 index로 다시 도착한
// content_block_start가 그 block의 text를 덮어써 잘못된 본문을 가진 성공 응답이 되지 않고 provider 오류로
// 종료되는지 확인한다.
func TestClaudeStreamChatRejectsDuplicateContentBlockStart(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"AAA"}}`)
		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		// 이미 닫힌 index 0을 다시 시작한다.
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"BBB"}}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if got := strings.Join(deltas, ""); got != "AAA" {
		t.Fatalf("deltas joined = %q, want only AAA delivered before duplicate content_block_start", got)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for duplicate content_block_start", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestClaudeStreamChatRejectsContentBlockStopWithoutStart 는 content_block_start 없이 도착한
// content_block_stop이 무음으로 넘어가지 않고 provider 오류로 종료되는지 확인한다.
func TestClaudeStreamChatRejectsContentBlockStopWithoutStart(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if len(deltas) != 0 {
		t.Fatalf("deltas = %v, want no delta", deltas)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for content_block_stop without start", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestClaudeStreamChatRejectsContentBlockStartMissingContentBlock 는 content_block 필드가 없는
// content_block_start가 조용히 넘어가 빈 text 성공 응답이 되지 않고 provider 오류로 종료되는지 확인한다.
func TestClaudeStreamChatRejectsContentBlockStartMissingContentBlock(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0}`)
		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
		writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	_, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for content_block_start missing content_block", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestClaudeStreamChatRejectsContentBlockDeltaMissingDelta 는 delta 필드가 없는 content_block_delta가
// 조용히 무시되지 않고 provider 오류로 종료되는지 확인한다.
func TestClaudeStreamChatRejectsContentBlockDeltaMissingDelta(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0}`)
		writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
		writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	_, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for content_block_delta missing delta", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestClaudeStreamChatRequestBodyCarriesStreamTrue 는 streaming 요청 본문에 stream:true가 실려 나가는지 확인한다.
func TestClaudeStreamChatRequestBodyCarriesStreamTrue(t *testing.T) {
	var gotStream bool
	var decodeErr error
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		var req claudeMessageRequest
		decodeErr = json.NewDecoder(r.Body).Decode(&req)
		gotStream = req.Stream
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	_, _, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
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

// TestClaudeStreamChatRejectsMessageStopWithOpenContentBlock 는 content_block_stop 없이 message_stop이 먼저
// 도착해도 빈 ToolCall을 가진 성공 응답으로 숨기지 않고 provider 오류로 종료하는지 확인한다.
func TestClaudeStreamChatRejectsMessageStopWithOpenContentBlock(t *testing.T) {
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"search","input":{}}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"docs\"}"}}`)
		// content_block_stop 없이 곧바로 message_delta·message_stop을 보낸다.
		writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`)
		writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	_, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if resp != nil {
		t.Fatalf("resp = %+v, want nil when message_stop arrives before content_block_stop", resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
}

// TestClaudeStreamChatHTTPErrorBeforeBodyStreaming 는 streaming 요청도 non-2xx status를 body streaming 전에
// 기존 HTTP 오류 처리로 반환하는지 확인한다.
func TestClaudeStreamChatHTTPErrorBeforeBodyStreaming(t *testing.T) {
	const apiKey = "secret-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key secret-key"}}`))
	}))
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: apiKey}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx := context.Background()
	deltas, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
	if len(deltas) != 0 || resp != nil {
		t.Fatalf("deltas/resp = %v/%v, want nothing on HTTP error", deltas, resp)
	}
	if streamErr == nil || !IsKind(streamErr, ErrorKindProvider) {
		t.Fatalf("streamErr = %v, want provider error", streamErr)
	}
	if strings.Contains(streamErr.Error(), apiKey) {
		t.Fatalf("streamErr exposed API key: %v", streamErr)
	}
}

// TestClaudeStreamChatContextCancellationEndsWithProviderError 는 명시적으로 취소한 context가 stream 수신
// 도중 provider 오류로 종료되고 성공 응답을 만들지 않는지 확인한다(SPEC §5.11).
func TestClaudeStreamChatContextCancellationEndsWithProviderError(t *testing.T) {
	firstEventSent := make(chan struct{})
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		close(firstEventSent)
		<-r.Context().Done()
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}
	streamer := client.(StreamingLLMClient)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-firstEventSent
		cancel()
	}()

	_, resp, streamErr := collectStreamEvents(t, ctx, streamer.StreamChat(ctx, ChatRequest{Messages: []message.Message{message.User("hi")}}))
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

// TestClaudeStreamChatStopsWithoutFurtherEventsWhenConsumerBreaks 는 consumer가 순회를 중단하면 추가 event 없이
// 요청이 정리되어 handler의 request context가 취소되는지 확인한다(SPEC §5.11).
func TestClaudeStreamChatStopsWithoutFurtherEventsWhenConsumerBreaks(t *testing.T) {
	cancelled := make(chan struct{})
	server := newSSEServer(func(w http.ResponseWriter, r *http.Request) {
		writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"model":"claude-stream","usage":{"input_tokens":1}}}`)
		writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first"}}`)
		<-r.Context().Done()
		close(cancelled)
	})
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{Model: "claude-test", APIKey: "secret"}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
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

// TestClaudeClientSendsMessagesHeadersAndDecodesResponse 는 Claude adapter가 내부 요청을 Messages API 형태로 변환하고 응답 tool_use를 보존하는지 확인한다.
func TestClaudeClientSendsMessagesHeadersAndDecodesResponse(t *testing.T) {
	const apiKey = "secret-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Method = %s, want POST", r.Method)
		}
		if r.URL.Path != claudeMessagesPath {
			t.Fatalf("Path = %s, want %s", r.URL.Path, claudeMessagesPath)
		}
		if got := r.Header.Get("x-api-key"); got != apiKey {
			t.Fatalf("x-api-key = %q, want api key", got)
		}
		if got := r.Header.Get("anthropic-version"); got != claudeAPIVersion {
			t.Fatalf("anthropic-version = %q, want %q", got, claudeAPIVersion)
		}
		if got := r.Header.Get("Content-Type"); got != claudeRequestMediaType {
			t.Fatalf("Content-Type = %q, want %q", got, claudeRequestMediaType)
		}

		var req claudeMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		if req.Model != "claude-test" {
			t.Fatalf("Model = %q, want claude-test", req.Model)
		}
		if req.MaxTokens != claudeDefaultMaxTokens {
			t.Fatalf("MaxTokens = %d, want %d", req.MaxTokens, claudeDefaultMaxTokens)
		}
		if req.System != "runtime rule" {
			t.Fatalf("System = %q, want runtime rule", req.System)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("len(Messages) = %d, want 2", len(req.Messages))
		}
		if len(req.Tools) != 1 {
			t.Fatalf("len(Tools) = %d, want 1", len(req.Tools))
		}
		if req.Tools[0].Name != "search" || req.Tools[0].Description != "Search documents" {
			t.Fatalf("tool = %+v, want search schema", req.Tools[0])
		}
		if string(req.Tools[0].InputSchema) != `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}` {
			t.Fatalf("tool input_schema = %s, want query schema", req.Tools[0].InputSchema)
		}
		if req.Messages[0].Role != "user" || req.Messages[0].Content[0].Text != "hello" {
			t.Fatalf("first message = %+v, want user text", req.Messages[0])
		}
		if req.Messages[1].Role != "assistant" {
			t.Fatalf("second role = %q, want assistant", req.Messages[1].Role)
		}
		if len(req.Messages[1].Content) != 2 {
			t.Fatalf("assistant content len = %d, want text and tool_use", len(req.Messages[1].Content))
		}
		if req.Messages[1].Content[1].Type != "tool_use" || req.Messages[1].Content[1].Name != "search" {
			t.Fatalf("assistant tool_use block = %+v, want search tool_use", req.Messages[1].Content[1])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"claude-test",
			"role":"assistant",
			"stop_reason":"tool_use",
			"usage":{"input_tokens":7,"output_tokens":11},
			"content":[
				{"type":"text","text":"checking"},
				{"type":"tool_use","id":"toolu_1","name":"search","input":{"query":"agent runtime"}}
			]
		}`))
	}))
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{
		Model:  "claude-test",
		APIKey: apiKey,
	}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
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
				ID:        "toolu_previous",
				Name:      "search",
				Arguments: json.RawMessage(`{"query":"before"}`),
			}),
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if resp.Provider != ProviderClaude || resp.Model != "claude-test" {
		t.Fatalf("response provider/model = %q/%q, want claude/claude-test", resp.Provider, resp.Model)
	}
	if resp.Message.Role != message.RoleAssistant || resp.Message.Text != "checking" {
		t.Fatalf("response message = %+v, want assistant text", resp.Message)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.ID != "toolu_1" || call.Name != "search" || string(call.Arguments) != `{"query":"agent runtime"}` {
		t.Fatalf("ToolCall = %+v, want toolu_1 search args", call)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("StopReason = %q, want tool_use", resp.StopReason)
	}
	if resp.FinishReason != FinishReasonToolCall {
		t.Fatalf("FinishReason = %q, want %q", resp.FinishReason, FinishReasonToolCall)
	}
	if resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 11 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("Usage = %+v, want 7/11/18", resp.Usage)
	}
}

// TestClaudeFinishReasonNormalization 은 Claude stop_reason을 Runtime 공통 완료 사유로 옮기고,
// 새로 생기거나 비어 있는 사유는 unknown으로 남겨 정상 완료로 오해하지 않는지 확인한다.
func TestClaudeFinishReasonNormalization(t *testing.T) {
	tests := []struct {
		stopReason string
		want       FinishReason
	}{
		{stopReason: "end_turn", want: FinishReasonComplete},
		{stopReason: "stop_sequence", want: FinishReasonComplete},
		{stopReason: "tool_use", want: FinishReasonToolCall},
		{stopReason: "max_tokens", want: FinishReasonLengthLimit},
		{stopReason: "refusal", want: FinishReasonBlocked},
		{stopReason: "pause_turn", want: FinishReasonUnknown},
		{stopReason: "future_reason", want: FinishReasonUnknown},
		{want: FinishReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.stopReason, func(t *testing.T) {
			if got := normalizeClaudeFinishReason(tt.stopReason); got != tt.want {
				t.Fatalf("normalizeClaudeFinishReason(%q) = %q, want %q", tt.stopReason, got, tt.want)
			}
		})
	}
}

// TestNewClaudeClientRejectsMissingRequiredConfig 는 Claude 호출 전에 model과 API key 누락을 설정 오류로 거절하는지 확인한다.
func TestNewClaudeClientRejectsMissingRequiredConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProviderConfig
	}{
		{name: "model", cfg: ProviderConfig{APIKey: "secret"}},
		{name: "api key", cfg: ProviderConfig{Model: "claude-test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClaudeClient(tt.cfg)
			if err == nil {
				t.Fatal("NewClaudeClient() error = nil, want config error")
			}
			if !IsKind(err, ErrorKindConfig) {
				t.Fatalf("NewClaudeClient() error kind mismatch: %v", err)
			}
		})
	}
}

// TestClaudeClientHTTPErrorDoesNotExposeAPIKey 는 provider 오류 메시지에 API key가 섞여 들어가도 외부 오류에서는 마스킹되는지 확인한다.
func TestClaudeClientHTTPErrorDoesNotExposeAPIKey(t *testing.T) {
	const apiKey = "secret-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key secret-key"}}`))
	}))
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{
		Model:  "claude-test",
		APIKey: apiKey,
	}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{Messages: []message.Message{message.User("hello")}})
	if err == nil {
		t.Fatal("Chat() error = nil, want provider error")
	}
	if !IsKind(err, ErrorKindProvider) {
		t.Fatalf("Chat() error kind mismatch: %v", err)
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("Chat() error exposed API key: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("Chat() error = %v, want redacted marker", err)
	}
}

// TestClaudeClientTimeoutUsesTimeoutErrorKind 는 context deadline 초과가 일반 provider 오류가 아니라 timeout 오류로 분류되는지 확인한다.
func TestClaudeClientTimeoutUsesTimeoutErrorKind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{
		Model:  "claude-test",
		APIKey: "secret",
	}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
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
