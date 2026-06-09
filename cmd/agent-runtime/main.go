// CLI 진입점: 사용자 프롬프트를 읽어 Agent loop를 통해 처리하고 결과를 stdout으로 출력한다.
// 실패(timeout·인증 오류·max step 초과)는 stderr로 출력하고 비정상 종료코드로 종료한다.
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
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// defaultMaxSteps 는 config에 노출하지 않는 Agent loop의 기본 step 상한이다.
const defaultMaxSteps = 10

// defaultToolTimeout 은 per-tool 실행의 기본 deadline이다(task-008에서 실제 tool 등록 전까지
// 빈 registry와 함께 쓰인다).
const defaultToolTimeout = 30 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	client, err := llm.NewClaudeClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		os.Exit(1)
	}

	prompt, err := readPrompt()
	if err != nil {
		fmt.Fprintf(os.Stderr, "input error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// task-008에서 실제 tool을 등록한다. 현재는 빈 registry와 기본 timeout으로 컴파일만 통과시킨다.
	registry := tool.NewRegistry()
	code := run(ctx, client, cfg.Model, prompt, defaultMaxSteps, registry, defaultToolTimeout)
	os.Exit(code)
}

// readPrompt 는 stdin을 EOF까지 읽어 전체 입력을 하나의 프롬프트로 합쳐 반환한다.
func readPrompt() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	// bufio.Scanner는 기본 한 줄 상한(64KB)이 있어, 그보다 긴 단일 라인 입력은 에러로 반환된다.
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

// run 은 Agent loop를 통해 프롬프트를 처리하고 종료 상태별로 출력을 분기한 뒤 종료코드를 반환한다.
// final이면 최종 답을 stdout에 출력하고 0을 반환한다.
// error·ctx 취소이면 원인을 stderr에 쓰고 1을 반환한다.
// max step 초과이면 "max step 초과" 문구를 stderr에 쓰고 1을 반환한다.
func run(ctx context.Context, client llm.LLMClient, model, prompt string, maxSteps int, registry *tool.Registry, toolTimeout time.Duration) int {
	a := agent.NewAgent(client, model, maxSteps, nil, registry, toolTimeout)
	state := a.Run(ctx, prompt)

	switch state.Status {
	case agent.StatusFinal:
		finalMsg, ok := state.FinalMessage()
		if ok {
			printMessage(finalMsg)
		}
		return 0
	case agent.StatusMaxSteps:
		// max step 초과는 실패로 표현 — error와 문구를 구분해 원인을 드러낸다.
		fmt.Fprintf(os.Stderr, "max step 초과로 최종 답에 도달하지 못함 (step=%d)\n", state.Steps)
		return 1
	default:
		// StatusError: 원인 에러를 stderr에 쓰고 비정상 종료코드(ctx 취소도 동일 처리).
		fmt.Fprintf(os.Stderr, "chat error: %v\n", state.Err)
		return 1
	}
}

// printMessage 는 Message의 Content를 텍스트 블록만 골라 stdout에 출력한다.
// tool_call 블록은 final 상태에서 나타나지 않으므로 텍스트만 처리한다.
func printMessage(msg message.Message) {
	for _, block := range msg.Content {
		if block.Type == message.BlockTypeText {
			fmt.Print(block.Text)
		}
	}
	fmt.Println()
}
