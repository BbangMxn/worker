# Deployment Guide

> Docker + Railway 기반 배포 가이드

---

## 1. Startup Modes

서버는 `--mode` 플래그로 3가지 모드를 지원합니다.

```bash
./server --mode=api       # API 서버만 (HTTP 요청 처리, SSE, Webhook)
./server --mode=worker    # Worker만 (Redis Stream 소비, 비동기 작업, 스케줄링)
./server --mode=all       # API + Worker 동시 실행 (기본값)
```

| Mode | Process | Use Case |
|------|---------|----------|
| `api` | Fiber HTTP Server | API 전용 인스턴스, 수평 확장 |
| `worker` | Redis Stream Consumer + Schedulers | 백그라운드 작업 전용 |
| `all` | Worker(goroutine) + API(main) | 단일 인스턴스 배포 |

---

## 2. Docker

### Dockerfile (Multi-stage Build)

```dockerfile
# Stage 1: Build
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o server .

# Stage 2: Runtime
FROM alpine:3.21
RUN apk add --no-cache curl ca-certificates tzdata
RUN adduser -D -u 1001 appuser
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations
RUN chown -R appuser:appuser /app
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1
CMD ["./server", "--mode=all"]
```

**최적화 포인트:**
- `CGO_ENABLED=0`: 정적 바이너리, alpine 호환
- `-ldflags="-w -s"`: 디버그 심볼 제거, 바이너리 크기 감소
- non-root user (`appuser:1001`)
- `alpine:3.21` 최소 이미지

### Build & Run

```bash
# 빌드
cd worker_server
docker build -t bridgify-server .

# 실행
docker run -p 8080:8080 --env-file .env bridgify-server

# API 모드만
docker run -p 8080:8080 --env-file .env bridgify-server ./server --mode=api

# Worker 모드만
docker run --env-file .env bridgify-server ./server --mode=worker
```

---

## 3. Railway Deployment

### railway.toml

```toml
[build]
builder = "dockerfile"
dockerfilePath = "./Dockerfile"
watchPatterns = ["**/*.go", "go.mod", "go.sum", "Dockerfile"]

[deploy]
healthcheckPath = "/health"
healthcheckTimeout = 100
restartPolicyType = "on_failure"
restartPolicyMaxRetries = 3
```

### Infrastructure Topology

```
Railway (Go Server)
    │
    ├──→ Supabase (PostgreSQL + Auth + pgvector)
    ├──→ MongoDB Atlas
    ├──→ Redis (Upstash / Railway)
    ├──→ Neo4j (Aura / Self-hosted)
    └──→ Google Cloud (Pub/Sub, Gmail API, Calendar API)
```

---

## 4. Health Check

### `/health` (Liveness)

서버 프로세스가 살아있는지만 확인합니다.

```json
{
  "status": "ok",
  "timestamp": "2026-02-26T12:34:56Z"
}
```

- 항상 200 OK
- 외부 의존성 확인 없음
- Docker HEALTHCHECK, Railway healthcheckPath에 사용

### `/ready` (Readiness)

DB/Redis 연결 상태를 확인합니다.

```json
{
  "status": "ready",
  "checks": {
    "postgres": "healthy",
    "redis": "healthy"
  },
  "timestamp": "2026-02-26T12:34:56Z"
}
```

- 200 OK: 모든 의존성 정상
- 503 Service Unavailable: 하나라도 실패
- 체크당 5초 타임아웃

---

## 5. Environment Variables

### Required

```bash
# Server
PORT=8080
ENV=development              # development | production

# PostgreSQL (Supabase)
DATABASE_URL=postgresql://...
DIRECT_URL=postgresql://...  # pgBouncer bypass (migrations용)

# MongoDB
MONGODB_URL=mongodb://...
MONGODB_DATABASE=bridgify

# Redis
REDIS_URL=redis://...

# Neo4j
NEO4J_URL=bolt://...
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=...

# Supabase Auth
SUPABASE_URL=https://...
SUPABASE_ANON_KEY=...
SUPABASE_SERVICE_ROLE_KEY=...
SUPABASE_JWT_SECRET=...

# OpenAI
OPENAI_API_KEY=sk-...
LLM_MODEL=gpt-4o-mini

# Google OAuth
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/oauth/google/callback
GOOGLE_PROJECT_ID=...

# Microsoft OAuth
MICROSOFT_CLIENT_ID=...
MICROSOFT_CLIENT_SECRET=...
MICROSOFT_REDIRECT_URL=http://localhost:8080/api/v1/oauth/microsoft/callback
MICROSOFT_TENANT_ID=...
```

