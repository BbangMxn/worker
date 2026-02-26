# Bridgify Roadmap & Research

> 프로젝트 최적화 방향 + 추후 연구 주제 종합 정리

---

## 1. Current State Analysis (현재 구현 상태)

### 완성된 핵심 기능

| Feature | Status | Notes |
|---------|--------|-------|
| Hexagonal Architecture | Done | Core/Port/Adapter 완전 분리 |
| 7-Stage Classification Pipeline | Done | RFC → Domain → Subject → User Rules → Known DB → (Cache) → LLM |
| Gmail Push Sync | Done | Pub/Sub → Webhook → Delta Sync → SSE |
| AI Agent Orchestrator | Done | Intent Detection → Tool Execution → Response Generation |
| RAG System | Done | pgvector + Neo4j Style Analyzer + Retriever + Ranker (RRF) |
| Worker Pool | Done | Redis Streams, 자동 스케일링 (2~20 goroutines) |
| JWT Auth + OAuth2 | Done | Google + Microsoft, Supabase JWKS |
| SSE Real-time | Done | email.new, email.updated, sync.progress |

### 미완성 / TODO 항목

| Component | Issue | Priority |
|-----------|-------|----------|
| `AIService.ConfirmProposal()` | 항상 에러 반환 (stub) — Proposal 실행 불가 | **Critical** |
| Outlook Sync | OAuth 연결만 가능, 이메일 동기화 미구현 | High |
| Outlook Watch | Push notification 미구현 (폴링 없음) | High |
| AI Reply Worker | `JobAIReply` 프로세서 stub | Medium |
| Contact Extraction | 이메일에서 연락처 자동 추출 미구현 | Medium |
| AI Personalization | `AnalyzeUserProfile()` LLM 호출 미구현 | Medium |
| DLQ Persistence | Dead Letter Queue가 로그만 남김, DB 저장 없음 | Medium |
| Pipeline Stats | `GetStats()` 항상 0 반환 — 계측 없음 | Low |
| Classification Cache (Stage 5) | 예약만 되어있고 미구현 | Low |

---

## 2. Performance Optimization (성능 최적화)

### P0 — 즉시 적용 가능

#### 2.1 Embedding Model 업그레이드

현재 `text-embedding-ada-002` → **`text-embedding-3-small`** 전환

| | ada-002 (현재) | 3-small (전환 후) |
|---|---|---|
| MTEB Score | ~60.9 | ~62.3 |
| Dimension | 1536 | 512 (truncatable) |
| Cost/M tokens | $0.10 | **$0.02** |
| Storage | 100% | **33%** |

**효과**: 임베딩 비용 80% 절감 + 스토리지 67% 절감 + 품질 향상

```sql
-- Migration: HNSW index + halfvec로 전환
DROP INDEX IF EXISTS email_embeddings_embedding_idx;

ALTER TABLE email_embeddings ALTER COLUMN embedding TYPE halfvec(512);

CREATE INDEX email_embeddings_hnsw_idx ON email_embeddings
  USING hnsw (embedding halfvec_cosine_ops)
  WITH (m = 16, ef_construction = 64);
```

#### 2.2 pgvector HNSW 인덱스 전환

IVFFlat → HNSW 전환 시 **동일 recall에서 15.5x 빠른 검색**

- pgvector 0.7.0: scalar/binary quantization, 병렬 인덱스 빌드
- `halfvec` 지원으로 메모리 50% 절약

#### 2.3 LLM Prompt Caching

OpenAI는 1024+ 토큰의 정적 접두사를 자동 캐싱 (50% 할인)

```go
// 정적 시스템 프롬프트를 앞에, 동적 컨텍스트를 뒤에 배치
messages := []Message{
    {Role: "system", Content: longStaticPrompt},  // CACHED (1024+ tokens)
    {Role: "user", Content: dynamicQuery},          // NOT cached
}
```

**효과**: 반복 쿼리에서 LLM 비용 50~90% 절감

### P1 — 1~2주 내 적용

#### 2.4 VectorStore Batch Write 최적화

현재 `StoreBatch`가 N개의 순차 `UPDATE` 실행 → pgx Pipeline 또는 `COPY FROM`으로 교체

```go
// Before: N sequential queries
for _, record := range records {
    s.Store(ctx, record) // 1 UPDATE per record
}

// After: Single batch operation
batch := &pgx.Batch{}
for _, record := range records {
    batch.Queue("UPDATE emails SET embedding = $1 WHERE id = $2", record.Embedding, record.ID)
}
pool.SendBatch(ctx, batch)
```

#### 2.5 Rate Limiter 글로벌 락 제거

