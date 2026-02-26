package database

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// PostgresConfig holds database configuration.
type PostgresConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// DefaultPostgresConfig returns optimized defaults.
func DefaultPostgresConfig() *PostgresConfig {
	maxConns := int32(25)
	if envMax := os.Getenv("DB_MAX_CONNS"); envMax != "" {
		if v, err := strconv.Atoi(envMax); err == nil {
			maxConns = int32(v)
		}
	}

	return &PostgresConfig{
		MaxConns:          maxConns,
		MinConns:          5,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: 1 * time.Minute,
	}
}

func NewPostgres(databaseURL string) (*pgxpool.Pool, error) {
	return NewPostgresWithConfig(databaseURL, DefaultPostgresConfig())
}

func NewPostgresWithConfig(databaseURL string, cfg *PostgresConfig) (*pgxpool.Pool, error) {
	if cfg == nil {
		cfg = DefaultPostgresConfig()
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	config.MaxConns = cfg.MaxConns
	config.MinConns = cfg.MinConns
	config.MaxConnLifetime = cfg.MaxConnLifetime
	config.MaxConnIdleTime = cfg.MaxConnIdleTime
	config.HealthCheckPeriod = cfg.HealthCheckPeriod

	// Disable prepared statement cache to avoid conflicts with sqlx
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, err
	}

	return pool, nil
}

// PoolStats returns connection pool statistics.
type PoolStats struct {
	TotalConns      int32 `json:"total_conns"`
	AcquiredConns   int32 `json:"acquired_conns"`
	IdleConns       int32 `json:"idle_conns"`
	MaxConns        int32 `json:"max_conns"`
	AcquireCount    int64 `json:"acquire_count"`
	AcquireDuration int64 `json:"acquire_duration_ms"`
}

// GetPoolStats returns pool statistics.
func GetPoolStats(pool *pgxpool.Pool) *PoolStats {
	stat := pool.Stat()
	return &PoolStats{
		TotalConns:      stat.TotalConns(),
		AcquiredConns:   stat.AcquiredConns(),
		IdleConns:       stat.IdleConns(),
		MaxConns:        stat.MaxConns(),
		AcquireCount:    stat.AcquireCount(),
		AcquireDuration: stat.AcquireDuration().Milliseconds(),
	}
}

// RedisConfig holds Redis configuration.
type RedisConfig struct {
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultRedisConfig returns optimized Redis defaults.
func DefaultRedisConfig() *RedisConfig {
	poolSize := 50
	if envPool := os.Getenv("REDIS_POOL_SIZE"); envPool != "" {
		if v, err := strconv.Atoi(envPool); err == nil {
			poolSize = v
		}
	}

	return &RedisConfig{
		PoolSize:     poolSize,
		MinIdleConns: 10,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
}

func NewRedis(redisURL string) (*redis.Client, error) {
	return NewRedisWithConfig(redisURL, DefaultRedisConfig())
}

func NewRedisWithConfig(redisURL string, cfg *RedisConfig) (*redis.Client, error) {
	if cfg == nil {
		cfg = DefaultRedisConfig()
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	// 최적화된 설정 적용
	opt.PoolSize = cfg.PoolSize
	opt.MinIdleConns = cfg.MinIdleConns
	opt.MaxRetries = cfg.MaxRetries
	opt.DialTimeout = cfg.DialTimeout
	opt.ReadTimeout = cfg.ReadTimeout
	opt.WriteTimeout = cfg.WriteTimeout

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return client, nil
}

// RedisStats returns Redis pool statistics.
type RedisStats struct {
	Hits       uint32 `json:"hits"`
	Misses     uint32 `json:"misses"`
	Timeouts   uint32 `json:"timeouts"`
	TotalConns uint32 `json:"total_conns"`
	IdleConns  uint32 `json:"idle_conns"`
	StaleConns uint32 `json:"stale_conns"`
}

// GetRedisStats returns Redis pool statistics.
func GetRedisStats(client *redis.Client) *RedisStats {
	stat := client.PoolStats()
	return &RedisStats{
		Hits:       stat.Hits,
		Misses:     stat.Misses,
		Timeouts:   stat.Timeouts,
		TotalConns: stat.TotalConns,
		IdleConns:  stat.IdleConns,
		StaleConns: stat.StaleConns,
	}
}
