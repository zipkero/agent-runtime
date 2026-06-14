package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

const tavilySearchEndpoint = "https://api.tavily.com/search"

// TavilyClient 는 WebSearch가 의존하는 Tavily Search 호출 경계다.
type TavilyClient interface {
	Search(ctx context.Context, req TavilySearchRequest) (TavilySearchResponse, error)
}

// TavilySearchRequest 는 Phase 5.1에서 노출하는 Tavily 검색 옵션만 담는다.
type TavilySearchRequest struct {
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth,omitempty"`
	MaxResults  int    `json:"max_results,omitempty"`
	Topic       string `json:"topic,omitempty"`
}

// TavilySearchResponse 는 tool result 구성에 필요한 Tavily 응답 필드만 보존한다.
type TavilySearchResponse struct {
	Query   string               `json:"query"`
	Answer  string               `json:"answer,omitempty"`
	Results []TavilySearchResult `json:"results"`
}

// TavilySearchResult 는 LLM이 다음 턴에서 출처와 요약을 읽을 수 있게 필요한 필드만 담는다.
type TavilySearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// HTTPTavilyClient 는 Tavily Search HTTP contract를 구현하는 기본 client다.
type HTTPTavilyClient struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

// NewHTTPTavilyClient 는 production Tavily endpoint를 사용하는 client를 만든다.
func NewHTTPTavilyClient(apiKey string, client *http.Client) *HTTPTavilyClient {
	return newHTTPTavilyClient(apiKey, tavilySearchEndpoint, client)
}

func newHTTPTavilyClient(apiKey, endpoint string, client *http.Client) *HTTPTavilyClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTavilyClient{
		apiKey:   apiKey,
		endpoint: endpoint,
		client:   client,
	}
}

// Search 는 context 취소와 Tavily non-2xx 응답을 호출자가 error로 구분할 수 있게 반환한다.
func (c *HTTPTavilyClient) Search(ctx context.Context, req TavilySearchRequest) (TavilySearchResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return TavilySearchResponse{}, fmt.Errorf("Tavily 요청 생성 실패: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return TavilySearchResponse{}, fmt.Errorf("Tavily 요청 생성 실패: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return TavilySearchResponse{}, fmt.Errorf("Tavily 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return TavilySearchResponse{}, fmt.Errorf("Tavily 응답 읽기 실패: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return TavilySearchResponse{}, fmt.Errorf("Tavily API 실패: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out TavilySearchResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return TavilySearchResponse{}, fmt.Errorf("Tavily 응답 파싱 실패: %w", err)
	}
	return out, nil
}

type webSearchInput struct {
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth,omitempty"`
	MaxResults  *int   `json:"max_results,omitempty"`
	Topic       string `json:"topic,omitempty"`
}

var webSearchInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["query"],
  "properties": {
    "query": {"type": "string", "description": "Tavily로 검색할 질문 또는 검색어"},
    "search_depth": {"type": "string", "enum": ["basic", "advanced", "fast", "ultra-fast"]},
    "max_results": {"type": "integer", "minimum": 1},
    "topic": {"type": "string", "enum": ["general", "news", "finance"]}
  }
}`)

// WebSearch 는 Tavily 검색을 Tool 계약에 맞게 감싼다.
// 설정 누락, 입력 오류, API 실패는 error로 반환해 Dispatcher의 IsError 정규화 경로를 사용한다.
type WebSearch struct {
	apiKey string
	client TavilyClient
}

// NewWebSearch 는 apiKey를 정규화하고, client가 없으면 기본 HTTP Tavily client를 연결한다.
func NewWebSearch(apiKey string, client TavilyClient) *WebSearch {
	apiKey = strings.TrimSpace(apiKey)
	if client == nil {
		client = NewHTTPTavilyClient(apiKey, nil)
	}
	return &WebSearch{
		apiKey: apiKey,
		client: client,
	}
}

// Spec 은 LLM에 노출할 web_search 입력 contract를 반환한다.
func (w *WebSearch) Spec() message.ToolSpec {
	return message.ToolSpec{
		Name:        "web_search",
		Description: "Tavily Search API로 웹 검색을 수행하고 관련 검색 결과를 반환한다.",
		InputSchema: webSearchInputSchema,
	}
}

// Execute 는 검색 성공만 ToolResult로 반환하고, 실패는 Dispatcher가 error result로 바꾸게 둔다.
func (w *WebSearch) Execute(ctx context.Context, input json.RawMessage) (message.ToolResult, error) {
	if w.apiKey == "" {
		return message.ToolResult{}, errors.New("Tavily 설정 누락: TAVILY_API_KEY가 필요합니다")
	}

	var in webSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return message.ToolResult{}, fmt.Errorf("입력 파싱 실패: %w", err)
	}

	req, err := validateWebSearchInput(in)
	if err != nil {
		return message.ToolResult{}, err
	}

	resp, err := w.client.Search(ctx, req)
	if err != nil {
		return message.ToolResult{}, err
	}

	return message.ToolResult{Content: formatTavilySearchResponse(req.Query, resp)}, nil
}

func validateWebSearchInput(in webSearchInput) (TavilySearchRequest, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return TavilySearchRequest{}, errors.New("입력 검증 실패: query 필드가 필요합니다")
	}
	maxResults := 0
	if in.MaxResults != nil {
		if *in.MaxResults <= 0 {
			return TavilySearchRequest{}, errors.New("입력 검증 실패: max_results는 1 이상이어야 합니다")
		}
		maxResults = *in.MaxResults
	}
	if in.SearchDepth != "" && !isAllowedSearchDepth(in.SearchDepth) {
		return TavilySearchRequest{}, fmt.Errorf("입력 검증 실패: 미지원 search_depth %q", in.SearchDepth)
	}
	if in.Topic != "" && !isAllowedTavilyTopic(in.Topic) {
		return TavilySearchRequest{}, fmt.Errorf("입력 검증 실패: 미지원 topic %q", in.Topic)
	}

	return TavilySearchRequest{
		Query:       query,
		SearchDepth: in.SearchDepth,
		MaxResults:  maxResults,
		Topic:       in.Topic,
	}, nil
}

// isAllowedSearchDepth 는 schema와 Tavily Search API가 허용하는 검색 깊이를 맞추는 guard다.
func isAllowedSearchDepth(value string) bool {
	switch value {
	case "basic", "advanced", "fast", "ultra-fast":
		return true
	default:
		return false
	}
}

// isAllowedTavilyTopic 은 Tavily topic enum 밖의 provider 옵션을 실행 전에 거부한다.
func isAllowedTavilyTopic(value string) bool {
	switch value {
	case "general", "news", "finance":
		return true
	default:
		return false
	}
}

func formatTavilySearchResponse(query string, resp TavilySearchResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tavily search results for %q", query)
	if resp.Answer != "" {
		fmt.Fprintf(&b, "\nAnswer: %s", resp.Answer)
	}
	if len(resp.Results) == 0 {
		b.WriteString("\nNo results.")
		return b.String()
	}

	for i, result := range resp.Results {
		fmt.Fprintf(&b, "\n\n%d. %s", i+1, result.Title)
		if result.URL != "" {
			fmt.Fprintf(&b, "\nURL: %s", result.URL)
		}
		if result.Content != "" {
			fmt.Fprintf(&b, "\nContent: %s", result.Content)
		}
		if result.Score != 0 {
			fmt.Fprintf(&b, "\nScore: %.4f", result.Score)
		}
	}
	return b.String()
}