현재 `AdvancedRateLimiter`가 모든 요청에 글로벌 write lock → sharded map 또는 Redis-only 방식으로 교체

#### 2.6 Worker Pool 이중 고루틴 제거

`processJob` 내부에서 pool 고루틴 안에 또 고루틴 생성 → context-aware blocking으로 교체

#### 2.7 Structured Outputs 적용

OpenAI Structured Outputs로 분류 JSON 파싱 에러 제거

```go
ResponseFormat: openai.ResponseFormatJSONSchema{
    Schema: classificationSchema,
    Strict: true, // 100% 스키마 준수 보장
}
```

### P2 — 다음 스프린트

#### 2.8 OpenAI Batch API

`email.initial_sync` 시 수천 건 분류 → Batch API로 50% 비용 절감 (24시간 처리 윈도우)

#### 2.9 Metrics 연결

`LatencyTracker`, `DBPoolMonitor`가 구현되어 있지만 **어디에도 연결되지 않음**
→ Prometheus export 엔드포인트 추가로 모든 기존 메트릭 활성화

#### 2.10 CacheMetrics Data Race 수정

`RedisHits`, `RedisMisses` 등이 non-atomic `int64++` → `atomic.AddInt64`로 교체

---

## 3. Feature Roadmap (기능 로드맵)

### Phase 1: Core Completion (핵심 완성)

#### 3.1 Proposal 실행 시스템 구현

AI Agent의 핵심 UX인 Proposal 확인/실행이 stub 상태:

```
사용자: "김팀장에게 미팅 요청 메일 보내줘"
AI: "다음 메일을 보낼까요? [확인] [취소]"
사용자: [확인] 클릭
시스템: ❌ "proposal confirmation not implemented yet"  ← 현재 상태
```

→ Redis 기반 ProposalStore 구현, `executeProposal()` 연결

#### 3.2 Intent Classification 추가

기존 `category + subcategory + priority`에 `intent` 필드 추가:

| Intent | Description | Inbox Action |
|--------|-------------|-------------|
| `action_required` | 행동 필요 | 상단 고정 |
| `reply_expected` | 답장 기대 | 답장 알림 |
| `approval_needed` | 승인 요청 | 승인 버튼 표시 |
| `fyi` | 참고용 | 일반 표시 |
| `scheduling` | 일정 관련 | 캘린더 연동 |
| `payment_request` | 결제 관련 | 금액 하이라이트 |

→ Superhuman의 "Split Inbox" 기능과 동등한 스마트 그룹핑 가능

#### 3.3 Proactive Auto Draft

수신 이메일의 intent가 `reply_expected`이고 priority >= 0.60이면 **자동으로 답장 초안 생성**:

```
이메일 수신 → Classification (intent: reply_expected)
           → RAG Reply Generation (비동기)
           → Draft 저장 + SSE {type: "draft.ready"} 전송
           → 사용자: 초안 확인 → 수정/전송
```

→ Superhuman Auto Draft (2025.10) 기능 대응

### Phase 2: Advanced AI (고급 AI)

#### 3.4 Stage 5 로컬 ML 분류기

현재 비어있는 Stage 5에 **Flan-T5 ONNX 추론** 도입:

```
Stage 0-4 미분류 이메일
    │
    ▼
Stage 5: Flan-T5-large (780M params, ONNX Runtime)
    │   ~150ms 로컬 추론, API 비용 $0
    │   정확도: ~94% (zero-shot)
    │
    ├─ 분류 성공 (confidence > 0.8) → 결과 반환
    └─ 실패 → Stage 6: LLM Fallback (GPT-4o-mini)
```

**효과**: 나머지 20-30% LLM 호출 중 10-15% 추가 절감

#### 3.5 Hybrid Search (BM25 + Semantic + Graph)

현재 검색이 pgvector semantic search만 사용 → 3가지 결합:

```
Query → pgvector ANN Search (semantic)
      + PostgreSQL tsvector FTS (lexical, BM25)
      + Neo4j Contact/Thread Traversal (graph)
      → Reciprocal Rank Fusion (기존 Ranker 활용)
      → Top-K Results
```

PostgreSQL `tsvector`는 추가 인프라 없이 즉시 사용 가능

#### 3.6 Agentic RAG (Reflection Loop)

현재 single-step 검색 → **multi-step adaptive retrieval**:

```go
for step := 0; step < maxReflectionSteps; step++ {
    context := rag.Retrieve(ctx, query)
    response := llm.Generate(ctx, query, context)

    if response.Confidence > threshold {
        return response // 충분히 좋은 답변
    }
    // 부족한 정보를 기반으로 쿼리 재구성
    query = llm.ReformulateQuery(ctx, query, response.MissingInfo)
}
```

