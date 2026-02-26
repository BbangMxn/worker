package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// generateWorkerID creates a unique worker ID using hostname and PID
func generateWorkerID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

type Config struct {
	Port        string
	Environment string

	// Database
	DatabaseURL string
	DirectURL   string
	MongoDBURL  string
	MongoDBName string
	RedisURL    string

	// Neo4j
	Neo4jURL      string
	Neo4jUsername string
	Neo4jPassword string

	// JWT
	JWTSecret string

	// Supabase
	SupabaseURL            string
	SupabaseAnonKey        string
	SupabaseServiceRoleKey string

	// OpenAI
	OpenAIAPIKey   string
	LLMModel       string
	LLMMaxTokens   int
	LLMTemperature float64
	LLMTimeoutSec  int
	LLMMaxRetries  int

	// OAuth - Google
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	GoogleProjectID    string

	// OAuth - Microsoft
	MicrosoftClientID     string
	MicrosoftClientSecret string
	MicrosoftRedirectURL  string
	MicrosoftTenantID     string

	// Worker
	WorkerID            string
	WorkerMin           int
	WorkerMax           int
	WorkerQueueSize     int
	WorkerScaleInterval time.Duration
	WorkerIdleTimeout   time.Duration

	// Consumer (Redis Stream)
	ConsumerBatchSize       int
	ConsumerBlockMS         int
	ConsumerMaxRetries      int
	ConsumerPendingCheckSec int
	ConsumerRetryDelaySec   int

	// Cache
	CacheDefaultTTLMin  int
	CacheEmailTTLMin    int
	CacheCalendarTTLMin int
	CacheSessionTTLHour int
	CacheMaxEntries     int

	// WebSocket
	WSMaxMessageSize  int
	WSPingIntervalSec int
	WSPongWaitSec     int
	WSWriteWaitSec    int

	// Webhook
	WebhookTimeoutSec    int
	WebhookMaxRetries    int
	WebhookRetryDelaySec int
	WebhookWorkerCount   int

	// CORS
	AllowedOrigins []string

	// Scheduler
	SchedulerEnabled bool
}

func Load() (*Config, error) {
	return &Config{
		Port:        getEnv("PORT", "8080"),
		Environment: getEnv("ENV", "development"),

		// Database
		DatabaseURL: getEnv("DATABASE_URL", ""),
		DirectURL:   getEnv("DIRECT_URL", ""),
		MongoDBURL:  getEnv("MONGODB_URL", ""),
		MongoDBName: getEnv("MONGODB_DATABASE", "bridgify"),
		RedisURL:    getEnv("REDIS_URL", ""),

		// Neo4j
		Neo4jURL:      getEnv("NEO4J_URL", ""),
		Neo4jUsername: getEnv("NEO4J_USERNAME", "neo4j"),
		Neo4jPassword: getEnv("NEO4J_PASSWORD", ""),

		// JWT
		JWTSecret: getEnv("SUPABASE_JWT_SECRET", ""),

		// Supabase
		SupabaseURL:            getEnv("SUPABASE_URL", ""),
		SupabaseAnonKey:        getEnv("SUPABASE_ANON_KEY", ""),
		SupabaseServiceRoleKey: getEnv("SUPABASE_SERVICE_ROLE_KEY", ""),

		// OpenAI
		OpenAIAPIKey:   getEnv("OPENAI_API_KEY", ""),
		LLMModel:       getEnv("LLM_MODEL", "gpt-4o-mini"),
		LLMMaxTokens:   getEnvInt("LLM_MAX_TOKENS", 2048),
		LLMTemperature: getEnvFloat("LLM_TEMPERATURE", 0.7),
		LLMTimeoutSec:  getEnvInt("LLM_TIMEOUT_SEC", 60),
		LLMMaxRetries:  getEnvInt("LLM_MAX_RETRIES", 3),

		// OAuth - Google
		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", ""),
		GoogleProjectID:    getEnv("GOOGLE_PROJECT_ID", ""),

		// OAuth - Microsoft
		MicrosoftClientID:     getEnv("MICROSOFT_CLIENT_ID", ""),
		MicrosoftClientSecret: getEnv("MICROSOFT_CLIENT_SECRET", ""),
		MicrosoftRedirectURL:  getEnv("MICROSOFT_REDIRECT_URL", ""),
		MicrosoftTenantID:     getEnv("MICROSOFT_TENANT_ID", "common"),

		// Worker
		WorkerID:            getEnv("WORKER_ID", generateWorkerID()),
		WorkerMin:           getEnvInt("WORKER_MIN", 2),
		WorkerMax:           getEnvInt("WORKER_MAX", 20),
		WorkerQueueSize:     getEnvInt("WORKER_QUEUE_SIZE", 1000),
		WorkerScaleInterval: time.Duration(getEnvInt("WORKER_SCALE_INTERVAL_SEC", 10)) * time.Second,
		WorkerIdleTimeout:   time.Duration(getEnvInt("WORKER_IDLE_TIMEOUT_SEC", 30)) * time.Second,

		// Consumer
		ConsumerBatchSize:       getEnvInt("CONSUMER_BATCH_SIZE", 50),
		ConsumerBlockMS:         getEnvInt("CONSUMER_BLOCK_MS", 5000),
		ConsumerMaxRetries:      getEnvInt("CONSUMER_MAX_RETRIES", 3),
		ConsumerPendingCheckSec: getEnvInt("CONSUMER_PENDING_CHECK_SEC", 60),
		ConsumerRetryDelaySec:   getEnvInt("CONSUMER_RETRY_DELAY_SEC", 5),

		// Cache
		CacheDefaultTTLMin:  getEnvInt("CACHE_DEFAULT_TTL_MIN", 30),
		CacheEmailTTLMin:    getEnvInt("CACHE_EMAIL_TTL_MIN", 60),
		CacheCalendarTTLMin: getEnvInt("CACHE_CALENDAR_TTL_MIN", 15),
		CacheSessionTTLHour: getEnvInt("CACHE_SESSION_TTL_HOUR", 24),
		CacheMaxEntries:     getEnvInt("CACHE_MAX_ENTRIES", 10000),

		// WebSocket
		WSMaxMessageSize:  getEnvInt("WS_MAX_MESSAGE_SIZE", 524288),
		WSPingIntervalSec: getEnvInt("WS_PING_INTERVAL_SEC", 25),
		WSPongWaitSec:     getEnvInt("WS_PONG_WAIT_SEC", 60),
		WSWriteWaitSec:    getEnvInt("WS_WRITE_WAIT_SEC", 10),

		// Webhook
		WebhookTimeoutSec:    getEnvInt("WEBHOOK_TIMEOUT_SEC", 30),
		WebhookMaxRetries:    getEnvInt("WEBHOOK_MAX_RETRIES", 3),
		WebhookRetryDelaySec: getEnvInt("WEBHOOK_RETRY_DELAY_SEC", 5),
		WebhookWorkerCount:   getEnvInt("WEBHOOK_WORKER_COUNT", 10),

		// CORS
		AllowedOrigins: getEnvSlice("ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://localhost:5173"}),

		// Scheduler
		SchedulerEnabled: getEnvBool("SCHEDULER_ENABLED", true),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}
