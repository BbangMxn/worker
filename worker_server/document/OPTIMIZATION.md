# Backend Optimization Guide

## Overview

이 문서는 Bridgify 백엔드의 성능 및 비용 최적화 전략을 설명합니다.

---

## 1. 3-Tier 캐싱 아키텍처

### 구조

```
┌─────────────────────────────────────────────────────────────┐
│  Tier 1: Redis (Hot Cache)                                  │
│  ├─ TTL: 5-10분                                             │
│  ├─ 용도: 자주 접근하는 데이터                               │
│  └─ 비용: ~$10/월                                           │
├─────────────────────────────────────────────────────────────┤
│  Tier 2: MongoDB (Warm Cache)                               │
│  ├─ TTL: 30일                                               │
│  ├─ 용도: 메일 본문 (압축)                                   │
│  └─ 비용: ~$57/월                                           │
├─────────────────────────────────────────────────────────────┤
│  Tier 3: Provider API (Cold)                                │
│  ├─ TTL: 없음 (on-demand)                                   │
│  ├─ 용도: 캐시 미스 시 원본 조회                             │
│  └─ 비용: 무료 (Rate Limit 주의)                            │
└─────────────────────────────────────────────────────────────┘
```

### 캐시 키 패턴

| 키 패턴 | 용도 | TTL |
|---------|------|-----|
| `body:{email_id}` | 메일 본문 | 10분 |
| `list:{user_id}:{folder}:{page}` | 메일 목록 | 5분 |
| `meta:{email_id}` | 메타데이터 | 10분 |
| `ai:{email_id}` | AI 결과 | 1시간 |

### 사용법

```go
// CacheService 생성
cacheService := service.NewCacheService(
    redisClient,
    mongoRepo,
    mailRepo,
    provider,
    oauthService,
    nil, // 기본 설정 사용
)

// 3-Tier 캐시로 본문 조회
body, err := cacheService.GetBody(ctx, emailID, connectionID)

// 목록 캐시 조회
cached, err := cacheService.GetList(ctx, userID, folder, page)

// Prefetch (백그라운드)
cacheService.PrefetchBodies(ctx, emailIDs, connectionID)
```

---

## 2. LLM 비용 최적화

### 모델 선택 전략

| 작업 | 모델 | 비용/1K | 이유 |
|------|------|---------|------|
| 분류 | gpt-4o-mini | $0.0002 | 단순 분류 |
| 우선순위 | gpt-4o-mini | $0.0002 | 숫자 판단 |
| 태그 추출 | gpt-4o-mini | $0.0002 | 키워드 추출 |
| 짧은 요약 | gpt-4o-mini | $0.0003 | 2-3줄 |
| 상세 요약 | gpt-4o | $0.01 | 품질 필요 |
| 답장 생성 | gpt-4o | $0.02 | 스타일 + 품질 |

### 배치 처리

개별 호출 vs 배치 호출:

```
개별: 10 메일 × 10 API 호출 = 10 호출
배치: 10 메일 × 1 API 호출  = 1 호출 (90% 절감)
```

### 사용법

```go
// 배치 분류
inputs := []llm.BatchClassifyInput{
    {ID: 1, Subject: "회의 일정", From: "boss@company.com", Snippet: "..."},
    {ID: 2, Subject: "뉴스레터", From: "newsletter@...", Snippet: "..."},
}
results, err := llmClient.ClassifyEmailBatch(ctx, inputs, userRules)

// 토큰 최적화
cleanBody := llm.CleanEmailBody(rawBody)  // 시그니처, 인용문 제거
prepared := llm.PrepareEmailForLLM(subject, cleanBody, from, 2000)
```

### 비용 절감 효과

```
최적화 전: $22,000/월 (1,000명 기준)
최적화 후: $6,130/월 (72% 절감)
```

---

## 3. API 응답 최적화

### 목표 응답 시간

| 엔드포인트 | 목표 | P95 | 전략 |
|------------|------|-----|------|
| GET /mail | < 50ms | < 100ms | Redis 캐시 |
| GET /mail/:id | < 30ms | < 50ms | Redis 캐시 |
| GET /mail/:id/body | < 100ms | < 200ms | 3-Tier 캐시 |
| POST /mail | < 200ms | < 500ms | 비동기 큐 |

### Prefetch 전략

