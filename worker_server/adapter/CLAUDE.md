# Adapter Layer

## 목적

외부 세계와 Core의 연결. Port 인터페이스 구현.

## 설계 방향

### In (Inbound Adapter)
- 외부 요청 → Core 호출
- HTTP Handler, Worker Processor, Webhook

### Out (Outbound Adapter)
- Core → 외부 시스템
- Provider (Gmail, Outlook), Persistence (DB), Messaging (Redis)

```
adapter/
├── in/
│   ├── http/         # REST API Handler
│   └── worker/       # Background Worker
│
└── out/
    ├── provider/     # 외부 API (Gmail, Outlook)
    ├── persistence/  # PostgreSQL
    ├── mongodb/      # MongoDB
    ├── graph/        # Neo4j
    ├── messaging/    # Redis Stream
    └── realtime/     # SSE (TODO)
```

## 구현 완료

### In
- [x] HTTP Handler (mail, calendar, contact, ai, oauth, settings)
- [x] Worker Pool 기본 구조
- [x] Mail/AI/RAG Processor

### Out
- [x] GmailAdapter - Gmail API 연동
- [x] MailAdapter - PostgreSQL CRUD
- [x] MailBodyAdapter - MongoDB 본문 저장
- [x] Redis Stream Producer/Consumer

## 구현 필요

### In
- [ ] `webhook/` - Pub/Sub Webhook Handler
- [ ] Worker Pool 지능형 스케일링
- [ ] Rate Limiter 통합

### Out
- [ ] `provider/pubsub_adapter.go` - Google Pub/Sub
- [ ] `realtime/sse_adapter.go` - SSE 푸시
- [ ] `persistence/sync_state_adapter.go` - 동기화 상태
