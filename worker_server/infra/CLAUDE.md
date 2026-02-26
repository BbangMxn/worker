# Infra Layer

## 목적

인프라 관련 코드. DB 연결, 미들웨어 등.

## 설계 방향

```
infra/
├── database/       # DB 연결
│   └── database.go # PostgreSQL, pgx pool
│
└── middleware/     # HTTP 미들웨어
    ├── auth.go     # JWT 인증
    ├── error.go    # 에러 핸들링
    └── ratelimit.go # Rate Limiting
```

## 구현 완료

- [x] database.go - PostgreSQL 연결 (pgxpool + sqlx)
- [x] auth.go - Supabase JWT 검증
- [x] error.go - 공통 에러 처리
- [x] ratelimit.go - 기본 Rate Limit

## 구현 필요

- [ ] Rate Limit 고도화 (사용자별, 엔드포인트별)
- [ ] 메트릭 미들웨어 (Prometheus)
- [ ] 로깅 미들웨어 개선
