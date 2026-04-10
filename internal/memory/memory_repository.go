package memory

import (
	"context"

	"github.com/zipkero/gogent/internal/types"
)

// MemoryRepository 는 Long-term Memory 의 저장과 조회를 추상화한다.
// InMemory, Postgres 등 다양한 백엔드로 교체할 수 있도록 인터페이스로 분리한다.
type MemoryRepository interface {
	// Save 는 Memory 레코드를 저장소에 저장한다.
	Save(ctx context.Context, memory types.Memory) error

	// LoadByTags 는 tags 중 하나라도 일치하는 Memory 를 최대 limit 개 반환한다 (OR 조건).
	// 결과가 없으면 빈 슬라이스와 nil error 를 반환한다.
	LoadByTags(ctx context.Context, tags []string, limit int) ([]types.Memory, error)
}
