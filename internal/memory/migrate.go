package memory

import (
	"context"
	"database/sql"
	"fmt"
)

// Migrate 는 Long-term Memory 저장에 필요한 Postgres 스키마를 초기화한다.
// 앱 시작 시 DB 연결 직후 호출하며, 이미 존재할 경우 아무 동작도 하지 않는다.
// Phase 9 에서 golang-migrate 등 전용 도구로 교체하는 것을 검토할 수 있다.
func Migrate(ctx context.Context, db *sql.DB) error {
	const createTable = `
CREATE TABLE IF NOT EXISTS memories (
	id         UUID        PRIMARY KEY,
	user_id    TEXT        NOT NULL,
	content    TEXT        NOT NULL,
	tags       TEXT[]      NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

	const createTagsIndex = `
CREATE INDEX IF NOT EXISTS memories_tags_gin_idx
	ON memories USING GIN (tags)`

	if _, err := db.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("memory: create memories table: %w", err)
	}

	if _, err := db.ExecContext(ctx, createTagsIndex); err != nil {
		return fmt.Errorf("memory: create memories tags index: %w", err)
	}

	return nil
}
