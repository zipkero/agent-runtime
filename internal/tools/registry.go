package tools

// ToolRegistry 는 Tool 을 등록하고 이름으로 조회하는 인터페이스다.
// Router 는 이 인터페이스에만 의존하므로 구현체(InMemoryToolRegistry 등)를 알 필요 없다.
type ToolRegistry interface {
	// Register 는 tool 을 registry 에 등록한다.
	// 동일한 이름의 tool 이 이미 등록된 경우 덮어쓴다.
	Register(tool Tool)

	// Get 은 name 에 해당하는 tool 을 반환한다.
	// 등록되지 않은 name 이면 error 를 반환한다.
	Get(name string) (Tool, error)

	// List 는 등록된 모든 tool 목록을 반환한다.
	List() []Tool
}
