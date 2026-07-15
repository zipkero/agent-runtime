package tool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearchExecutesSearch(t *testing.T) {
	client := &fakeTavilySearchClient{
		response: tavilySearchResponse{
			Query:     "golang testing",
			Answer:    "Use go test.",
			RequestID: "req-123",
			Results: []tavilySearchResult{
				{
					Title:   "Go testing",
					URL:     "https://go.dev/doc/tutorial/add-a-test",
					Content: "The go test command executes tests.",
					Score:   0.93,
				},
			},
		},
	}
	webSearch := newWebSearch("tvly-test", client)
	args := json.RawMessage(`{"query":"golang testing","max_results":3}`)

	if err := webSearch.Validate(args); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	got, err := webSearch.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("client requests = %d, want 1", len(client.requests))
	}
	request := client.requests[0]
	if request.Query != "golang testing" {
		t.Fatalf("request query = %q, want golang testing", request.Query)
	}
	if request.MaxResults != 3 {
		t.Fatalf("request max_results = %d, want 3", request.MaxResults)
	}
	if request.SearchDepth != "basic" || request.Topic != "general" {
		t.Fatalf("request defaults = (%q, %q), want basic general", request.SearchDepth, request.Topic)
	}
	if !request.IncludeAnswer || request.IncludeRawContent {
		t.Fatalf("request include flags = (%v, %v), want true false", request.IncludeAnswer, request.IncludeRawContent)
	}

	var content webSearchContent
	if err := json.Unmarshal([]byte(got.Content), &content); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if content.Query != "golang testing" || content.Answer != "Use go test." || content.RequestID != "req-123" {
		t.Fatalf("content metadata = %+v, want query answer request_id", content)
	}
	if len(content.Results) != 1 {
		t.Fatalf("content results = %d, want 1", len(content.Results))
	}
	result := content.Results[0]
	if result.Title != "Go testing" || result.URL == "" || result.Content == "" || result.Score != 0.93 {
		t.Fatalf("content result = %+v, want source metadata", result)
	}
}

