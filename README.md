# Bridgify (Worker)

> AI-powered work automation platform — Email, Calendar, Contacts를 지능적으로 통합 관리하는 Superhuman 스타일 워크스페이스

## Overview

Bridgify는 Gmail/Outlook 이메일, Google Calendar, 연락처를 하나의 통합 인터페이스에서 관리하며, AI Agent가 자연어 명령을 이해하고 사용자를 대신해 작업을 수행하는 플랫폼입니다.

### Core Value

| Feature | Description |
|---------|-------------|
| **AI Agent** | 자연어로 이메일 전송, 캘린더 생성, 검색 등 수행 — Proposal 기반 안전 실행 |
| **7-Stage Classification** | RFC 헤더 → 도메인 → 패턴 → 사용자 규칙 → LLM 순서로 분류, LLM API 비용 ~75% 절감 |
| **Real-time Sync** | Gmail Pub/Sub Push → historyId 델타 동기화 → SSE 브로드캐스트 |
| **RAG Personalization** | 발신 이메일 분석으로 사용자 문체 학습, 개인화된 답장 생성 |
| **Multi-Provider** | Gmail + Outlook 동시 지원, 통합 메일함 |

---

## Tech Stack

### Backend (`worker_server/`)

| Layer | Technology |
|-------|-----------|
| Language | **Go 1.24** |
| HTTP Framework | Fiber v2 (fasthttp 기반 고성능) |
| Architecture | Hexagonal (Ports & Adapters) |
| Primary DB | PostgreSQL (Supabase, pgxpool + sqlx) |
| Document DB | MongoDB (이메일 본문, gzip 압축, 30-day TTL) |
| Graph DB | Neo4j (사용자 문체, 연락처 관계 그래프) |
| Vector DB | pgvector (1536-dim OpenAI embeddings) |
| Queue / Cache | Redis Streams + Redis Cache |
| AI / LLM | OpenAI GPT-4o-mini (분류, 요약, 답장, Function Calling) |
| Embeddings | OpenAI text-embedding-ada-002 |
| Auth | JWT (Supabase JWKS), OAuth2 (Google, Microsoft) |
| Deployment | Docker (multi-stage alpine) → Railway |

### Frontend (`worker_client/`)

| Layer | Technology |
|-------|-----------|
| Language | **TypeScript** |
| Framework | Next.js 14 (App Router) |
| Styling | Tailwind CSS 3.4 |
| Auth | Supabase SSR (@supabase/ssr) |
| Animation | Framer Motion |
| Icons | Lucide React |
| Utilities | clsx + tailwind-merge |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Frontend (Next.js 14)                     │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │   Mail   │ │ Calendar │ │ Contacts │ │ AI Chat  │           │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘           │
│       └─────────────┴─────────────┴─────────────┘               │
│                          REST API + SSE                          │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│                     Backend (Go / Fiber v2)                      │
│                                                                  │
│  ┌─── Adapter (In) ──────────────────────────────────────────┐  │
│  │  HTTP Handlers (24 files)  │  Worker Processors (13 files) │  │
│  └─────────────┬──────────────┴──────────────┬───────────────┘  │
│                │                              │                  │
│  ┌─── Core ───▼──────────────────────────────▼───────────────┐  │
│  │  ┌──────────┐  ┌────────────────┐  ┌──────────────────┐   │  │
│  │  │  Domain   │  │   Services     │  │   AI Agent       │   │  │
│  │  │ (Entities)│  │ (Business      │  │ ┌─────────────┐  │   │  │
│  │  │          │  │  Logic)        │  │ │ Orchestrator│  │   │  │
│  │  │          │  │                │  │ │ LLM Client  │  │   │  │
│  │  │          │  │ • Email        │  │ │ RAG System  │  │   │  │
│  │  │          │  │ • Calendar     │  │ │ Tool Registry│ │   │  │
│  │  │          │  │ • Classification│ │ │ Proposals   │  │   │  │
│  │  │          │  │ • Search       │  │ └─────────────┘  │   │  │
│  │  │          │  │ • Notification │  │                   │   │  │
│  │  └──────────┘  └────────────────┘  └──────────────────┘   │  │
│  │                     Ports (In/Out Interfaces)              │  │
│  └────────────────────────────┬───────────────────────────────┘  │
│                               │                                  │
│  ┌─── Adapter (Out) ─────────▼───────────────────────────────┐  │
│  │  PostgreSQL │ MongoDB │ Neo4j │ Redis │ Gmail/Outlook API  │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

