# Backend - AI 업무 자동화 플랫폼

> **핵심 목표**: AI Agent를 통해 메일, 캘린더, 연락처를 통합 관리하는 업무 자동화 플랫폼

---

## 프로젝트 정보

| 항목 | 내용 |
|------|------|
| Language | Go 1.21+ |
| Framework | Fiber v2 |
| Architecture | Hexagonal (Ports & Adapters) |
| Database | PostgreSQL (Supabase), MongoDB, Neo4j |
| Queue | Redis Stream |
| AI | OpenAI GPT-4o-mini |
| Provider | Gmail API, Outlook API, Google Calendar |

---

## 디렉토리 구조

```
backend/
├── main.go                    # 진입점
├── config/                    # 설정 로드
│
├── core/                      # 핵심 비즈니스 로직
│   ├── domain/                # 도메인 모델 (Entity, Value Object)
│   ├── port/
│   │   ├── in/                # 인바운드 포트 (UseCase 인터페이스)
│   │   └── out/               # 아웃바운드 포트 (Repository 인터페이스)
│   ├── service/               # 서비스 구현 (→ CLAUDE.md)
│   │   ├── ai/                # AI 서비스 (분류, 요약, 답장)
│   │   ├── auth/              # OAuth, 설정
│   │   ├── mail/              # 메일 서비스
│   │   ├── calendar/          # 캘린더 서비스
│   │   ├── classification/    # 4단계 분류 파이프라인
│   │   └── common/            # 공용 유틸 (캐시, 에러)
│   └── agent/                 # AI Agent (→ CLAUDE.md)
│       ├── orchestrator.go    # 중앙 오케스트레이터
│       ├── llm/               # LLM 클라이언트 (OpenAI)
│       ├── rag/               # RAG 시스템 (pgvector + Neo4j)
│       ├── tools/             # 도구 시스템 (Function Calling)
│       └── session/           # 세션/Proposal 관리
│
├── adapter/
│   ├── in/                    # 인바운드 어댑터 (→ CLAUDE.md)
│   │   ├── http/              # REST API (Fiber)
│   │   └── worker/            # Redis Stream Worker
│   └── out/                   # 아웃바운드 어댑터 (→ CLAUDE.md)
│       ├── persistence/       # PostgreSQL
│       ├── mongodb/           # MongoDB (본문 저장)
│       ├── graph/             # Neo4j (개인화)
│       ├── messaging/         # Redis Stream
│       └── provider/          # Gmail, Outlook API
│
├── internal/
│   └── bootstrap/             # 의존성 주입, 초기화
│       ├── deps.go            # 의존성 조립
│       ├── api.go             # API 서버 초기화
│       └── worker.go          # Worker 초기화
│
├── pkg/                       # 공용 패키지
│   ├── logger/                # 구조화 로깅
│   └── ratelimit/             # Rate Limiting, 캐시
│
└── migrations/                # DB 마이그레이션 (SQL)
```

---

## 핵심 설계

### 1. 데이터 저장소 분리

| 저장소 | 역할 | 데이터 |
|--------|------|--------|
| **PostgreSQL** | 시스템 필수 데이터 | 사용자, OAuth, 메일 메타, 분류 결과, 설정 |
| **MongoDB** | 대용량 본문 | 이메일 본문 (gzip 압축, 30일 TTL) |
| **Neo4j** | 개인화 그래프 | 작문 스타일, 관계, 톤 선호도 |
| **Redis** | 큐 + 캐시 | Stream (작업 큐), L2 캐시 |
| **pgvector** | RAG 벡터 | 이메일 임베딩 (1536차원) |

### 2. 4단계 분류 파이프라인 (~75% LLM 비용 절감)

```
Stage 0: User Rules (10%)   → ImportantDomains, Keywords
Stage 1: Headers (35%)      → List-Unsubscribe, Precedence
Stage 2: Known Domain (30%) → SenderProfile, KnownDomain DB
Stage 3: LLM (25%)          → 나머지만 OpenAI 호출
```

