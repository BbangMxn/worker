# Port Layer

## 목적

Core와 외부 세계의 경계를 정의하는 인터페이스.

## 설계 방향

### In (Inbound Port)
- UseCase 인터페이스
- Adapter(HTTP, Worker)가 호출

### Out (Outbound Port)
- Repository, Provider 인터페이스
- Service가 호출, Adapter가 구현

```
port/
├── in/           # UseCase 인터페이스
│   ├── mail.go
│   ├── calendar.go
│   ├── contact.go
│   └── ai.go
│
└── out/          # Repository 인터페이스
    ├── mail_repository.go
    ├── mail_provider.go
    ├── messaging.go
    └── ...
```

## 구현 완료

### In
- [x] MailService - CRUD, 검색
- [x] CalendarService - 일정 관리
- [x] ContactService - 연락처 관리
- [x] AIService - 분류, 요약, 답장

### Out
- [x] MailRepository - PostgreSQL
- [x] MailBodyRepository - MongoDB
- [x] MailProvider - Gmail API
- [x] MessageQueue - Redis Stream (기본)

## 구현 필요

### In
- [ ] MailSyncUseCase - 동기화 오케스트레이션
- [ ] RealtimePushUseCase - SSE 푸시

### Out
- [ ] SyncStateRepository - 동기화 상태 관리
- [ ] PushNotificationPort - Pub/Sub 연동
- [ ] RealtimePort - SSE/WebSocket