```go
// 목록 조회 시 다음 페이지 프리페치
cacheService.PrefetchNextPage(ctx, userID, folder, currentPage)

// 목록 조회 시 처음 5개 본문 프리페치
cacheService.PrefetchBodies(ctx, first5IDs, connectionID)

// 메일 열람 시 다음 2개 프리페치
cacheService.PrefetchBodies(ctx, next2IDs, connectionID)
```

---

## 4. Worker 최적화

### 배치 처리

```go
// AI Processor는 자동으로 배치 축적
processor := worker.NewAIProcessor(aiService)

// 개별 요청도 내부적으로 배치 처리됨
// - batchSize: 10
// - batchTimeout: 3초
```

### 처리 흐름

```
개별 분류 요청 도착
       │
       ▼
배치 큐에 추가
       │
       ├── 10개 도달 → 즉시 처리
       └── 3초 경과 → 타이머 처리
       │
       ▼
LLM 배치 API 호출 (1회)
       │
       ▼
결과 분배 및 DB 업데이트
```

---

## 5. 동기화 최적화

### 2단계 동기화

**Phase 1: 메타데이터 우선 (빠른 UI 표시)**
```
1. Gmail API: messages.list
2. Gmail API: messages.get (metadata format) - 배치
3. PostgreSQL: 메타데이터 저장
4. SSE: sync_progress 이벤트
5. 클라이언트: 목록 표시 가능 ✓
소요시간: ~3초
```

**Phase 2: 본문 + AI (백그라운드)**
```
1. Redis Stream: ai.classify.batch 작업 발행
2. Worker: 본문 가져오기 + MongoDB 저장
3. Worker: AI 분류 (배치)
4. SSE: email_updated 이벤트
소요시간: ~30초 (백그라운드)
```

---

## 6. 비용 시뮬레이션

### 1,000명 사용자 기준 월간 비용

| 항목 | 최적화 전 | 최적화 후 | 절감 |
|------|-----------|-----------|------|
| LLM - 분류 | $15,000 | $30 | 99.8% |
| LLM - 답장 | $6,000 | $6,000 | 0% |
| LLM - 기타 | $1,000 | $100 | 90% |
| **LLM 소계** | **$22,000** | **$6,130** | **72%** |
| PostgreSQL | $25 | $25 | - |
| MongoDB | $57 | $57 | - |
| Redis | $10 | $10 | - |
| 서버 | $20 | $20 | - |
| **총 비용** | **$22,112** | **$6,242** | **71.8%** |
| **유저당** | **$22.11** | **$6.24** | - |

---

## 7. 파일 구조

```
backend/
├── core/
│   ├── service/
│   │   ├── cache.go           # 3-Tier 캐싱 서비스
│   │   ├── ai_optimized.go    # 최적화된 AI 서비스
│   │   └── ...
│   └── agent/
│       └── llm/
│           ├── client.go      # LLM 클라이언트
│           └── batch.go       # 배치 처리 + 토큰 최적화
│
├── adapter/
│   └── in/
│       ├── http/
│       │   └── mail_optimized.go  # 최적화된 API 핸들러
│       └── worker/
│           └── ai_processor.go    # 배치 AI 프로세서
│
└── document/
    └── OPTIMIZATION.md        # 이 문서
```

---

## 8. 환경변수

```env
# Cache TTLs
CACHE_BODY_TTL=10m
CACHE_LIST_TTL=5m
CACHE_AI_TTL=1h
CACHE_MONGO_TTL_DAYS=30

# LLM
OPENAI_API_KEY=sk-...
LLM_BATCH_SIZE=10
LLM_BATCH_TIMEOUT=3s

# Worker
WORKER_MIN=2
WORKER_MAX=20
```

---

## 9. 모니터링

### 캐시 통계 엔드포인트

```
GET /v2/mail/cache/stats

Response:
{
  "enabled": true,
  "redis_hits": 1234,
  "redis_misses": 56,
  "mongo_hits": 45,
  "mongo_misses": 11,
  "provider_hits": 11,
  "hit_rate": 0.95
}
```

### AI 통계

```go
metrics := aiService.GetMetrics()
// TotalClassified, TotalSummarized, BatchesProcessed, EstimatedCostUSD
```

---

## 10. 추가 최적화 옵션

1. **OpenAI Batch API** (50% 할인, 24시간 지연)
   - 비긴급 분류 작업에 적용 가능

2. **로컬 LLM (Ollama)**
   - 분류 작업 로컬 처리
   - 초기 비용 ↑, 운영 비용 ↓

3. **CDN 캐싱**
   - 정적 첨부파일 캐싱

4. **Database 읽기 복제본**
   - 읽기 트래픽 분산
