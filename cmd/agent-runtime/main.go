// Command agent-runtime은 설정된 LLM 공급자와 Tool로 단일 Agent run을 실행하는 CLI 진입점이다.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/zipkero/agent-runtime/internal/agent"
	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/tool"
)

const (
	cliMaxSteps           = 10
	cliMaxToolCalls       = 20
	cliMaxToolResultBytes = 64 * 1024
	cliRunTimeout         = 10 * time.Minute
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, config.Load, newConfiguredClient, newConfiguredTools))
}

type configLoader func() (config.Config, error)
type clientBuilder func(config.Config) (llm.LLMClient, error)
type toolBuilder func(config.Config, string) (*tool.Registry, error)

func run(
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	loadConfig configLoader,
	buildClient clientBuilder,
	buildTools toolBuilder,
) int {
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

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "tool configuration error: get current working directory: %v\n", err)
		return 1
	}
	tools, err := buildTools(cfg, root)
	if err != nil {
		fmt.Fprintf(stderr, "tool configuration error: %v\n", err)
		return 1
	}

	runner, err := agent.NewRunner(agent.RunnerOptions{
		Client:             client,
		Model:              cfg.LLMModel,
		MaxSteps:           cliMaxSteps,
		ModelTimeout:       cfg.LLMTimeout,
		Tools:              tools,
		MaxToolCalls:       cliMaxToolCalls,
		MaxToolResultBytes: cliMaxToolResultBytes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "runner error: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), cliRunTimeout)
	defer cancel()

	result := runner.Run(ctx, prompt)
	if result.State.Status == agent.StatusFinal {
		fmt.Fprintln(stdout, result.State.FinalAnswer)
		return 0
	}
	if result.State.Status == agent.StatusError {
		if result.State.LastError != nil {
			fmt.Fprintf(stderr, "agent error: %v\n", result.State.LastError)
		} else {
			fmt.Fprintln(stderr, "agent error: execution failed without an error")
		}
		return 1
	}

	fmt.Fprintf(stderr, "agent stopped: %s\n", result.State.Status)
	return 1
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
