# Worker

> 이메일, 캘린더, 연락처를 하나의 워크스페이스로 묶고 AI Agent까지 연결해본 업무 자동화 플랫폼 실험

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-14-000000?style=flat-square&logo=nextdotjs&logoColor=white)](https://nextjs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?style=flat-square&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![MongoDB](https://img.shields.io/badge/MongoDB-7-47A248?style=flat-square&logo=mongodb&logoColor=white)](https://www.mongodb.com/)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=flat-square&logo=redis&logoColor=white)](https://redis.io/)
[![OpenAI](https://img.shields.io/badge/OpenAI-GPT--4o--mini-412991?style=flat-square&logo=openai&logoColor=white)](https://openai.com/)

Worker는 Gmail/Outlook 이메일, Google Calendar, 연락처를 하나의 인터페이스에서 다루고, AI Agent가 자연어 명령을 이해해 사용자를 대신해 작업을 제안하거나 실행하는 워크스페이스를 목표로 만든 프로젝트입니다.  
단순 메일 클라이언트가 아니라, `업무 도구를 AI와 함께 다시 설계해보는 실험`에 가깝습니다.

## Portfolio Summary

- 목표: 이메일, 캘린더, 연락처를 통합하고 AI Agent 기반 자동화까지 연결
- 역할: 1인 개발
- 담당: 제품 방향 설정, 프론트엔드, 백엔드, AI 파이프라인, 데이터 설계, 문서화
- 현재 상태: 동작 검증 완료, 구조 재설계 진행 중

이 프로젝트는 `기능 구현`도 있지만, 그보다 `과도한 엔지니어링을 실제로 겪고 다시 단순화하려는 판단`까지 포함해 보여주는 프로젝트입니다.

## Why This Project Exists

업무 도구는 대개 이메일, 캘린더, 연락처가 서로 분리되어 있고, 사용자는 반복적인 분류와 정리를 계속 직접 해야 합니다.  
그래서 이 프로젝트에서는 다음 질문을 직접 검증해보고 싶었습니다.

- AI Agent가 실제 업무 흐름 안에서 유용하게 동작할 수 있는가
- 이메일 분류와 요약을 어디까지 자동화할 수 있는가
- 개인화된 답장 생성이 실제로 가능한가
- 단순 기능 추가가 아니라, 업무 워크스페이스 자체를 다시 설계할 수 있는가

즉 Worker는 `메일 클라이언트`를 만드는 프로젝트가 아니라, `AI 기반 업무 자동화 인터페이스`를 실험하는 프로젝트입니다.

## What I Built

### 1. Unified Workspace

- 이메일 뷰
- 캘린더 뷰
- 연락처 뷰
- AI 채팅 / 명령 인터페이스
- REST API + SSE 기반 실시간 연결

### 2. AI Agent Workflow

- 자연어 명령 해석
- 도구 선택 및 실행 계획 수립
- Proposal 기반 안전 실행
- 이메일 / 일정 / 연락처 작업 연동

즉 AI가 바로 파괴적 작업을 실행하는 것이 아니라, 먼저 `Action Proposal`을 만들고 사용자의 확인을 받은 뒤 실행하는 구조를 택했습니다.

### 3. Email Classification Pipeline

- RFC 헤더 분석
- 도메인 기반 분류
- 제목 패턴 분류
- 사용자 규칙 적용
- Known domain DB
- 최종 LLM fallback

이 파이프라인을 통해 이메일 분류에서 불필요한 LLM 호출을 줄이는 구조를 만들었습니다.

### 4. Real-time Sync

- Gmail Pub/Sub 기반 델타 동기화
- historyId 기반 변경분 수집
- SSE 브로드캐스트
- 서버 재시작 이후 갭 감지 및 보정

### 5. RAG Personalization

- 사용자가 보낸 이메일을 분석해 문체를 학습
- 유사 이메일 검색
- 개인화된 답장 생성

## Key Technical Decisions

- `Go + Fiber + Hexagonal Architecture`
  - 실시간 동기화와 워커 처리, 다수의 외부 연동을 감당하기 위해 Go를 선택했고, 도메인/어댑터 분리를 위해 헥사고날 구조를 적용했습니다.
- `Polyglot Persistence`
  - PostgreSQL, MongoDB, Redis, Neo4j를 각 역할에 맞게 분리했지만, 실제 운영 관점에서는 과도한 복잡성이 생긴다는 점도 같이 확인했습니다.
- `Proposal Safety Pattern`
  - AI Agent가 이메일 전송이나 일정 생성 같은 작업을 곧바로 실행하지 않고, 사용자 확인을 거치도록 설계했습니다.
- `7-Stage Classification`
  - 모든 이메일을 곧바로 LLM에 던지지 않고, RFC 헤더와 규칙 기반 분류를 먼저 수행해 비용을 낮추는 방향을 선택했습니다.
- `RAG Personalization`
  - 단순 분류/요약을 넘어서, 사용자의 실제 발신 이메일을 바탕으로 답장 문체를 학습하는 구조를 실험했습니다.

## Results and Current State

현재까지 확인한 결과는 이렇습니다.

- 이메일 자동 번역, 분류, 요약: 동작 확인
- 7-Stage Classification: LLM 호출 약 `75%` 절감
- Gmail 실시간 동기화: 동작 확인
- RAG 기반 개인화 답장: 구조 검증 완료

다만 이 프로젝트는 `성공한 기능`만 있는 게 아니라, 실제로 써보면서 발견한 구조 문제도 함께 남아 있습니다.

- `Neo4j`
  - 관계 탐색보다 문체 데이터 쓰기가 더 많아, Graph DB의 장점이 크지 않았습니다.
- `4개 DB 운영`
  - PostgreSQL + MongoDB + Neo4j + Redis는 개인 프로젝트 기준으로 복잡도가 높았습니다.
- `Proposal execution`
  - Proposal 생성은 되지만 실제 실행 연결은 일부 미완성입니다.
- `Outlook sync`
  - OAuth 연결은 되지만 실제 동기화는 아직 보완이 필요합니다.

즉 Worker는 단순히 “기능이 많다”는 프로젝트가 아니라, `어떤 설계가 과했고 무엇을 줄여야 하는지`까지 확인한 프로젝트입니다.

## Current Retrospective

이 프로젝트에서 중요한 건 기능 목록보다 아래 판단들입니다.

- Neo4j를 유지할 이유가 충분한가
- 4개 DB 구조가 실제로 필요한가
- RFC 분류 파이프라인 40+개를 계속 유지할 가치가 있는가
- LLM 비용 절감보다 유지보수 복잡도가 더 큰 문제는 아닌가

현재는 이런 문제를 바탕으로 더 단순한 구조로 다시 가져가는 방향을 검토하고 있습니다.

- Neo4j 제거
- PostgreSQL + MongoDB + Redis 중심 재정리
- Proposal 실행 플로우 완성
- Outlook 동기화 보완

## Repository Layout

```text
worker/
├── worker_client/   # Next.js 14 frontend
├── worker_server/   # Go backend
└── docs/            # 로드맵 및 설계 문서
```

## Stack

### Backend

- Go 1.24
- Fiber v2
- Hexagonal Architecture
- PostgreSQL
- MongoDB
- Redis Streams / Cache
- Neo4j
- OpenAI GPT-4o-mini
- pgvector

### Frontend

- Next.js 14
- TypeScript
- Tailwind CSS
- Supabase SSR
- Framer Motion

## What This Project Shows

- AI Agent 제품을 실제로 설계하고 구현한 경험
- 복잡한 백엔드 구조를 헥사고날로 정리한 경험
- 실시간 동기화, 워커 큐, 외부 API 연동을 묶은 경험
- RAG와 개인화 답장 생성까지 연결한 경험
- 구현 후 과도한 설계를 스스로 비판하고 재설계 방향을 잡는 능력

## Related Docs

- [Roadmap](./docs/ROADMAP.md)
- `worker_server/` 내부 설계 문서
- `worker_client/` 워크스페이스 UI 구현
