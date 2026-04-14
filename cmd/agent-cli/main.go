package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zipkero/agent-runtime/internal/agent"
	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/executor"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/memory"
	"github.com/zipkero/agent-runtime/internal/observability"
	"github.com/zipkero/agent-runtime/internal/planner"
	"github.com/zipkero/agent-runtime/internal/state"
	"github.com/zipkero/agent-runtime/internal/tools"
	"github.com/zipkero/agent-runtime/internal/tools/calculator"
	"github.com/zipkero/agent-runtime/internal/tools/search_mock"
	"github.com/zipkero/agent-runtime/internal/tools/weather_mock"
	"github.com/zipkero/agent-runtime/internal/types"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Logger (전체 프로세스에서 단일 인스턴스)
	logger := observability.New()

	// Registry + tools 등록
	registry := tools.NewInMemoryToolRegistry()
	registry.Register(calculator.New())
	registry.Register(search_mock.New())
	registry.Register(weather_mock.New())

	// ToolRouter + ToolExecutor
	router := tools.NewToolRouter(registry, logger)
	exec := executor.NewToolExecutor(router)

	// LLMPlanner
	client := llm.NewOpenAIClient(cfg.OpenAIAPIKey, logger)
	p := planner.NewLLMPlanner(client, registry, logger)

	// MemoryManager
	sessionRepo := state.NewInMemorySessionRepository()
	memoryRepo := memory.NewInMemoryMemoryRepository()
	mm := memory.NewDefaultMemoryManager(sessionRepo, memoryRepo)

	// Runtime
	rt := agent.NewRuntime(p, exec, mm, 10, logger)

	fmt.Print("입력: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		fmt.Fprintln(os.Stderr, "입력 읽기 실패")
		os.Exit(1)
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		fmt.Fprintln(os.Stderr, "입력이 비어있습니다")
		os.Exit(1)
	}

	sessionID := agent.FixedSessionID

	// Session Load: 이전 세션 상태 복원
	sess, err := mm.LoadSession(context.Background(), sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "세션 로드 실패: %v\n", err)
		os.Exit(1)
	}
	if sess.SessionID == "" {
		sess.SessionID = sessionID
	}

	s := state.AgentState{
		Request: state.RequestState{
			RequestID: agent.NewRequestID(),
			UserInput: input,
		},
		Session: &sess,
		Status:  state.StatusRunning,
	}

	// context에 추적 ID 주입 (structured logger에서 사용)
	ctx := context.Background()
	ctx = observability.WithRequestID(ctx, s.Request.RequestID)
	ctx = observability.WithSessionID(ctx, s.Session.SessionID)

	result, err := rt.Run(ctx, s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "실행 실패: %v\n", err)
		os.Exit(1)
	}

	// 정상 완료 + FinalAnswer 가 있을 때만 Memory 저장 + Session 갱신
	if result.FinalAnswer != "" {
		content := buildMemoryContent(result)
		tags := extractTags(input)

		mem := types.Memory{
			ID:        agent.NewRequestID(),
			Content:   content,
			Tags:      tags,
			CreatedAt: time.Now(),
		}
		if saveErr := mm.SaveMemory(ctx, mem); saveErr != nil {
			fmt.Fprintf(os.Stderr, "메모리 저장 실패: %v\n", saveErr)
		}

		// Session Save: RecentContext에 이번 대화 요약 추가 후 저장
		if result.Session != nil {
			updatedSess := *result.Session
			exchange := fmt.Sprintf("Q: %s → A: %s", input, result.FinalAnswer)
			updatedSess.RecentContext = append(updatedSess.RecentContext, exchange)
			updatedSess.LastUpdated = time.Now()
			if saveErr := mm.SaveSession(ctx, sessionID, updatedSess); saveErr != nil {
				fmt.Fprintf(os.Stderr, "세션 저장 실패: %v\n", saveErr)
			}
		}
	}

	fmt.Printf("최종 답변: %s\n", result.FinalAnswer)
}

// buildMemoryContent 는 FinalAnswer 와 ToolResults 요약을 결합하여 Memory Content 를 생성한다.
func buildMemoryContent(s state.AgentState) string {
	var sb strings.Builder
	sb.WriteString(s.FinalAnswer)

	if len(s.Request.ToolResults) > 0 {
		sb.WriteString("\n\n[tool results]\n")
		for _, tr := range s.Request.ToolResults {
			if tr.IsError {
				fmt.Fprintf(&sb, "- %s: error: %s\n", tr.ToolName, tr.ErrMsg)
			} else {
				summary := tr.Output
				if len(summary) > 200 {
					summary = summary[:200] + "..."
				}
				fmt.Fprintf(&sb, "- %s: %s\n", tr.ToolName, summary)
			}
		}
	}
	return sb.String()
}

// extractTags 는 input 을 공백으로 분리한 뒤 길이 2 이하의 토큰을 제외하여 태그 목록을 반환한다.
func extractTags(input string) []string {
	words := strings.Fields(input)
	tags := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.ToLower(w)
		if len(w) > 2 {
			tags = append(tags, w)
		}
	}
	return tags
}