### Optional (with defaults)

```bash
# LLM
LLM_MAX_TOKENS=2048
LLM_TEMPERATURE=0.7
LLM_TIMEOUT_SEC=60
LLM_MAX_RETRIES=3

# Worker Pool
WORKER_MIN=4                 # 최소 고루틴 수
WORKER_MAX=16                # 최대 고루틴 수
WORKER_QUEUE_SIZE=10000      # 작업 큐 크기
WORKER_SCALE_THRESHOLD=0.7   # 스케일 업 임계값

# Redis Stream Consumer
CONSUMER_BATCH_SIZE=50
CONSUMER_BLOCK_MS=5000
CONSUMER_MAX_RETRIES=3
CONSUMER_PENDING_CHECK_SEC=60
CONSUMER_RETRY_DELAY_SEC=5

# Cache
CACHE_DEFAULT_TTL_MIN=30
CACHE_EMAIL_TTL_MIN=60
CACHE_CALENDAR_TTL_MIN=15
CACHE_SESSION_TTL_HOUR=24
CACHE_MAX_ENTRIES=10000

# WebSocket
WS_MAX_MESSAGE_SIZE=524288   # 512KB
WS_PING_INTERVAL_SEC=25
WS_PONG_WAIT_SEC=60

# Webhook
WEBHOOK_TIMEOUT_SEC=30
WEBHOOK_MAX_RETRIES=3
WEBHOOK_WORKER_COUNT=10

# CORS
ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173

# Scheduler
SCHEDULER_ENABLED=true
```

---

## 6. Middleware Stack

API 서버에 적용되는 미들웨어 (적용 순서):

| Order | Middleware | Description |
|-------|-----------|-------------|
| 1 | Panic Recovery | 패닉 발생 시 500 반환, 서버 유지 |
| 2 | Request ID | `X-Request-ID` 헤더 생성 |
| 3 | Security Headers | XSS, MIME sniffing 방지 |
| 4 | Path Traversal Protection | `../` 경로 차단 |
| 5 | Input Sanitization | XSS 입력 필터링 |
| 6 | Request Logging | zerolog 구조화 로깅 |
| 7 | Response Compression | gzip/brotli 압축 |
| 8 | ETag Caching | 응답 캐시 (304 Not Modified) |
| 9 | CORS | Origin 기반 접근 제어 |

### CORS Configuration

| Environment | Origins |
|-------------|---------|
| Development | `localhost:3000`, `localhost:5173` |
| Production | 명시적 ALLOWED_ORIGINS 필수 (`*` 차단) |

---

## 7. Fiber Server Configuration

```go
fiber.Config{
    ReadBufferSize:  16 * 1024,          // 16KB
    WriteBufferSize: 16 * 1024,          // 16KB
    JSONEncoder:     gojson.Marshal,      // goccy/go-json (2-3x faster)
    JSONDecoder:     gojson.Unmarshal,
    BodyLimit:       10 * 1024 * 1024,   // 10MB
    Concurrency:     256 * 1024,          // 256K connections
    EnableSplitting: true,               // Streaming 지원
}
```

---

## 8. Graceful Shutdown

```
SIGINT / SIGTERM 수신
    │
    ├─ 30초 타임아웃 시작
    │
    ├─ API 서버: 새 요청 거부 → 진행 중인 요청 완료 대기
    ├─ Worker Pool: 진행 중인 Job 완료 대기 → 드레인
    ├─ Schedulers: 중지
    ├─ DB 연결 정리: PostgreSQL, MongoDB, Redis, Neo4j
    │
    └─ 타임아웃 초과 시 강제 종료
```

