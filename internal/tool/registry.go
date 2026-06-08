package tool

import (
	"fmt"

	"github.com/zipkero/agent-runtime/internal/message"
)

// Registry 는 tool을 이름으로 등록·조회하고 등록된 schema를 수집하는 저장소다.
// 이름 키는 각 tool의 Spec().Name에서 얻는다.
// Registry는 동시 접근을 보호하지 않으므로 단일 고루틴에서 사용한다.
type Registry struct {
	tools []Tool            // 등록 순서 보존을 위한 슬라이스
	index map[string]Tool   // 이름 → tool 빠른 조회
}

// NewRegistry 는 빈 Registry를 생성한다.
func NewRegistry() *Registry {
	return &Registry{
		index: make(map[string]Tool),
	}
}

// Register 는 tool을 이름으로 등록한다.
// 같은 이름이 이미 등록되어 있으면 등록을 거부하고 error를 반환한다.
func (r *Registry) Register(t Tool) error {
	name := t.Spec().Name
	if _, exists := r.index[name]; exists {
		return fmt.Errorf("tool %q is already registered", name)
	}
	r.tools = append(r.tools, t)
	r.index[name] = t
	return nil
}

// Lookup 은 이름으로 tool을 조회한다.
// 등록된 tool이 있으면 (tool, true)를 반환하고, 없으면 (nil, false)를 반환한다.
func (r *Registry) Lookup(name string) (Tool, bool) {
	t, ok := r.index[name]
	return t, ok
}

// Specs 는 등록된 모든 tool의 schema를 등록 순서대로 반환한다.
// 반환된 슬라이스는 ChatRequest.Tools에 그대로 실을 수 있다.
func (r *Registry) Specs() []message.ToolSpec {
	specs := make([]message.ToolSpec, len(r.tools))
	for i, t := range r.tools {
		specs[i] = t.Spec()
	}
	return specs
}