### 3. AI Agent Proposal 기반 액션

수정 작업은 직접 실행하지 않고 **Proposal 생성 → 사용자 확인 → 실행**:

```go
// 지원 액션
mail.send, mail.reply, mail.delete, mail.archive
calendar.create, calendar.update, calendar.delete
label.add, label.remove, label.create
```

### 4. Push 기반 실시간 동기화 (Superhuman 스타일)

```
Gmail Pub/Sub → Webhook → Delta Sync → SSE Broadcast
```

- Gmail Watch로 실시간 알림 수신
- historyId 기반 증분 동기화
- SSE로 클라이언트에 실시간 전달

### 5. RAG 시스템

- **pgvector**: 이메일 임베딩 저장 (시맨틱 검색)
- **Neo4j**: 사용자 작문 스타일/관계 저장
- **StyleAnalyzer**: 발송 이메일 분석 → 스타일 학습

---

## Redis Stream Job 타입

```go
// 메일
mail.sync       // 초기 동기화
mail.delta_sync // 증분 동기화
mail.send       // 메일 발송
mail.modify     // Provider 상태 동기화

// AI
ai.classify     // 이메일 분류
ai.summarize    // 이메일 요약

// RAG
rag.index       // 단일 인덱싱
rag.batch       // 배치 인덱싱

// 프로필
profile.analyze // 사용자 프로필 분석
```

---

## 실행 방법

```bash
# API 서버
./backend --mode=api

# Worker
./backend --mode=worker

# 둘 다 (개발용)
./backend --mode=all
```

---

## 환경 변수

```env
# Database
DATABASE_URL=postgres://...
MONGODB_URI=mongodb://...
REDIS_URL=redis://...

# Neo4j
NEO4J_URI=bolt://...
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=...

# OAuth
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URL=...

# AI
OPENAI_API_KEY=...

# Server
PORT=8080
JWT_SECRET=...
```

---

## 모듈별 상세 문서

각 모듈의 상세 설계는 해당 디렉토리의 CLAUDE.md 참조:

| 모듈 | 경로 | 핵심 내용 |
|------|------|----------|
| **AI Agent** | `core/agent/CLAUDE.md` | Orchestrator, LLM, RAG, Tools |
| **Service** | `core/service/CLAUDE.md` | 분류 파이프라인, 메일/AI 서비스 |
| **Adapter In** | `adapter/in/CLAUDE.md` | HTTP 핸들러, Worker 프로세서 |
| **Adapter Out** | `adapter/out/CLAUDE.md` | DB, Provider, Messaging |

---

## 구현 상태

### 완료

- [x] Hexagonal Architecture 기반 구조
- [x] Gmail OAuth + 실시간 동기화 (Watch)
- [x] 4단계 분류 파이프라인
- [x] AI Agent (Orchestrator, Proposal)
- [x] RAG 시스템 (pgvector + Neo4j StyleAnalyzer)
- [x] Redis Stream Worker Pool
- [x] 배치 작업 최적화

### 진행 중

- [ ] Outlook 완전 지원
- [ ] SSE 실시간 브로드캐스트 고도화
- [ ] Dead Letter Queue 처리

---

## 코딩 컨벤션

### Import 정렬

```go
import (
    // 1. 표준 라이브러리
    "context"
    "fmt"

    // 2. 외부 패키지
    "github.com/gofiber/fiber/v2"

    // 3. 프로젝트 내부
    "worker_server/core/service"
)
```

### 에러 처리

```go
// 에러 메시지: 소문자, 마침표 없음
errors.New("failed to connect")

// 구조화 로깅
logger.WithFields(map[string]any{
    "email_id": emailID,
}).WithError(err).Error("classification failed")
```

### 네이밍

```go
// 패키지: 소문자, 단수형
package mail

// 변수/함수: MixedCaps
var userCount int
func GetUserByID(id int64) (*User, error)

// 인터페이스: 동사 + er
type Reader interface { Read() }
```
