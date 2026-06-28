package main

import (
	"log"
	"os"

	"github.com/zipkero/agent-runtime/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.New(os.Stderr, "agent-runtime ", log.LstdFlags).Printf("config error: %v", err)
		os.Exit(1)
	}

	logger := log.New(os.Stdout, "agent-runtime ", log.LstdFlags)
	logger.Printf("starting provider=%s model=%s host=%s timeout=%s log_level=%s",
		cfg.LLMProvider,
		displayValue(cfg.LLMModel),
		cfg.LLMHost,
		cfg.LLMTimeout,
		cfg.LogLevel,
	)
	logger.Print("phase=0 external_provider_calls=false")
}

func displayValue(value string) string {
	if value == "" {
		return "(unset)"
	}
	return value
}
