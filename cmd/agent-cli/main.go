package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zipkero/gogent/internal/agent"
	"github.com/zipkero/gogent/internal/executor"
	"github.com/zipkero/gogent/internal/llm"
	"github.com/zipkero/gogent/internal/planner"
	"github.com/zipkero/gogent/internal/state"
	"github.com/zipkero/gogent/internal/tools"
	"github.com/zipkero/gogent/internal/tools/calculator"
	"github.com/zipkero/gogent/internal/tools/search_mock"
	"github.com/zipkero/gogent/internal/tools/weather_mock"
)

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY 환경변수가 설정되지 않았습니다")
		os.Exit(1)
	}

	// Registry + tools 등록
	registry := tools.NewInMemoryToolRegistry()
	registry.Register(calculator.New())
	registry.Register(search_mock.New())
	registry.Register(weather_mock.New())

	// ToolRouter + ToolExecutor
	router := tools.NewToolRouter(registry)
	exec := executor.NewToolExecutor(router)

	// LLMPlanner
	client := llm.NewOpenAIClient(apiKey)
	p := planner.NewLLMPlanner(client, registry)

	// Runtime
	rt := &agent.Runtime{
		Planner:  p,
		Executor: exec,
		MaxStep:  10,
	}

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

	s := state.AgentState{
		Request: state.RequestState{
			RequestID: agent.NewRequestID(),
			UserInput: input,
		},
		Session: &state.SessionState{
			SessionID: agent.FixedSessionID,
		},
		Status: state.StatusRunning,
	}

	result, err := rt.Run(context.Background(), s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "실행 실패: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("최종 답변: %s\n", result.FinalAnswer)
}