func TestWebSearchRejectsInvalidArguments(t *testing.T) {
	webSearch := newWebSearch("tvly-test", &fakeTavilySearchClient{})
	longQuery := strings.Repeat("가", maxWebSearchQueryLength+1)
	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "invalid json", args: json.RawMessage(`{"query":`)},
		{name: "missing query", args: json.RawMessage(`{}`)},
		{name: "empty query", args: json.RawMessage(`{"query":" "}`)},
		{name: "long query", args: json.RawMessage(`{"query":` + quoteJSON(t, longQuery) + `}`)},
		{name: "zero max results", args: json.RawMessage(`{"query":"go","max_results":0}`)},
		{name: "wrong max results type", args: json.RawMessage(`{"query":"go","max_results":"3"}`)},
		{name: "unsupported depth", args: json.RawMessage(`{"query":"go","search_depth":"deep"}`)},
		{name: "unsupported topic", args: json.RawMessage(`{"query":"go","topic":"sports"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := webSearch.Validate(tt.args)
			if !IsValidationError(err) {
				t.Fatalf("Validate() error = %v, want validation error", err)
			}
		})
	}
}

func TestWebSearchReturnsConfigurationErrorForMissingAPIKey(t *testing.T) {
	webSearch := newWebSearch(" ", &fakeTavilySearchClient{})

	_, err := webSearch.Execute(context.Background(), json.RawMessage(`{"query":"go"}`))
	if !IsConfigurationError(err) {
		t.Fatalf("Execute() error = %v, want configuration error", err)
	}
}

func TestWebSearchReturnsExecutionErrorForProviderFailure(t *testing.T) {
	webSearch := newWebSearch("tvly-test", &fakeTavilySearchClient{
		err: errors.New("provider unavailable"),
	})

	_, err := webSearch.Execute(context.Background(), json.RawMessage(`{"query":"go"}`))
	if !IsExecutionError(err) {
		t.Fatalf("Execute() error = %v, want execution error", err)
	}
}

func TestWebSearchReturnsExecutionErrorWhenContextCanceled(t *testing.T) {
	client := &fakeTavilySearchClient{}
	webSearch := newWebSearch("tvly-test", client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := webSearch.Execute(ctx, json.RawMessage(`{"query":"go"}`))
	if !IsExecutionError(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want canceled execution error", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("client requests = %d, want 0", len(client.requests))
	}
}

func TestWebSearchSchemaMatchesToolIdentity(t *testing.T) {
	webSearch := NewWebSearch("tvly-test")
	schema := webSearch.Schema()

	if webSearch.Name() != "web_search" {
		t.Fatalf("Name() = %q, want web_search", webSearch.Name())
	}
	if schema.Name != webSearch.Name() {
		t.Fatalf("Schema().Name = %q, want %q", schema.Name, webSearch.Name())
	}
	if schema.Description == "" || webSearch.Description() == "" {
		t.Fatal("description must not be empty")
	}
	if !json.Valid(schema.InputSchema) {
		t.Fatalf("InputSchema is not valid JSON: %s", schema.InputSchema)
	}
}

func TestTavilyHTTPClientSendsSearchRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tvly-test" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q, want application/json", got)
		}

		var request tavilySearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if request.Query != "go docs" || request.MaxResults != 2 || !request.IncludeAnswer || request.IncludeRawContent {
			t.Fatalf("request = %+v, want Tavily search payload", request)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"go docs","answer":"Go docs answer","results":[{"title":"Go","url":"https://go.dev","content":"Docs","score":0.8}],"request_id":"req-http"}`))
	}))
	defer server.Close()

	client := newTavilyHTTPClient(server.URL, server.Client())
	got, err := client.Search(context.Background(), "tvly-test", tavilySearchRequest{
		Query:             "go docs",
		SearchDepth:       "basic",
		MaxResults:        2,
		Topic:             "general",
		IncludeAnswer:     true,
		IncludeRawContent: false,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if got.Query != "go docs" || got.Answer != "Go docs answer" || got.RequestID != "req-http" {
		t.Fatalf("Search() = %+v, want response metadata", got)
	}
	if len(got.Results) != 1 || got.Results[0].Title != "Go" || got.Results[0].Score != 0.8 {
		t.Fatalf("Search() results = %+v, want Tavily results", got.Results)
	}
}

func TestTavilyHTTPClientReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":{"error":"Unauthorized: missing or invalid API key."}}`))
	}))
	defer server.Close()

	client := newTavilyHTTPClient(server.URL, server.Client())
	_, err := client.Search(context.Background(), "bad-key", tavilySearchRequest{Query: "go", MaxResults: 1})
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("Search() error = %v, want provider error detail", err)
	}
}

func TestTavilyHTTPClientRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":`))
	}))
	defer server.Close()

	client := newTavilyHTTPClient(server.URL, server.Client())
	_, err := client.Search(context.Background(), "tvly-test", tavilySearchRequest{Query: "go", MaxResults: 1})
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("Search() error = %v, want malformed response error", err)
	}
}

func TestTavilyHTTPClientLimitsResponseBytesWhileReading(t *testing.T) {
	prefix := `{"query":"go","answer":"`
	suffix := `","results":[]}`
	tests := []struct {
		name      string
		size      int
		wantError bool
	}{
		{name: "at limit", size: DefaultMaxResultBytes},
		{name: "over limit", size: DefaultMaxResultBytes + 1, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			padding := tt.size - len(prefix) - len(suffix)
			body := prefix + strings.Repeat("a", padding) + suffix
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			client := newTavilyHTTPClient(server.URL, server.Client())
			result, err := client.Search(context.Background(), "tvly-test", tavilySearchRequest{Query: "go", MaxResults: 1})
			if tt.wantError {
				if err == nil || !strings.Contains(err.Error(), "byte limit") {
					t.Fatalf("Search() error = %v, want response size error", err)
				}
				return
			}
			if err != nil || len(result.Answer) != padding {
				t.Fatalf("Search() answer/error = %d/%v, want %d bytes", len(result.Answer), err, padding)
			}
		})
	}
}

type fakeTavilySearchClient struct {
	response tavilySearchResponse
	err      error
	requests []tavilySearchRequest
}

func (f *fakeTavilySearchClient) Search(_ context.Context, _ string, request tavilySearchRequest) (tavilySearchResponse, error) {
	f.requests = append(f.requests, request)
	if f.err != nil {
		return tavilySearchResponse{}, f.err
	}
	return f.response, nil
}
