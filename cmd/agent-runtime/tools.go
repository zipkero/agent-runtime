package main

import (
	"fmt"

	"github.com/zipkero/agent-runtime/internal/config"
	runtimetool "github.com/zipkero/agent-runtime/internal/tool"
)

func newConfiguredTools(cfg config.Config, root string) (*runtimetool.Registry, error) {
	fileRead, err := runtimetool.NewFileRead(root)
	if err != nil {
		return nil, err
	}
	fileSave, err := runtimetool.NewFileSave(root)
	if err != nil {
		return nil, err
	}

	configured := []runtimetool.Tool{
		runtimetool.NewCalculator(),
		fileRead,
		runtimetool.NewWebSearch(cfg.TavilyAPIKey),
		fileSave,
	}
	if cfg.EnableCodeExecution {
		codeExecution, err := runtimetool.NewCodeExecution(root)
		if err != nil {
			return nil, err
		}
		configured = append(configured, codeExecution)
	}

	registry := runtimetool.NewRegistry()
	for _, configuredTool := range configured {
		if err := registry.Register(configuredTool); err != nil {
			return nil, fmt.Errorf("register tool %q: %w", configuredTool.Name(), err)
		}
	}
	return registry, nil
}