---

## 9. Worker Initialization

### Worker Pool

| Setting | Default | Description |
|---------|---------|-------------|
| Min Workers | 4 | 최소 고루틴 수 |
| Max Workers | 16 | 최대 고루틴 수 |
| Queue Size | 1,000 | 작업 대기열 |
| Scale Up Threshold | 0.8 | 큐 사용률 80% 이상 시 스케일 업 |
| Scale Down Threshold | 0.2 | 큐 사용률 20% 이하 시 스케일 다운 |

### Redis Stream Consumer

| Setting | Default | Description |
|---------|---------|-------------|
| Group | `workspace-workers` | Consumer Group 이름 |
| Consumer | `{hostname}-{pid}` | Consumer 식별자 |
| Batch Size | 50 | 1회 읽기 메시지 수 |
| Block MS | 5,000 | 블로킹 대기 시간 |
| Max Retries | 3 | 최대 재시도 횟수 |
| Pending Check | 60초 | Pending 메시지 확인 주기 |

### Monitored Streams

```
mail:sync, mail:send, mail:batch, mail:save, mail:modify
calendar:sync
ai:classify, ai:summarize, ai:reply
rag:index, rag:batch
```

### Schedulers

| Scheduler | Interval | Description |
|-----------|----------|-------------|
| Watch Renew | 매 6시간 | Gmail Watch 7일 만료 전 자동 갱신 |
| Sync Retry | 매 1분 | 실패한 동기화 작업 재시도 |
| Gap Sync | 서버 시작 시 1회 | 서버 다운타임 동안 놓친 이메일 보정 |

---

## 10. Dependencies

### Backend (Go 1.24)

| Package | Purpose |
|---------|---------|
| `gofiber/fiber/v2` | HTTP Framework |
| `jackc/pgx/v5` | PostgreSQL Driver |
| `redis/go-redis/v9` | Redis Client |
| `mongo-driver` | MongoDB Driver |
| `neo4j/neo4j-go-driver/v5` | Neo4j Driver |
| `sashabaranov/go-openai` | OpenAI API |
| `golang.org/x/oauth2` | OAuth2 Client |
| `google.golang.org/api` | Gmail/Calendar API |
| `golang-jwt/jwt/v5` | JWT Auth |
| `rs/zerolog` | Structured Logging |
| `goccy/go-json` | Fast JSON |
| `sony/gobreaker` | Circuit Breaker |

### Frontend (Node.js 18+)

| Package | Purpose |
|---------|---------|
| `next@14` | React Framework |
| `@supabase/ssr` | Supabase Auth |
| `framer-motion` | Animation |
| `lucide-react` | Icons |
| `tailwindcss@3.4` | Styling |

---

## 11. Deployment Checklist

### Prerequisites

- [ ] Go 1.24+
- [ ] PostgreSQL 15+ with pgvector extension
- [ ] MongoDB 7+
- [ ] Redis 7+
- [ ] Neo4j 5+
- [ ] Google Cloud Console project (Gmail API, Calendar API, Pub/Sub)
- [ ] OpenAI API Key
- [ ] Supabase project (Auth + Database)

### First Deploy

```bash
# 1. 환경변수 설정
cp .env.example .env
# .env 편집

# 2. 마이그레이션 실행
# migrations/ 폴더의 SQL 파일을 순서대로 PostgreSQL에 실행

# 3. Docker 빌드 & 실행
docker build -t bridgify-server .
docker run -p 8080:8080 --env-file .env bridgify-server

# 4. 헬스체크 확인
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

### Railway Deploy

```bash
# 1. Railway CLI 설치
npm install -g @railway/cli

# 2. 프로젝트 연결
railway link

# 3. 환경변수 설정 (Railway Dashboard 또는 CLI)
railway variables set PORT=8080
railway variables set ENV=production
# ... 나머지 환경변수

# 4. 배포
railway up
```

### Verification

```bash
# Liveness
curl -f http://localhost:8080/health

# Readiness (DB + Redis)
curl -f http://localhost:8080/ready

# API 테스트
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/v1/mail
```
