package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/zipkero/agent-runtime/internal/message"
)

type testTool struct {
	name        string
	description string
	schema      message.ToolSchema
}

func (t testTool) Name() string {
	return t.name
}

func (t testTool) Description() string {
	return t.description
}

func (t testTool) Schema() message.ToolSchema {
	return t.schema
}

func (t testTool) Validate(json.RawMessage) error {
	return nil
}

func (t testTool) Execute(context.Context, json.RawMessage) (Result, error) {
	return Result{Content: "ok"}, nil
}

// TestRegistryRegistersLooksUpAndExposesSchemas 는 registry가 등록 순서대로 Tool과 schema를 보존하는지 확인한다.
func TestRegistryRegistersLooksUpAndExposesSchemas(t *testing.T) {
	registry := NewRegistry()
	searchSchema := message.ToolSchema{
		Name:        "search",
		Description: "Search documents",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
	}
	readSchema := message.ToolSchema{
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	}

	if err := registry.Register(testTool{name: "search", description: "Search documents", schema: searchSchema}); err != nil {
		t.Fatalf("Register(search) error = %v", err)
	}
	if err := registry.Register(testTool{name: "read_file", description: "Read a file", schema: readSchema}); err != nil {
		t.Fatalf("Register(read_file) error = %v", err)
	}

	got, ok := registry.Lookup("search")
	if !ok {
		t.Fatal("Lookup(search) ok = false, want true")
	}
	if got.Name() != "search" || got.Description() != "Search documents" {
		t.Fatalf("Lookup(search) = %+v, want search tool", got)
	}

	schemas := registry.Schemas()
	if len(schemas) != 2 {
		t.Fatalf("len(Schemas()) = %d, want 2", len(schemas))
	}
	if schemas[0].Name != "search" || schemas[1].Name != "read_file" {
		t.Fatalf("Schemas() order = %+v, want search then read_file", schemas)
	}
	if string(schemas[0].InputSchema) != string(searchSchema.InputSchema) {
		t.Fatalf("first InputSchema = %s, want %s", schemas[0].InputSchema, searchSchema.InputSchema)
	}
	if string(schemas[1].InputSchema) != string(readSchema.InputSchema) {
		t.Fatalf("second InputSchema = %s, want %s", schemas[1].InputSchema, readSchema.InputSchema)
	}
}

// TestRegistryRejectsNilAndEmptyName 는 실행할 수 없는 Tool 등록을 명확한 오류로 거절하는지 확인한다.
func TestRegistryRejectsNilAndEmptyName(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
		want error
	}{
		{name: "nil interface", tool: nil, want: ErrNilTool},
		{name: "typed nil", tool: (*testPointerTool)(nil), want: ErrNilTool},
		{name: "empty", tool: testTool{name: " "}, want: ErrEmptyName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRegistry().Register(tt.tool)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Register() error = %v, want %v", err, tt.want)
			}
		})
	}
}

type testPointerTool struct {
	testTool
}

// TestRegistryRejectsDuplicateName 는 같은 이름의 Tool을 두 번 등록할 수 없게 막는지 확인한다.
func TestRegistryRejectsDuplicateName(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testTool{name: "search"}); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}

	err := registry.Register(testTool{name: " search "})
	if !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("Register(duplicate) error = %v, want %v", err, ErrDuplicateName)
	}
}

// TestRegistryLookupUnknownDistinguishesMissingTool 는 등록되지 않은 이름을 정상 lookup과 구분하는지 확인한다.
func TestRegistryLookupUnknownDistinguishesMissingTool(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testTool{name: "search"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if got, ok := registry.Lookup("read_file"); ok || got != nil {
		t.Fatalf("Lookup(unknown) = (%v, %v), want nil false", got, ok)
	}
}

func TestRegistryLen(t *testing.T) {
	var nilRegistry *Registry
	if got := nilRegistry.Len(); got != 0 {
		t.Fatalf("nil Registry Len() = %d, want 0", got)
	}

	registry := NewRegistry()
	if got := registry.Len(); got != 0 {
		t.Fatalf("empty Registry Len() = %d, want 0", got)
	}
	if err := registry.Register(testTool{name: "search"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got := registry.Len(); got != 1 {
		t.Fatalf("Registry Len() = %d, want 1", got)
	}
}

// TestRegistrySchemasReturnsCopy 는 호출자가 반환된 schema를 바꿔도 registry 내부 schema가 오염되지 않는지 확인한다.
func TestRegistrySchemasReturnsCopy(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testTool{
		name: "search",
		schema: message.ToolSchema{
			Name:        "search",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	schemas := registry.Schemas()
	schemas[0].InputSchema[0] = '['

	got := registry.Schemas()
	if string(got[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("Schemas() InputSchema = %s, want original copy", got[0].InputSchema)
	}
}
