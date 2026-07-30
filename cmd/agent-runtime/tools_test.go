package main

import (
	"reflect"
	"testing"

	"github.com/zipkero/agent-runtime/internal/config"
)

// TestNewConfiguredToolsRegistersCodeExecutionOnlyWhenEnabled 은 기본 Tool 묶음과 등록 순서를 고정하고,
// code execution은 설정이 켜졌을 때만 추가되는지 확인한다.
func TestNewConfiguredToolsRegistersCodeExecutionOnlyWhenEnabled(t *testing.T) {
	tests := []struct {
		name                string
		enableCodeExecution bool
		wantNames           []string
	}{
		{
			name:      "disabled by default",
			wantNames: []string{"calculator", "read_file", "web_search", "save_file"},
		},
		{
			name:                "enabled",
			enableCodeExecution: true,
			wantNames:           []string{"calculator", "read_file", "web_search", "save_file", "code_execution"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := newConfiguredTools(config.Config{
				EnableCodeExecution: tt.enableCodeExecution,
			}, t.TempDir())
			if err != nil {
				t.Fatalf("newConfiguredTools() error = %v", err)
			}

			schemas := registry.Schemas()
			gotNames := make([]string, 0, len(schemas))
			for _, schema := range schemas {
				gotNames = append(gotNames, schema.Name)
			}
			if !reflect.DeepEqual(gotNames, tt.wantNames) {
				t.Fatalf("tool names = %v, want %v", gotNames, tt.wantNames)
			}
		})
	}
}
