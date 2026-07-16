// Package tool은 Agent가 호출할 수 있는 Tool 계약, 레지스트리, 내장 구현을 제공한다.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

var (
	// ErrNilTool 변수는 nil 인터페이스 또는 구체 값이 nil인 Tool 등록을 거부할 때 반환한다.
	ErrNilTool = errors.New("tool is nil")
	// ErrEmptyName 변수는 공백을 제거한 Tool 이름이 비어 있을 때 반환한다.
	ErrEmptyName = errors.New("tool name is empty")
	// ErrDuplicateName 변수는 같은 이름의 Tool을 두 번 등록할 때 반환한다.
	ErrDuplicateName = errors.New("tool name is already registered")
)

// DefaultMaxResultBytes 상수는 Runtime과 내장 Tool이 사용하는 기본 Tool 결과 크기 상한이다.
const DefaultMaxResultBytes = 64 * 1024

// Tool 인터페이스는 Runtime이 이름으로 찾아 검증하고 실행할 수 있는 공급자 중립 계약이다.
type Tool interface {
	Name() string
	Description() string
	Schema() message.ToolSchema
	// Validate 메서드는 외부 효과 없이 인수를 검증해야 한다.
	Validate(args json.RawMessage) error
	// Execute 메서드는 ctx 취소를 관찰해 반환해야 하며, Runtime은 반환될 때까지 다음 상태로 전이하지 않는다.
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}

// Result 구조체는 Tool 실행 결과를 LLM에 다시 전달할 내용 문자열로 정규화한다.
type Result struct {
	Content string
}

// Registry 구조체는 등록된 Tool을 이름으로 조회하고 LLM 요청용 스키마 목록을 제공한다.
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry 함수는 등록 순서를 보존하는 빈 Tool 레지스트리를 만든다.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 메서드는 공백을 제거한 이름으로 Tool을 등록하고 중복 이름을 거부한다.
func (r *Registry) Register(tool Tool) error {
	if isNilTool(tool) {
		return ErrNilTool
	}

	name := strings.TrimSpace(tool.Name())
	if name == "" {
		return ErrEmptyName
	}

	r.ensureReady()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateName, name)
	}

	r.tools[name] = tool
	r.order = append(r.order, name)
	return nil
}

func isNilTool(tool Tool) bool {
	if tool == nil {
		return true
	}

	value := reflect.ValueOf(tool)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Lookup 메서드는 공백을 제거한 이름으로 등록된 Tool을 찾는다.
func (r *Registry) Lookup(name string) (Tool, bool) {
	if r == nil || len(r.tools) == 0 {
		return nil, false
	}

	tool, ok := r.tools[strings.TrimSpace(name)]
	return tool, ok
}

// Len 메서드는 현재 등록된 Tool 수를 반환한다.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.tools)
}

// Schemas 메서드는 등록 순서대로 복제한 Tool 스키마를 반환해 레지스트리 내부 값의 변경을 막는다.
func (r *Registry) Schemas() []message.ToolSchema {
	if r == nil || len(r.order) == 0 {
		return nil
	}

	schemas := make([]message.ToolSchema, 0, len(r.order))
	for _, name := range r.order {
		tool, ok := r.tools[name]
		if !ok {
			continue
		}
		schemas = append(schemas, cloneSchema(tool.Schema()))
	}
	return schemas
}

func (r *Registry) ensureReady() {
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
}

func cloneSchema(schema message.ToolSchema) message.ToolSchema {
	if len(schema.InputSchema) > 0 {
		schema.InputSchema = append(json.RawMessage(nil), schema.InputSchema...)
	}
	return schema
}
