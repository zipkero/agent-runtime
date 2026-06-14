package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

type fakeTavilyClient struct {
	gotReq tool.TavilySearchRequest
	resp   tool.TavilySearchResponse
	err    error
}

func (f *fakeTavilyClient) Search(_ context.Context, req tool.TavilySearchRequest) (tool.TavilySearchResponse, error) {
	f.gotReq = req
	if f.err != nil {
		return tool.TavilySearchResponse{}, f.err
	}
	return f.resp, nil
}

type waitForCancelTavilyClient struct{}

func (w waitForCancelTavilyClient) Search(ctx context.Context, _ tool.TavilySearchRequest) (tool.TavilySearchResponse, error) {
	<-ctx.Done()
	return tool.TavilySearchResponse{}, ctx.Err()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func webSearchInput(values map[string]any) json.RawMessage {
	raw, _ := json.Marshal(values)
	return raw
}

func TestWebSearch_Spec(t *testing.T) {
	ws := tool.NewWebSearch("tvly-test", &fakeTavilyClient{})
	spec := ws.Spec()

	if spec.Name != "web_search" {
		t.Errorf("Spec().Name = %q, want %q", spec.Name, "web_search")
	}
	if spec.Description == "" {
		t.Error("Spec().Description이 비어 있습니다")
	}
	if len(spec.InputSchema) == 0 {
		t.Error("Spec().InputSchema가 비어 있습니다")
	}
}

func TestWebSearch_SearchSuccess(t *testing.T) {
	fake := &fakeTavilyClient{
		resp: tool.TavilySearchResponse{
			Answer: "Agent runtime summary",
			Results: []tool.TavilySearchResult{
				{
					Title:   "Agent Runtime",
					URL:     "https://example.com/agent-runtime",
					Content: "Runtime details for agents.",
					Score:   0.92,
				},
			},
		},
	}
	ws := tool.NewWebSearch("tvly-test", fake)

	result, err := ws.Execute(context.Background(), webSearchInput(map[string]any{
		"query":        " agent runtime ",
		"search_depth": "basic",
		"max_results":  3,
		"topic":        "general",
	}))
	if err != nil {
		t.Fatalf("예상치 못한 error: %v", err)
	}
	if result.IsError {
		t.Fatalf("IsError=true, Content=%q", result.Content)
	}
	if fake.gotReq.Query != "agent runtime" {
		t.Errorf("Query = %q, want %q", fake.gotReq.Query, "agent runtime")
	}
	if fake.gotReq.SearchDepth != "basic" {
		t.Errorf("SearchDepth = %q, want %q", fake.gotReq.SearchDepth, "basic")
	}
	if fake.gotReq.MaxResults != 3 {
		t.Errorf("MaxResults = %d, want %d", fake.gotReq.MaxResults, 3)
	}
	for _, want := range []string{"Agent runtime summary", "Agent Runtime", "https://example.com/agent-runtime", "Runtime details"} {
		if !strings.Contains(result.Content, want) {
			t.Errorf("Content가 %q를 포함해야 합니다: %q", want, result.Content)
		}
	}
}

func TestWebSearch_MissingAPIKey(t *testing.T) {
	ws := tool.NewWebSearch("", &fakeTavilyClient{})

	_, err := ws.Execute(context.Background(), webSearchInput(map[string]any{"query": "agent runtime"}))
	if err == nil {
		t.Fatal("TAVILY_API_KEY 누락에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

func TestWebSearch_APIError(t *testing.T) {
	ws := tool.NewWebSearch("tvly-test", &fakeTavilyClient{err: errors.New("remote API failed")})

	_, err := ws.Execute(context.Background(), webSearchInput(map[string]any{"query": "agent runtime"}))
	if err == nil {
		t.Fatal("Tavily client 실패에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
	if !strings.Contains(err.Error(), "remote API failed") {
		t.Fatalf("error = %q, want remote API failed 포함", err.Error())
	}
}

func TestWebSearch_ContextCancelViaDispatcher(t *testing.T) {
	d := newDispatcher(10*time.Millisecond, tool.NewWebSearch("tvly-test", waitForCancelTavilyClient{}))
	call := message.ToolCall{
		ID:    "web-1",
		Name:  "web_search",
		Input: webSearchInput(map[string]any{"query": "agent runtime"}),
	}

	result := d.Dispatch(context.Background(), call)
	if !result.IsError {
		t.Fatal("context deadline 초과가 IsError=true로 정규화되어야 하는데 false를 반환했습니다")
	}
	if result.ToolCallID != call.ID {
		t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
	}
}

func TestWebSearch_HTTPClientSuccess(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", r.Method, http.MethodPost)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tvly-test" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer tvly-test")
		}
		var req tool.TavilySearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("request decode 실패: %v", err)
		}
		if req.Query != "agent runtime" {
			t.Errorf("Query = %q, want %q", req.Query, "agent runtime")
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
			"query": "agent runtime",
			"results": [
				{"title": "Runtime Doc", "url": "https://example.com/runtime", "content": "Runtime content", "score": 0.81}
			]
		}`)),
		}, nil
	})}

	client := tool.NewHTTPTavilyClient("tvly-test", httpClient)

	resp, err := client.Search(context.Background(), tool.TavilySearchRequest{Query: "agent runtime"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("len(Results) = %d, want %d", len(resp.Results), 1)
	}
	if resp.Results[0].Title != "Runtime Doc" {
		t.Errorf("Title = %q, want %q", resp.Results[0].Title, "Runtime Doc")
	}
}

func TestWebSearch_HTTPClientNon2xx(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("bad tavily request")),
		}, nil
	})}
	client := tool.NewHTTPTavilyClient("tvly-test", httpClient)

	_, err := client.Search(context.Background(), tool.TavilySearchRequest{Query: "agent runtime"})
	if err == nil {
		t.Fatal("non-2xx 응답에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
	if !strings.Contains(err.Error(), "status=401") {
		t.Fatalf("error = %q, want status=401 포함", err.Error())
	}
}

func TestWebSearch_InvalidInput(t *testing.T) {
	ws := tool.NewWebSearch("tvly-test", &fakeTavilyClient{})

	tests := []struct {
		name  string
		input json.RawMessage
	}{
		{"malformed JSON", json.RawMessage(`not json`)},
		{"missing query", webSearchInput(map[string]any{"query": ""})},
		{"invalid depth", webSearchInput(map[string]any{"query": "agent", "search_depth": "deep"})},
		{"invalid topic", webSearchInput(map[string]any{"query": "agent", "topic": "sports"})},
		{"invalid max results", webSearchInput(map[string]any{"query": "agent", "max_results": -1})},
		{"zero max results", webSearchInput(map[string]any{"query": "agent", "max_results": 0})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ws.Execute(context.Background(), tt.input)
			if err == nil {
				t.Fatal("잘못된 입력에서 error를 반환해야 하는데 nil을 반환했습니다")
			}
		})
	}
}
