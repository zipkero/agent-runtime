package search_mock

import (
	"context"
	"fmt"
	"strings"

	"github.com/zipkero/gogent/internal/tools"
	"github.com/zipkero/gogent/internal/types"
)

// searchResult 는 단일 검색 결과 항목이다.
type searchResult struct {
	Title   string
	Snippet string
	URL     string
}

// mockDB 는 쿼리 키워드별 고정 검색 결과 데이터다.
var mockDB = map[string][]searchResult{
	"golang": {
		{Title: "The Go Programming Language", Snippet: "Go is an open source programming language that makes it easy to build simple, reliable, and efficient software.", URL: "https://go.dev"},
		{Title: "Go Documentation", Snippet: "Official documentation for the Go programming language, including tutorials and reference material.", URL: "https://go.dev/doc"},
	},
	"agent": {
		{Title: "AI Agent Overview", Snippet: "An AI agent is a system that perceives its environment and takes actions to achieve goals.", URL: "https://example.com/ai-agent"},
		{Title: "LLM-based Agents", Snippet: "Large language model agents combine reasoning and tool use to complete complex tasks autonomously.", URL: "https://example.com/llm-agents"},
	},
	"weather": {
		{Title: "Weather Forecasting", Snippet: "Weather forecasting uses atmospheric data to predict future weather conditions.", URL: "https://example.com/weather"},
		{Title: "Climate vs Weather", Snippet: "Weather refers to short-term atmospheric conditions, while climate describes long-term patterns.", URL: "https://example.com/climate"},
	},
	"calculator": {
		{Title: "Online Calculator", Snippet: "A simple calculator for basic arithmetic operations.", URL: "https://example.com/calc"},
	},
}

// SearchMock 은 쿼리 문자열을 받아 고정된 검색 결과를 반환하는 mock Tool 구현체다.
type SearchMock struct{}

func New() *SearchMock {
	return &SearchMock{}
}

func (s *SearchMock) Name() string {
	return "search_mock"
}

func (s *SearchMock) Description() string {
	return "쿼리 문자열을 받아 검색 결과를 반환한다. 테스트용 mock 데이터를 사용한다."
}

func (s *SearchMock) InputSchema() tools.Schema {
	return tools.Schema{
		Fields: []tools.FieldSchema{
			{
				Name:        "query",
				Type:        tools.FieldTypeString,
				Description: "검색할 쿼리 문자열 (예: 'golang', 'AI agent')",
				Required:    true,
			},
		},
	}
}

func (s *SearchMock) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	raw, ok := input["query"]
	if !ok {
		return types.ToolResult{ToolName: s.Name(), IsError: true, ErrMsg: "query 필드가 없습니다"}, nil
	}
	query, ok := raw.(string)
	if !ok {
		return types.ToolResult{ToolName: s.Name(), IsError: true, ErrMsg: "query 는 string 이어야 합니다"}, nil
	}

	normalized := strings.ToLower(strings.TrimSpace(query))

	// 키워드 매칭: 쿼리에 키가 포함되어 있으면 해당 결과 반환
	var matched []searchResult
	for key, results := range mockDB {
		if strings.Contains(normalized, key) {
			matched = append(matched, results...)
		}
	}

	if len(matched) == 0 {
		return types.ToolResult{
			ToolName: s.Name(),
			IsError:  true,
			ErrMsg:   fmt.Sprintf("'%s' 에 대한 검색 결과가 없습니다", query),
		}, nil
	}

	var sb strings.Builder
	for i, r := range matched {
		fmt.Fprintf(&sb, "[%d] %s\n    %s\n    %s\n", i+1, r.Title, r.Snippet, r.URL)
	}

	return types.ToolResult{
		ToolName: s.Name(),
		Output:   strings.TrimRight(sb.String(), "\n"),
	}, nil
}