### Hexagonal Architecture

```
Port (Interface)                    Adapter (Implementation)
─────────────────                   ─────────────────────────
EmailRepositoryPort      ────→     PostgreSQL Adapter
EmailProviderPort        ────→     Gmail API / Outlook API Adapter
EmailBodyRepositoryPort  ────→     MongoDB Adapter
ClassificationPort       ────→     Neo4j Graph Adapter
VectorStorePort          ────→     pgvector Adapter
MessageQueuePort         ────→     Redis Stream Adapter
```

---

## Directory Structure

```
worker/
├── worker_client/                    # Frontend (Next.js 14)
│   ├── src/
│   │   ├── app/                      # Next.js App Router
│   │   │   ├── (auth)/               # 로그인 / 회원가입
│   │   │   ├── (workspace)/          # 메인 워크스페이스
│   │   │   │   ├── mail/             # 이메일 뷰
│   │   │   │   ├── calendar/         # 캘린더 뷰
│   │   │   │   ├── contacts/         # 연락처 뷰
│   │   │   │   ├── documents/        # 문서 뷰
│   │   │   │   └── image/            # 이미지 뷰
│   │   │   └── auth/callback/        # OAuth 콜백
│   │   ├── entities/                 # 도메인 엔티티 (타입, 목업, UI)
│   │   ├── lib/supabase/             # Supabase 클라이언트
│   │   ├── shared/                   # 공통 컴포넌트, 훅, 유틸
│   │   └── widgets/                  # 복합 UI 위젯 (FSD 패턴)
│   │       ├── command-palette/      # Cmd+K 커맨드 팔레트
│   │       ├── compose/              # 이메일 작성 모달
│   │       ├── email-list/           # 이메일 목록
│   │       ├── email-detail/         # 이메일 상세
│   │       ├── calendar-view/        # 일/주 캘린더 뷰
│   │       ├── sidebar/              # 네비게이션 사이드바
│   │       └── split-view/           # 분할 뷰 레이아웃
│   └── package.json
│
└── worker_server/                    # Backend (Go 1.24)
    ├── main.go                       # 엔트리포인트 (--mode: api/worker/all)
    ├── Dockerfile                    # Multi-stage Docker 빌드
    ├── railway.toml                  # Railway 배포 설정
    ├── config/                       # 환경변수 기반 설정
    ├── core/                         # 비즈니스 로직 (Hexagonal Core)
    │   ├── domain/                   # 도메인 엔티티 (20+ 파일)
    │   ├── port/                     # 인터페이스
    │   │   ├── in/                   # UseCase 인터페이스 (6)
    │   │   └── out/                  # Repository 인터페이스 (18+)
    │   ├── service/                  # 서비스 구현체
    │   │   ├── ai/                   # AI 서비스 (개인화, 최적화)
    │   │   ├── classification/       # 7-Stage 분류 파이프라인
    │   │   │   └── rfc/              # 40+ RFC 기반 분류기
    │   │   ├── email/                # 이메일 서비스, 동기화
    │   │   ├── search/               # 검색 (7개 모듈)
    │   │   └── notification/         # 알림 서비스, 웹훅
    │   └── agent/                    # AI Agent 시스템
    │       ├── worker_orchestrator.go # 중앙 AI 브레인
    │       ├── llm/                  # OpenAI 클라이언트
    │       ├── rag/                  # RAG 시스템 (스타일 분석, 벡터)
    │       ├── tools/                # Function Calling 도구
    │       └── session/              # 세션, Proposal 관리
    ├── adapter/                      # 어댑터 (Hexagonal 외부)
    │   ├── in/
    │   │   ├── http/                 # HTTP 핸들러 (24 파일)
    │   │   └── worker/               # 워커 프로세서 (13 파일)
    │   └── out/
    │       ├── persistence/          # PostgreSQL 어댑터 (30+)
    │       ├── mongodb/              # MongoDB 어댑터
    │       ├── graph/                # Neo4j 어댑터
    │       ├── messaging/            # Redis Stream 어댑터
    │       ├── provider/             # Gmail/Outlook API 어댑터
    │       └── realtime/             # SSE 어댑터
    ├── internal/
    │   ├── bootstrap/                # DI 컨테이너, 앱 부트스트랩
    │   └── stream/                   # Redis Stream 인프라
    ├── infra/                        # 미들웨어, DB 초기화
    ├── pkg/                          # 로거, Rate Limit, Metrics
    └── migrations/                   # SQL 마이그레이션 (31개)
```

