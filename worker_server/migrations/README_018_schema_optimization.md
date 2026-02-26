# 018_schema_optimization 마이그레이션

## 개요
데이터베이스 스키마 최적화를 위한 마이그레이션입니다.
테이블 수를 27개에서 23개로 줄이고, 저장소 역할을 명확히 분리합니다.

## 저장소 역할 분리

| 저장소 | 역할 | 데이터 |
|--------|------|--------|
| **Supabase (PostgreSQL)** | 메타데이터 | 이메일 메타, 사용자, OAuth, 동기화 상태 |
| **MongoDB** | 원본 본문 | 이메일 본문 (30일 TTL, 압축) |
| **Neo4j** | AI/개인화 | 임베딩, 관계, 스타일 분석, RAG |
| **Redis** | 캐시 | AI 캐시, 세션, 임시 데이터 |

---

## 변경 사항

### 1. 삭제 테이블 (9개)

#### Neo4j로 이동 (5개)
| 테이블 | 이유 |
|--------|------|
| `email_embeddings` | 벡터 임베딩 → Neo4j |
| `classification_patterns` | 분류 패턴 → Neo4j |
| `user_writing_styles` | 작문 스타일 분석 → Neo4j |
| `user_profiles` | 개인화 프로필 → Neo4j |
| `ai_cache` | AI 캐시 → Redis |

#### 미사용/중복 (4개)
| 테이블 | 이유 |
|--------|------|
| `gmail_labels` | `labels` 테이블과 중복 |
| `email_subscriptions` | 코드에서 미사용 |
| `contact_interactions` | 코드에서 미사용 |
| `notification_settings` | 코드에서 미사용 |

### 2. 생성 테이블 (5개)

| 테이블 | 용도 |
|--------|------|
| `calendars` | 사용자 캘린더 목록 관리 |
| `companies` | 회사 정보 관리 |
| `webhook_configs` | Gmail/Outlook 웹훅 설정 |
| `user_settings` | 사용자 설정 (AI, 알림, UI) |
| `classification_rules` | 이메일 분류 규칙 |

### 3. 삭제 인덱스 (8개 중복)

| 테이블 | 삭제 인덱스 | 대체 인덱스 |
|--------|------------|------------|
| `emails` | `idx_emails_folder` | `idx_emails_folder_date` |
| `emails` | `idx_emails_user_id` | `idx_emails_user_id_email_date` |
| `emails` | `idx_emails_is_read` | `idx_emails_user_is_read` |
| `emails` | `idx_emails_user_folder` | `idx_emails_folder_date` |
| `email_threads` | `idx_email_threads_user_id` | `idx_email_threads_user_date` |
| `email_threads` | `idx_email_threads_user_id_latest` | `idx_email_threads_user_date` |
| `calendar_sync_states` | `idx_calendar_sync_states_connection` | `idx_calendar_sync_states_connection_id` |
| `contacts` | `idx_contacts_relationship_score` | `idx_contacts_relationship` |

---

## 최종 테이블 구조 (23개)

### 핵심 테이블
- `users` - 사용자
- `oauth_connections` - OAuth 연결
- `user_settings` - 사용자 설정 (신규)
- `classification_rules` - 분류 규칙 (신규)

### 이메일
- `emails` - 이메일 메타데이터
- `email_threads` - 스레드
- `email_labels` - 이메일-라벨 연결
- `email_translations` - 번역
- `email_versions` - 버전 관리
- `email_templates` - 템플릿

### 라벨
- `labels` - 라벨

### 캘린더
- `calendars` - 캘린더 목록 (신규)
- `calendar_events` - 일정
- `calendar_sync_states` - 동기화 상태
- `calendar_attendee_responses` - 참석자 응답

### 연락처
- `contacts` - 연락처
- `companies` - 회사 (신규)

### 동기화
- `sync_states` - 메일 동기화 상태
- `webhook_configs` - 웹훅 설정 (신규)

### 오프라인 우선
- `modifiers` - 수정 작업 큐
- `modifier_batches` - 배치 작업
- `conflicts` - 충돌 관리

### 기타
- `notifications` - 알림
- `keyboard_shortcuts` - 키보드 단축키

---

## 코드 변경 필요

### 삭제 필요 (Neo4j로 이동 후)
1. `core/agent/rag/vectorstore.go` - pgvector → Neo4j 클라이언트로 변경

### 확인 필요
1. `email_embeddings` 참조하는 코드 확인
2. `user_profiles` 참조하는 코드 확인

---

## 적용 방법

```bash
# Supabase에서 직접 SQL 실행 또는
# supabase migration 명령 사용
```

## 롤백

`+migrate Down` 섹션 참조. 단, 삭제된 테이블의 데이터는 복구 불가.
