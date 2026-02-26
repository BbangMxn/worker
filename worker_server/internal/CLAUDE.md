# Internal Layer

## 목적

내부 유틸리티 및 초기화 로직. 외부에 노출되지 않음.

## 설계 방향

```
internal/
├── bootstrap/      # 의존성 주입, 초기화
│   ├── deps.go     # 의존성 생성
│   ├── api.go      # API 서버 설정
│   └── worker.go   # Worker 설정
│
└── stream/         # Redis Stream 유틸리티
    ├── redis.go    # Redis 연결
    ├── producer.go
    └── consumer.go
```

## 구현 완료

- [x] deps.go - 전체 의존성 주입
- [x] api.go - Fiber 라우팅 설정
- [x] worker.go - Worker Pool 초기화
- [x] Redis Stream 기본 연결

## 구현 필요

- [ ] deps.go에 새 어댑터 추가 (PubSub, SSE)
- [ ] worker.go 스케일링 로직
- [ ] stream/priority.go - 우선순위 큐
