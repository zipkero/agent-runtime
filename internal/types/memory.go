package types

import "time"

// Memory 는 Long-term Memory 저장소에 보관되는 단일 기억 레코드다.
// 사용자 발화, 선호, 사실 등 여러 Run() 호출을 넘어 보존할 정보를 담는다.
type Memory struct {
	// ID 는 기억 레코드의 고유 식별자다 (UUID).
	ID string
	// UserID 는 이 기억이 속한 사용자 식별자다.
	UserID string
	// Content 는 기억의 실제 내용이다.
	Content string
	// Tags 는 조회 시 필터링에 사용되는 태그 목록이다.
	// MemoryRepository.LoadByTags 는 OR 조건으로 검색한다.
	Tags []string
	// CreatedAt 은 이 기억이 저장된 시각이다.
	CreatedAt time.Time
}
