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
	ErrNilTool       = errors.New("tool is nil")
	ErrEmptyName     = errors.New("tool name is empty")
	ErrDuplicateName = errors.New("tool name is already registered")
)

// DefaultMaxResultBytes 는 Runtime과 내장 Tool이 사용하는 기본 Tool result 크기 상한이다.
const DefaultMaxResultBytes = 64 * 1024

// Tool 은 Runtime이 이름으로 찾아 검증하고 실행할 수 있는 provider-neutral contract다.
type Tool interface {
	Name() string
	Description() string
	Schema() message.ToolSchema
	Validate(args json.RawMessage) error
	// Execute 는 ctx 취소를 관찰해 반환해야 하며, Runtime은 반환될 때까지 다음 상태로 전이하지 않는다.
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}

// Result 는 Tool 실행 결과를 LLM에 다시 전달할 문자열 content로 정규화한다.
type Result struct {
	Content string
}

// Registry 는 등록된 Tool을 이름으로 조회하고 LLM 요청용 schema 목록을 제공한다.
type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

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

func (r *Registry) Lookup(name string) (Tool, bool) {
	if r == nil || len(r.tools) == 0 {
		return nil, false
	}

	tool, ok := r.tools[strings.TrimSpace(name)]
	return tool, ok
}

// Len 은 현재 등록된 Tool 수를 반환한다.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.tools)
}

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