---

## Key Features

### 1. AI Agent & Orchestrator

자연어 명령을 이해하고 실행하는 AI 에이전트 시스템입니다.

```
사용자: "오늘 오후 3시에 김민수님과 미팅 잡아줘"

┌─────────────┐     ┌─────────────┐     ┌──────────────┐
│ Intent       │     │ Tool        │     │ Proposal     │
│ Detection   │────→│ Execution   │────→│ Generation   │
│ (LLM)       │     │ (Calendar)  │     │ (확인 대기)   │
└─────────────┘     └─────────────┘     └──────┬───────┘
                                                │
                                        사용자 확인/거부
                                                │
                                        ┌───────▼───────┐
                                        │  실행 or 취소  │
                                        └───────────────┘
```

**Proposal Safety Pattern**: 이메일 전송, 삭제, 캘린더 생성 등 파괴적 작업은 즉시 실행하지 않고, `ActionProposal`을 생성하여 사용자 확인 후 실행합니다 (10분 만료).

**지원 도구:**
- **Email**: 목록 조회, 읽기, 검색, 전송, 답장, 전달, 삭제, 보관, 별표, 번역, 요약
- **Calendar**: 일정 조회, 생성, 수정, 삭제, 빈 시간 검색
- **Contact**: 연락처 조회, 생성, 수정

### 2. Email Classification Pipeline

LLM API 비용을 ~75% 절감하는 7-Stage 분류 파이프라인입니다.

```
수신 이메일
    │
    ├─ Stage 0: RFC Header 분석 ──────── ~50-60% 해결
    │   (List-Unsubscribe, Precedence, Auto-Submitted,
    │    ESP 감지: Mailchimp, SendGrid, SES 등)
    │
    ├─ Stage 1: Domain Score ─────────── ~10% 추가 해결
    │   (github.com, stripe.com 등 알려진 서비스 도메인)
    │
    ├─ Stage 2: Subject Pattern ──────── ~5% 추가 해결
    │   (CI/CD 패턴, 금융 알림, 배송 추적 등)
    │
    ├─ Stage 3: User Rules ───────────── ~10% 추가 해결
    │   (사용자 정의 중요 도메인, 키워드, 무시 발신자)
    │
    ├─ Stage 4: Known Domain DB ──────── ~5% 추가 해결
    │   (SenderProfile, KnownDomain 데이터베이스)
    │
    ├─ Stage 5: Cache (예약) ─────────── (향후 구현)
    │
    └─ Stage 6: LLM Fallback ─────────── ~20-30% (나머지)
        (OpenAI GPT-4o-mini, 자연어 규칙 포함)
```

**RFC 분류기 (40+):**
GitHub, GitLab, Jira, Slack, Notion, Linear, PagerDuty, Datadog, Sentry, Stripe, AWS, Google Cloud, Vercel, Netlify 등 개발자 서비스별 전용 분류기

**분류 카테고리:**

| Category | SubCategories |
|----------|---------------|
| `work` | meeting, project, client, team, hr |
| `personal` | family, friend, health, finance |
| `newsletter` | tech, business, lifestyle |
| `marketing` | promotion, sale, product_launch |
| `notification` | social, system, security, shipping |
| `developer` | ci_cd, pr_review, issue, deploy, alert |
| `finance` | invoice, payment, statement, tax |
| `travel` | booking, itinerary, loyalty |
| `other` | - |

