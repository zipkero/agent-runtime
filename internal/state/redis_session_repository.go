package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const sessionKeyPrefix = "session:"

// RedisSessionRepository 는 Redis에 SessionState를 JSON으로 직렬화하여 저장하는 구현체다.
// 프로세스 재시작 후에도 세션이 복원된다 (AOF persistence 활성화 전제).
type RedisSessionRepository struct {
	client *redis.Client
}

// NewRedisSessionRepository 는 주어진 Redis 클라이언트로 RedisSessionRepository를 생성한다.
func NewRedisSessionRepository(client *redis.Client) *RedisSessionRepository {
	return &RedisSessionRepository{client: client}
}

// Load 는 sessionID에 해당하는 SessionState를 Redis에서 조회한다.
// 존재하지 않으면 빈 SessionState와 nil error를 반환한다.
func (r *RedisSessionRepository) Load(ctx context.Context, sessionID string) (SessionState, error) {
	key := sessionKeyPrefix + sessionID

	data, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return SessionState{}, nil
	}
	if err != nil {
		return SessionState{}, fmt.Errorf("redis GET %s: %w", key, err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return SessionState{}, fmt.Errorf("unmarshal session %s: %w", sessionID, err)
	}
	return state, nil
}

// Save 는 sessionID에 SessionState를 JSON으로 직렬화하여 Redis에 저장한다.
func (r *RedisSessionRepository) Save(ctx context.Context, sessionID string, state SessionState) error {
	key := sessionKeyPrefix + sessionID

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal session %s: %w", sessionID, err)
	}

	if err := r.client.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("redis SET %s: %w", key, err)
	}
	return nil
}
