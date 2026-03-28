package tools

import "fmt"

// InMemoryToolRegistry 는 map 기반 ToolRegistry 구현체다.
type InMemoryToolRegistry struct {
	tools map[string]Tool
}

// NewInMemoryToolRegistry 는 빈 InMemoryToolRegistry 를 생성한다.
func NewInMemoryToolRegistry() *InMemoryToolRegistry {
	return &InMemoryToolRegistry{
		tools: make(map[string]Tool),
	}
}

func (r *InMemoryToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *InMemoryToolRegistry) Get(name string) (Tool, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return tool, nil
}

func (r *InMemoryToolRegistry) List() []Tool {
	list := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}
