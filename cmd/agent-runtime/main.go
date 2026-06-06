// CLI 진입점: 사용자 프롬프트를 읽어 LLM을 호출하고 결과를 stdout으로 출력한다.
// 실패(timeout·인증 오류)는 stderr로 출력하고 비정상 종료코드로 종료한다.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

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

	code := run(ctx, client, cfg.Model, prompt)
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

// run 은 프롬프트로 LLM을 호출해 응답을 출력하고 종료코드(0: 성공, 1: 실패)를 반환한다.
func run(ctx context.Context, client llm.LLMClient, model, prompt string) int {
	req := llm.ChatRequest{
		Model: model,
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock(prompt)},
			},
		},
	}

	resp, err := client.Chat(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chat error: %v\n", err)
		return 1
	}

	printResponse(resp)
	return 0
}

// printResponse 는 ChatResponse의 Content를 텍스트와 tool call로 구분해 stdout에 출력한다.
func printResponse(resp llm.ChatResponse) {
	for _, block := range resp.Message.Content {
		switch block.Type {
		case message.BlockTypeText:
			fmt.Print(block.Text)
		case message.BlockTypeToolCall:
			tc := block.ToolCall
			fmt.Printf("[tool_call] name=%s id=%s input=%s\n", tc.Name, tc.ID, string(tc.Input))
		}
	}
	fmt.Println()
}