### Phase 3: Platform Extension (플랫폼 확장)

#### 3.7 외부 도구 통합 (Tasklet Pattern)

Bridgify의 Tool Registry에 외부 서비스 도구 추가:

| Tool | Action |
|------|--------|
| `create_notion_page` | 이메일 → Notion 페이지 생성 |
| `create_linear_issue` | 이메일 → Linear 이슈 생성 |
| `send_slack_message` | 이메일 요약 → Slack 전송 |
| `create_jira_ticket` | 이메일 → Jira 티켓 생성 |

→ Shortwave Tasklet (2025.10) 기능 대응, 워크플로우 자동화 허브로 진화

#### 3.8 Web Push Notification

SSE는 탭이 열려있을 때만 동작 → **Web Push API (VAPID)** 추가:

```
이메일 수신 → SSE (탭 열린 경우)
           → Web Push (탭 닫힌 경우, 백그라운드 알림)
```

WWDC 2025 Declarative Web Push: Service Worker 없이도 네이티브 알림 가능

#### 3.9 Outlook 완전 지원

현재 stub 상태인 Outlook 동기화 완성:
- Microsoft Graph API 이메일 동기화
- Outlook Subscription API (Push Notification)
- Outlook Calendar 연동

---

## 4. Research Topics (연구 주제)

### 4.1 Model Distillation (모델 증류)

GPT-4o(teacher)로 라벨링한 데이터로 소형 모델(student) fine-tuning:

```
Phase 1: 데이터 수집 — 기존 분류된 이메일 50,000건 (익명화)
Phase 2: Fine-tune — Flan-T5-large 또는 Mistral-7B
Phase 3: Shadow 배포 — GPT-4o-mini와 병렬 비교 1주
Phase 4: 교체 — 정확도 동등 시 자체 호스팅 추론으로 전환
```

**최종 목표**: Stage 6 LLM 비용을 $0으로 (자체 모델 추론)

### 4.2 Federated Learning (연합 학습)

이메일 내용을 서버로 보내지 않고 **디바이스에서 학습, 모델 가중치만 업로드**:

```
사용자 A 디바이스: 로컬 분류 모델 학습 → gradient 전송
사용자 B 디바이스: 로컬 분류 모델 학습 → gradient 전송
서버: gradient 집계 → 글로벌 모델 업데이트 → 배포
```

→ 프라이버시 보존 + 데이터 증가에 따른 모델 개선

### 4.3 Homomorphic Encryption (동형 암호)

**암호화된 상태에서 분류 추론** 수행:

- NYU "Orion" (2025 ASPLOS Best Paper): FHE로 139M params 모델 구동 가능
- 현재 한계: LLM 규모에서는 1000~10000x 느림
- 적용 가능: Flan-T5-small (80M params) → FHE 추론 가능한 수준

**중기 목표 (12-18개월)**: Zama Concrete ML로 FHE 분류 → 엔터프라이즈 프라이버시 프리미엄 기능

### 4.4 Zero-Knowledge Proof (영지식 증명)

GDPR Article 25 (데이터 최소화) 준수를 위한 ZKP 발신자 검증:

```go
// 평문 이메일 주소 대신 commitment 저장
type SenderProfile struct {
    EmailCommitment []byte  // hash(email + salt), NOT plaintext
    SenderScore     float64
}
// 검증: 이메일 주소를 노출하지 않고 "이 발신자를 아는가?" 증명
```

