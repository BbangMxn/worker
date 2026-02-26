package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// OAuthStateKey Redis key prefix for OAuth state
const OAuthStateKey = "oauth:state:"

// RedisOAuthStateStore Redis 기반 OAuth state 저장소 (CSRF 보호)
type RedisOAuthStateStore struct {
	client *redis.Client
}

// NewRedisOAuthStateStore 새 Redis OAuth state 저장소 생성
func NewRedisOAuthStateStore(client *redis.Client) *RedisOAuthStateStore {
	return &RedisOAuthStateStore{client: client}
}

// StoreState state를 Redis에 저장
func (s *RedisOAuthStateStore) StoreState(ctx context.Context, state string, userID uuid.UUID, ttl time.Duration) error {
	if state == "" {
		return errors.New("state cannot be empty")
	}
	if userID == uuid.Nil {
		return errors.New("userID cannot be nil")
	}

	key := OAuthStateKey + state
	err := s.client.Set(ctx, key, userID.String(), ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to store OAuth state: %w", err)
	}

	return nil
}

// ValidateState state 검증 및 userID 반환 (검증 후 즉시 삭제 - 일회용)
func (s *RedisOAuthStateStore) ValidateState(ctx context.Context, state string) (uuid.UUID, error) {
	if state == "" {
		return uuid.Nil, errors.New("state cannot be empty")
	}

	key := OAuthStateKey + state

	// GETDEL: 값을 가져오면서 동시에 삭제 (atomic operation, 재사용 방지)
	userIDStr, err := s.client.GetDel(ctx, key).Result()
	if err == redis.Nil {
		return uuid.Nil, errors.New("state not found or expired")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to validate OAuth state: %w", err)
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid userID in state: %w", err)
	}

	return userID, nil
}

// CleanupExpiredStates 만료된 state 정리 (Redis TTL이 자동으로 처리하므로 필요 없지만 명시적 정리용)
func (s *RedisOAuthStateStore) CleanupExpiredStates(ctx context.Context) error {
	// Redis TTL이 자동으로 만료된 키를 삭제하므로 별도 정리 불필요
	// 이 메서드는 수동 정리가 필요할 때를 위해 존재
	return nil
}
