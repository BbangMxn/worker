package cache

import (
	"context"
	"time"

	"github.com/goccy/go-json"

	"github.com/redis/go-redis/v9"
)

// RedisCache Redis 기반 캐시 구현
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache 새 Redis 캐시 생성
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

// Get 캐시에서 값 조회
func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Set 캐시에 값 저장
func (c *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Delete 캐시에서 키 삭제
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// Exists 키 존재 여부 확인
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := c.client.Exists(ctx, key).Result()
	return result > 0, err
}

// GetJSON JSON으로 저장된 값 조회
func (c *RedisCache) GetJSON(ctx context.Context, key string, dest interface{}) (bool, error) {
	data, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return false, err
	}

	return true, nil
}

// SetJSON 값을 JSON으로 저장
func (c *RedisCache) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

// GetMulti 여러 키 한번에 조회
func (c *RedisCache) GetMulti(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return make(map[string]string), nil
	}

	values, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for i, key := range keys {
		if values[i] != nil {
			result[key] = values[i].(string)
		}
	}

	return result, nil
}

// SetMulti 여러 키-값 한번에 저장
func (c *RedisCache) SetMulti(ctx context.Context, items map[string]string, ttl time.Duration) error {
	if len(items) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()
	for key, value := range items {
		pipe.Set(ctx, key, value, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// DeleteMulti 여러 키 삭제
func (c *RedisCache) DeleteMulti(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// Increment 값 증가
func (c *RedisCache) Increment(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, key).Result()
}

// IncrementBy 지정된 값만큼 증가
func (c *RedisCache) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.client.IncrBy(ctx, key, value).Result()
}

// Expire TTL 설정
func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

// TTL 남은 TTL 조회
func (c *RedisCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

// Close 연결 종료
func (c *RedisCache) Close() error {
	return c.client.Close()
}