### 3. Real-time Email Sync

Gmail Pub/Sub 기반 실시간 동기화 시스템입니다.

```
Gmail 서버                    Worker 서버                     클라이언트
    │                             │                              │
    │   Pub/Sub Push 알림          │                              │
    │──────────────────────────→  │                              │
    │                   Webhook 수신 → Delta Sync 시작            │
    │                             │                              │
    │   history.list 요청          │                              │
    │←────────────────────────── │                              │
    │   변경된 메시지 목록          │                              │
    │──────────────────────────→  │                              │
    │                    DB 업데이트 + AI 분류 (비동기)            │
    │                             │       SSE Event              │
    │                             │──────────────────────────→   │
    │                             │   {type: "email.new"}        │
    │                             │                        UI 업데이트
```

- Gmail Watch 7일 만료 전 자동 재등록 스케줄러 내장
- 동기화 실패 시 자동 재시도 스케줄러
- 서버 재시작 시 동기화 갭 감지 및 보정

### 4. RAG Personalization System

사용자의 발신 이메일을 분석하여 문체를 학습하고, 개인화된 답장을 생성합니다.

```
발신 이메일 → Style Analyzer → Neo4j (Writing Style)
                                    │
                         ┌──────────┼──────────┐
                         │          │          │
                   Formality   Sentence    Tone
                   Score       Length Avg   Preference
                         │          │          │
                   Emoji Freq  Greetings  Closings

답장 생성 시:
    pgvector (유사 이메일 검색)
         +
    Neo4j (수신자 관계, 문체 패턴)
         +
    LLM (Context + Style → Personalized Reply)
```

### 5. Worker Pool & Job Queue

Redis Streams 기반 비동기 작업 처리 시스템입니다.

| Job Type | Description |
|----------|-------------|
| `email.initial_sync` | 최초 이메일 전체 동기화 |
| `email.delta_sync` | historyId 기반 변경분 동기화 |
| `email.send` | 이메일 전송 |
| `email.modify` | 라벨 변경, 읽음 처리 등 |
| `email.watch_setup` | Gmail Watch 등록 |
| `ai.classify` | AI 이메일 분류 |
| `ai.summarize` | AI 이메일 요약 |
| `rag.index` | 이메일 임베딩 및 인덱싱 |
| `calendar.sync` | 캘린더 동기화 |

- Worker Pool: 최소 2개 → 최대 20개 고루틴, 부하에 따라 자동 스케일링
- Consumer Group: at-least-once 메시지 처리 보장
- Pending 자동 재처리: 실패 메시지 자동 재시도 (최대 3회)

---

## Database Schema

### PostgreSQL (Primary — 31 migrations)

| Table | Description |
|-------|-------------|
| `users` | 사용자 계정 (Supabase Auth 연동) |
| `oauth_connections` | OAuth 연결 정보 (Google, Microsoft) |
| `emails` | 이메일 메타데이터 + AI 분류 결과 |
| `labels` | 이메일 라벨 (Gmail/Outlook 매핑) |
| `calendar_events` | 캘린더 이벤트 |
| `contacts` | 연락처 |
| `settings` | 사용자 설정 |
| `classification_rules` | 사용자 정의 분류 규칙 |
| `email_embeddings` | pgvector 이메일 임베딩 (1536-dim) |
| `sender_profiles` | 발신자 프로필 |
| `sync_states` | Gmail historyId 동기화 상태 |
| `notifications` | 푸시 알림 |
| `email_attachments` | 첨부파일 메타데이터 |
| `email_templates` | 이메일 템플릿 |
| `todos` | 할 일 목록 |

**Priority Score (Eisenhower Matrix 기반):**

| Score | Level | Description |
|-------|-------|-------------|
| 0.80 ~ 1.00 | Urgent | 즉각 조치 필요 |
| 0.60 ~ 0.79 | High | 중요, 빠른 대응 필요 |
| 0.40 ~ 0.59 | Normal | 관련성 있음 |
| 0.20 ~ 0.39 | Low | 지연 가능 |
| 0.00 ~ 0.19 | Lowest | 배경 노이즈 |