Go ZKP 라이브러리: [gnark](https://github.com/ConsenSys/gnark)

### 4.5 Cross-Encoder Reranking

현재 RRF(Reciprocal Rank Fusion)만 사용 → **학습된 Cross-Encoder**로 reranking:

```
Candidate Documents (Top 20 from RRF)
    │
    ▼
Cross-Encoder (query, document) → relevance score
    │
    ▼
Top 5 Reranked Results
```

옵션: Cohere Rerank API ($0.002/query) 또는 Self-hosted `cross-encoder/ms-marco-MiniLM-L-6-v2`

### 4.6 분류 피드백 루프

현재 사용자가 라벨을 수정해도 분류 정확도에 반영 안 됨:

```
사용자가 "newsletter" → "work"로 재분류
    │
    ▼
ClassificationFeedback 수집
    │
    ├─ User Rules 자동 업데이트 (단기)
    ├─ SenderProfile 점수 조정 (단기)
    └─ Fine-tuning 데이터로 축적 (장기, 4.1 연계)
```

---

## 5. Priority Matrix

| Priority | Item | Effort | Impact |
|----------|------|--------|--------|
| **P0** | Embedding 3-small + HNSW 전환 | 3시간 | 임베딩 비용 -80%, 검색 15x 빠름 |
| **P0** | LLM Prompt Cache 구조 최적화 | 1일 | LLM 비용 -50~90% |
| **P0** | Structured Outputs 적용 | 2시간 | JSON 파싱 에러 제거 |
| **P0** | Proposal 실행 시스템 구현 | 3일 | AI Agent 핵심 기능 완성 |
| **P1** | Intent Classification 추가 | 3일 | 스마트 인박스 그룹핑 |
| **P1** | Batch API (initial_sync) | 2일 | 초기 동기화 LLM 비용 -50% |
| **P1** | VectorStore Batch 최적화 | 1일 | RAG 인덱싱 처리량 향상 |
| **P1** | Rate Limiter 글로벌 락 제거 | 1일 | API 처리량 향상 |
| **P2** | Hybrid Search (BM25 + Semantic) | 1주 | 검색 recall 대폭 향상 |
| **P2** | Proactive Auto Draft | 3일 | Superhuman 기능 대응 |
| **P2** | Web Push Notification | 2일 | 백그라운드 사용자 참여 |
| **P2** | Metrics/Prometheus 연결 | 2일 | 운영 가시성 확보 |
| **P3** | Stage 5 로컬 ML (Flan-T5 ONNX) | 1주 | LLM 호출 -10~15% 추가 절감 |
| **P3** | 외부 도구 통합 (Notion, Slack) | 2주 | 플랫폼 확장 |
| **P3** | Agentic RAG Reflection | 1주 | 복잡 쿼리 품질 향상 |
| **P3** | GDPR 데이터 최소화 | 1일 | 컴플라이언스 |
| **P4** | Model Distillation Pipeline | 지속 | 장기 비용 $0 분류 |
| **P4** | Federated Learning | 3-6개월 | 프라이버시 보존 학습 |
| **P4** | FHE Classification | 6개월+ | 엔터프라이즈 프리미엄 |

---

## 6. Known Technical Debt (기술 부채)

| Issue | Location | Risk |
|-------|----------|------|
| 이중 Classification Pipeline | `Pipeline` + `ScorePipeline` 병존 | 유지보수 혼란 |
| `logSuspiciousRequest` no-op | `worker_security.go:146` | 보안 이벤트 미감지 |
| Webhook 인증 없음 | `/webhook/*` 경로 auth skip | 가짜 Push 주입 가능 |
| SSE 토큰 query string 노출 | `worker_auth.go:268` | 브라우저 히스토리 유출 |
| `SyncRetryScheduler` 무제한 고루틴 | `worker_sync_retry.go:93` | 대규모 백로그 시 OOM |
| Mixed Logger 사용 | `log.Printf` 71곳 vs `zerolog` | 글로벌 뮤텍스 병목 |
| Provider 하드코딩 "google" | `worker_email_handler.go:735` | Outlook 사용자 라우팅 오류 |
| Semantic Cache 미연결 | `ScoreClassifierInput.Embedding` 미설정 | Stage 3 영구 비활성 |
| Test Coverage 부족 | 6개 테스트 파일, 주로 structural | 리그레션 위험 |

---

## References

- [Zero-Shot Email Classification — arXiv 2405.15936](https://arxiv.org/abs/2405.15936)
- [RAG Optimization 2025 — SynthiMind](https://synthimind.net/blog/rag-optimization-strategies-2025/)
- [GraphRAG + Neo4j — Neo4j Blog](https://neo4j.com/blog/developer/graphrag-and-agentic-architecture-with-neoconverse/)
- [pgvector HNSW 150x Speedup — Jonathan Katz](https://jkatz05.com/post/postgres/pgvector-performance-150x-speedup/)
- [Embedding Models 2026 — Elephas](https://elephas.app/blog/best-embedding-models)
- [Prompt Caching 2025 — PromptBuilder](https://promptbuilder.cc/blog/prompt-caching-token-economics-2025)
- [Structured Outputs — OpenAI](https://openai.com/index/introducing-structured-outputs-in-the-api/)
- [Superhuman vs Shortwave 2025](https://blog.superhuman.com/shortwave-email/)
- [Declarative Web Push — WebKit](https://webkit.org/blog/16535/meet-declarative-web-push/)
- [FHE for ML — NYU Orion](https://engineering.nyu.edu/news/encryption-breakthrough-lays-groundwork-privacy-preserving-ai-models)
- [ZKP + GDPR — INATBA](https://inatba.org/wp-content/uploads/2025/08/Leveraging-ZKP-for-GDPR-Compliance-in-Blockchain-Projects.pdf)
