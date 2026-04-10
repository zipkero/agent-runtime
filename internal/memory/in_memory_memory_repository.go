package memory

import (
	"context"
	"sync"

	"github.com/zipkero/agent-runtime/internal/types"
)

// InMemoryMemoryRepository 는 슬라이스 기반의 MemoryRepository 구현체다.
// 프로세스 내 메모리에만 저장되므로 재시작 시 데이터가 소실된다.
// Postgres 구현(PostgresMemoryRepository)으로 교체하기 전 MemoryManager 단위 테스트용으로 사용한다.
type InMemoryMemoryRepository struct {
	mu      sync.RWMutex
	records []types.Memory
}

// NewInMemoryMemoryRepository 는 초기화된 InMemoryMemoryRepository 를 반환한다.
func NewInMemoryMemoryRepository() *InMemoryMemoryRepository {
	return &InMemoryMemoryRepository{
		records: make([]types.Memory, 0),
	}
}

// Save 는 Memory 레코드를 내부 슬라이스에 append 한다.
func (r *InMemoryMemoryRepository) Save(_ context.Context, memory types.Memory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.records = append(r.records, memory)
	return nil
}

// LoadByTags 는 tags 중 하나라도 일치하는 Memory 를 최대 limit 개 반환한다 (OR 조건).
// tags 가 비어 있거나 limit 이 0 이하이면 빈 슬라이스를 반환한다.
func (r *InMemoryMemoryRepository) LoadByTags(_ context.Context, tags []string, limit int) ([]types.Memory, error) {
	if len(tags) == 0 || limit <= 0 {
		return []types.Memory{}, nil
	}

	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagSet[t] = struct{}{}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]types.Memory, 0, limit)
	for _, m := range r.records {
		if matchesAnyTag(m.Tags, tagSet) {
			result = append(result, m)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func matchesAnyTag(memoryTags []string, queryTags map[string]struct{}) bool {
	for _, t := range memoryTags {
		if _, ok := queryTags[t]; ok {
			return true
		}
	}
	return false
}