### MongoDB (Document Store)

| Collection | Description |
|-----------|-------------|
| `email_bodies` | 이메일 본문 (text/html), gzip 압축, 30-day TTL |

### Neo4j (Graph DB)

```
(:User)
  ├──[:HAS_WRITING_STYLE]──→(:WritingStyle)
  ├──[:HAS_TONE_PREF]──→(:TonePreference)
  ├──[:HAS_PATTERN]──→(:CommunicationPattern)
  ├──[:USES_PHRASE]──→(:Phrase)
  └──[:COMMUNICATES_WITH]──→(:Contact)
```

### Redis

| Key Pattern | Description |
|------------|-------------|
| `stream:jobs` | 작업 큐 (Redis Stream) |
| `cache:emails:{userId}:*` | L2 이메일 목록 캐시 (1분 TTL) |
| `cache:session:{sessionId}` | AI 세션 캐시 (24시간 TTL) |

---

## API Reference

모든 인증 엔드포인트는 `Authorization: Bearer <supabase_jwt_token>` 헤더가 필요합니다.

### OAuth

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/oauth/connect/:provider` | OAuth 연결 시작 |
| `GET` | `/api/v1/oauth/:provider/callback` | OAuth 콜백 처리 |
| `DELETE` | `/api/v1/oauth/disconnect/:provider` | OAuth 연결 해제 |
| `GET` | `/api/v1/oauth/connections` | 연결된 계정 목록 |

### Email

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/mail` | 이메일 목록 (필터 지원) |
| `GET` | `/api/v1/mail/unified` | 통합 메일함 (커서 페이지네이션) |
| `GET` | `/api/v1/mail/search` | Gmail API 직접 검색 |
| `GET` | `/api/v1/mail/:id` | 이메일 상세 조회 |
| `GET` | `/api/v1/mail/:id/body` | 이메일 본문 (MongoDB) |
| `POST` | `/api/v1/mail` | 이메일 전송 |
| `POST` | `/api/v1/mail/:id/reply` | 답장 |
| `POST` | `/api/v1/mail/:id/forward` | 전달 |
| `POST` | `/api/v1/mail/read` | 읽음 처리 (batch) |
| `POST` | `/api/v1/mail/archive` | 보관 (batch) |
| `POST` | `/api/v1/mail/trash` | 휴지통 (batch) |
| `POST` | `/api/v1/mail/delete` | 영구 삭제 (batch) |
| `POST` | `/api/v1/mail/move` | 폴더 이동 (batch) |
| `POST` | `/api/v1/mail/snooze` | 다시 알림 |
| `POST` | `/api/v1/mail/sync` | 수동 동기화 트리거 |

### AI Agent

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/ai/chat` | AI 대화 (동기) |
| `GET` | `/api/v1/ai/chat/stream` | AI 대화 (SSE 스트리밍) |
| `POST` | `/api/v1/ai/classify/:id` | 이메일 분류 |
| `POST` | `/api/v1/ai/classify/batch` | 배치 분류 |
| `POST` | `/api/v1/ai/summarize/:id` | 이메일 요약 |
| `POST` | `/api/v1/ai/reply/:id` | AI 답장 생성 |
| `POST` | `/api/v1/ai/autocomplete` | 작성 자동완성 |
| `GET` | `/api/v1/ai/proposals` | 대기 중 Proposal 목록 |
| `POST` | `/api/v1/ai/proposals/:id/confirm` | Proposal 승인 실행 |
| `POST` | `/api/v1/ai/proposals/:id/reject` | Proposal 거부 |

### Calendar

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/calendar/events` | 이벤트 목록 |
| `POST` | `/api/v1/calendar/events` | 이벤트 생성 |
| `PUT` | `/api/v1/calendar/events/:id` | 이벤트 수정 |
| `DELETE` | `/api/v1/calendar/events/:id` | 이벤트 삭제 |

