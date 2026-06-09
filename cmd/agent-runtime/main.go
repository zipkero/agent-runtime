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

// defaultToolTimeout 은 per-tool 실행의 기본 deadline이다.
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

	// file read의 base 경로는 CLI 실행 시점의 작업 디렉터리로 고정한다.
	// os.Getwd() 실패 시 비정상 종료한다.
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "작업 디렉터리 조회 실패: %v\n", err)
		os.Exit(1)
	}

	registry, err := buildRegistry(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tool 등록 실패: %v\n", err)
		os.Exit(1)
	}

	code := run(ctx, client, cfg.Model, prompt, defaultMaxSteps, registry, defaultToolTimeout)
	os.Exit(code)
}

// buildRegistry 는 calculator와 file read tool을 등록한 Registry를 생성한다.
// fileBase는 file read tool의 허용 범위 루트 디렉터리다.
// 등록 실패(충돌 또는 base 경로 오류) 시 error를 반환한다.
func buildRegistry(fileBase string) (*tool.Registry, error) {
	reg := tool.NewRegistry()

	if err := reg.Register(&tool.Calculator{}); err != nil {
		return nil, fmt.Errorf("calculator 등록 실패: %w", err)
	}

	fr, err := tool.NewFileRead(fileBase)
	if err != nil {
		return nil, fmt.Errorf("file_read 생성 실패: %w", err)
	}
	if err := reg.Register(fr); err != nil {
		return nil, fmt.Errorf("file_read 등록 실패: %w", err)
	}

	return reg, nil
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
