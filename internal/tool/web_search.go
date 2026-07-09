package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

const (
	DefaultTavilySearchEndpoint = "https://api.tavily.com/search"

	defaultWebSearchMaxResults = 5
	maxWebSearchQueryLength    = 400
)

type WebSearch struct {
	apiKey string
	client tavilySearchClient
}

func NewWebSearch(apiKey string) WebSearch {
	return newWebSearch(apiKey, newTavilyHTTPClient(DefaultTavilySearchEndpoint, http.DefaultClient))
}

func newWebSearch(apiKey string, client tavilySearchClient) WebSearch {
	return WebSearch{
		apiKey: strings.TrimSpace(apiKey),
		client: client,
	}
}

func (WebSearch) Name() string {
	return "web_search"
}

func (WebSearch) Description() string {
	return "Search the web using Tavily."
}

func (WebSearch) Schema() message.ToolSchema {
	return message.ToolSchema{
		Name:        "web_search",
		Description: "Search the web using Tavily.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"max_results":{"type":"integer","minimum":1},"search_depth":{"type":"string","enum":["basic","advanced"]},"topic":{"type":"string","enum":["general","news","finance"]}},"required":["query"],"additionalProperties":false}`),
	}
}

func (w WebSearch) Validate(args json.RawMessage) error {
	_, err := decodeWebSearchArguments(args)
	return err
}

func (w WebSearch) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	arguments, err := decodeWebSearchArguments(args)
	if err != nil {
		return Result{}, err
	}
	if w.apiKey == "" {
		return Result{}, ConfigurationErrorf("Tavily API key is required")
	}
	if w.client == nil {
		return Result{}, ConfigurationErrorf("Tavily search client is required")
	}

	response, err := w.client.Search(ctx, w.apiKey, tavilySearchRequest{
		Query:             arguments.Query,
		SearchDepth:       arguments.SearchDepth,
		MaxResults:        *arguments.MaxResults,
		Topic:             arguments.Topic,
		IncludeAnswer:     true,
		IncludeRawContent: false,
	})
	if err != nil {
		return Result{}, ExecutionErrorf("Tavily search failed: %v", err)
	}

	content := webSearchContent{
		Query:     response.Query,
		Answer:    response.Answer,
		RequestID: response.RequestID,
		Results:   make([]webSearchResult, 0, len(response.Results)),
	}
	for _, result := range response.Results {
		content.Results = append(content.Results, webSearchResult{
			Title:   result.Title,
			URL:     result.URL,
			Content: result.Content,
			Score:   result.Score,
		})
	}

	encoded, err := json.Marshal(content)
	if err != nil {
		return Result{}, ExecutionErrorf("encode search result: %v", err)
	}
	return Result{Content: string(encoded)}, nil
}

type webSearchArguments struct {
	Query       string `json:"query"`
	MaxResults  *int   `json:"max_results"`
	SearchDepth string `json:"search_depth"`
	Topic       string `json:"topic"`
}

func decodeWebSearchArguments(raw json.RawMessage) (webSearchArguments, error) {
	var arguments webSearchArguments
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return webSearchArguments{}, ValidationErrorf("invalid JSON: %v", err)
	}

	arguments.Query = strings.TrimSpace(arguments.Query)
	if arguments.Query == "" {
		return webSearchArguments{}, ValidationErrorf("query is required")
	}
	if len([]rune(arguments.Query)) > maxWebSearchQueryLength {
		return webSearchArguments{}, ValidationErrorf("query must be %d characters or fewer", maxWebSearchQueryLength)
	}

	if arguments.MaxResults == nil {
		defaultMaxResults := defaultWebSearchMaxResults
		arguments.MaxResults = &defaultMaxResults
	}
	if *arguments.MaxResults < 1 {
		return webSearchArguments{}, ValidationErrorf("max_results must be at least 1")
	}

	if strings.TrimSpace(arguments.SearchDepth) == "" {
		arguments.SearchDepth = "basic"
	}
	if !isWebSearchDepth(arguments.SearchDepth) {
		return webSearchArguments{}, ValidationErrorf("unsupported search_depth %q", arguments.SearchDepth)
	}

	if strings.TrimSpace(arguments.Topic) == "" {
		arguments.Topic = "general"
	}
	if !isWebSearchTopic(arguments.Topic) {
		return webSearchArguments{}, ValidationErrorf("unsupported topic %q", arguments.Topic)
	}

	return arguments, nil
}

func isWebSearchDepth(searchDepth string) bool {
	switch searchDepth {
	case "basic", "advanced":
		return true
	default:
		return false
	}
}

func isWebSearchTopic(topic string) bool {
	switch topic {
	case "general", "news", "finance":
		return true
	default:
		return false
	}
}

type tavilySearchClient interface {
	Search(ctx context.Context, apiKey string, request tavilySearchRequest) (tavilySearchResponse, error)
}

type tavilySearchRequest struct {
	Query             string `json:"query"`
	SearchDepth       string `json:"search_depth"`
	MaxResults        int    `json:"max_results"`
	Topic             string `json:"topic"`
	IncludeAnswer     bool   `json:"include_answer"`
	IncludeRawContent bool   `json:"include_raw_content"`
}

type tavilySearchResponse struct {
	Query     string               `json:"query"`
	Answer    string               `json:"answer"`
	Results   []tavilySearchResult `json:"results"`
	RequestID string               `json:"request_id"`
}

type tavilySearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type webSearchContent struct {
	Query     string            `json:"query"`
	Answer    string            `json:"answer,omitempty"`
	Results   []webSearchResult `json:"results"`
	RequestID string            `json:"request_id,omitempty"`
}

type webSearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type tavilyHTTPClient struct {
	endpoint   string
	httpClient *http.Client
}

func newTavilyHTTPClient(endpoint string, httpClient *http.Client) tavilyHTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return tavilyHTTPClient{
		endpoint:   strings.TrimSpace(endpoint),
		httpClient: httpClient,
	}
}

func (c tavilyHTTPClient) Search(ctx context.Context, apiKey string, request tavilySearchRequest) (tavilySearchResponse, error) {
	endpoint := c.endpoint
	if endpoint == "" {
		endpoint = DefaultTavilySearchEndpoint
	}

	body, err := json.Marshal(request)
	if err != nil {
		return tavilySearchResponse{}, fmt.Errorf("encode request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return tavilySearchResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return tavilySearchResponse{}, err
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return tavilySearchResponse{}, fmt.Errorf("read response: %w", err)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return tavilySearchResponse{}, fmt.Errorf("status %d: %s", httpResponse.StatusCode, tavilyErrorMessage(responseBody))
	}

	var response tavilySearchResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return tavilySearchResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return response, nil
}

func tavilyErrorMessage(body []byte) string {
	var payload struct {
		Detail struct {
			Error string `json:"error"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Detail.Error != "" {
		return payload.Detail.Error
	}
	return strings.TrimSpace(string(body))
}