### Real-time & Webhooks

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/sse/events` | SSE 이벤트 스트림 |
| `POST` | `/webhook/gmail` | Gmail Pub/Sub 수신 (no auth) |
| `POST` | `/webhook/outlook` | Outlook 수신 (no auth) |
| `GET` | `/health` | 헬스체크 |

**SSE Event Types:** `email.new`, `email.updated`, `sync.progress`, `sync.complete`

---

## Getting Started

### Prerequisites

- Go 1.24+
- Node.js 18+ / npm
- PostgreSQL 15+ (with pgvector extension)
- MongoDB 7+
- Redis 7+
- Neo4j 5+
- Google Cloud Console 프로젝트 (Gmail API, Calendar API, Pub/Sub)
- OpenAI API Key

### Backend Setup

```bash
cd worker_server

# 환경변수 설정
cp .env.example .env
# .env 파일을 편집하여 DB URL, API 키 등 입력

# 의존성 설치
go mod download

# 마이그레이션 실행
# migrations/ 폴더의 SQL 파일을 순서대로 PostgreSQL에 실행

# 서버 시작
go run . --mode=all       # API + Worker 모두 실행
go run . --mode=api       # API 서버만
go run . --mode=worker    # Worker만
```

### Frontend Setup

```bash
cd worker_client

npm install

# .env.local 파일 편집 (NEXT_PUBLIC_API_URL, Supabase 키)

npm run dev
```

### Docker

```bash
cd worker_server
docker build -t bridgify-server .
docker run -p 8080:8080 --env-file .env bridgify-server
```

---

## Environment Variables

```bash
# Server
PORT=8080
ENV=development

# PostgreSQL (Supabase)
DATABASE_URL=postgresql://...
DIRECT_URL=postgresql://...

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
```

전체 환경변수 목록: `.env.example` 참조

---

## Design Decisions

### Why Hexagonal Architecture?

- **테스트 용이성**: Port 인터페이스를 통해 모든 외부 의존성을 Mock으로 교체 가능
- **기술 교체 유연성**: PostgreSQL → CockroachDB 전환 시 Adapter만 교체
- **관심사 분리**: Domain은 외부 기술에 대한 의존성 zero

### Why 7-Stage Classification?

- **비용 효율**: 이메일의 ~70-80%는 RFC 헤더와 도메인 분석만으로 분류 가능
- **속도**: LLM 호출 없이 즉시 분류 → 사용자 경험 향상
- **정확도**: 각 단계가 특화된 도메인 지식 활용

### Why MongoDB for Email Bodies?

- **대용량 텍스트**: HTML 이메일 본문은 수백 KB에 달할 수 있음
- **TTL**: 30일 자동 만료로 스토리지 비용 관리
- **gzip 압축**: 저장 공간 50-70% 절약

### Why Neo4j for Personalization?

- **관계 중심 데이터**: "누구와 어떤 톤으로 소통하는가"는 그래프 모델에 최적
- **Traversal 성능**: 2-3 hop 관계 탐색이 RDBMS 대비 수백 배 빠름
- **EMA 기반 트렌드**: 시간에 따른 관계 변화를 Exponential Moving Average로 추적

### Why Redis Streams?

- **At-least-once 보장**: Consumer Group으로 메시지 유실 방지
- **자동 스케일링**: 여러 Worker가 동일 Stream을 분산 소비
- **Pending 재처리**: 처리 실패 메시지 자동 재시도

---

## Deployment

### Railway (Production)

```toml
[build]
builder = "dockerfile"
dockerfilePath = "worker_server/Dockerfile"

[deploy]
healthcheckPath = "/health"
restartPolicyType = "on_failure"
restartPolicyMaxRetries = 5
```

### Infrastructure

```
Railway (Go Server) ──→ Supabase (PostgreSQL + Auth)
        │
        ├──→ MongoDB Atlas
        ├──→ Redis (Upstash / Railway)
        ├──→ Neo4j (Aura / Self-hosted)
        └──→ Google Cloud (Pub/Sub, Gmail API)
```

---

## License

This project is proprietary software. All rights reserved.
