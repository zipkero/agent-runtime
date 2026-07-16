// Command agent-runtime은 설정된 LLM 공급자에 단발 프롬프트를 보내는 CLI 진입점이다.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, config.Load, newConfiguredClient))
}

type configLoader func() (config.Config, error)
type clientBuilder func(config.Config) (llm.LLMClient, error)

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, loadConfig configLoader, buildClient clientBuilder) int {
	prompt, err := readPrompt(args, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "input error: %v\n", err)
		return 1
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	client, err := buildClient(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.LLMTimeout)
	defer cancel()

	resp, err := client.Chat(ctx, llm.ChatRequest{
		Model:    cfg.LLMModel,
		Messages: []message.Message{message.User(prompt)},
	})
	if err != nil {
		fmt.Fprintf(stderr, "llm error: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, resp.Message.Text)
	return 0
}

func readPrompt(args []string, stdin io.Reader) (string, error) {
	var prompt string
	if len(args) > 0 {
		prompt = strings.Join(args, " ")
	} else {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		prompt = string(data)
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt is required")
	}
	return prompt, nil
}

func newConfiguredClient(cfg config.Config) (llm.LLMClient, error) {
	registry := llm.NewRegistry()
	if err := registry.Register(llm.ProviderClaude, llm.ProviderRequirements{Model: true, APIKey: true}, llm.NewClaudeClient); err != nil {
		return nil, err
	}
	if err := registry.Register(llm.ProviderOllama, llm.ProviderRequirements{Model: true, Host: true}, llm.NewOllamaClient); err != nil {
		return nil, err
	}

	return registry.NewClient(llm.ProviderConfig{
		Provider: cfg.LLMProvider,
		Model:    cfg.LLMModel,
		Host:     cfg.LLMHost,
		APIKey:   cfg.LLMAPIKey,
	})
}
