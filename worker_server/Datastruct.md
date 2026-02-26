# 데이터 구조 설계

## 저장소 역할 분리

| 저장소 | 역할 | 데이터 |
|--------|------|--------|
| **Supabase (PostgreSQL)** | 메타데이터 + 임베딩 | 이메일 메타, 사용자, OAuth, 동기화, 벡터 |
| **MongoDB** | 원본 본문 | 이메일 본문 (30일 TTL, 압축) |
| **Neo4j** | AI/개인화 | 관계 분석, 스타일 학습, 개인화 그래프 |
| **Redis** | 캐시 | AI 캐시, 세션, 임시 데이터 |

---

## Supabase 테이블 구조 (23개)

### 사용자 관련
| 테이블 | 용도 | 주요 컬럼 |
|--------|------|-----------|
| `users` | 사용자 기본 정보 | id, email, name, avatar |
| `user_settings` | 사용자 설정 | ai_enabled, theme, language, timezone |
| `classification_rules` | 분류 규칙 | important_domains, ignore_senders |
| `keyboard_shortcuts` | 키보드 단축키 | preset, shortcuts (JSON) |

### OAuth
| 테이블 | 용도 | 주요 컬럼 |
|--------|------|-----------|
| `oauth_connections` | OAuth 연결 | provider, access_token, refresh_token |

### 이메일
| 테이블 | 용도 | 주요 컬럼 |
|--------|------|-----------|
| `emails` | 이메일 메타데이터 | subject, snippet, from_email, **sender_photo_url**, **embedding** |
| `email_threads` | 스레드 | subject, participants, message_count |
| `email_labels` | 이메일-라벨 연결 | email_id, label_id |
| `email_translations` | 번역 | translated_subject, translated_body |
| `email_versions` | 버전 관리 | version, mod_type, previous_state |
| `email_templates` | 템플릿 | name, body, variables |
| `labels` | 라벨 | name, color, is_system |

### 캘린더
| 테이블 | 용도 | 주요 컬럼 |
|--------|------|-----------|
| `calendars` | 캘린더 목록 | name, provider_id, is_default |
| `calendar_events` | 일정 | title, start_time, end_time, attendees |
| `calendar_sync_states` | 동기화 상태 | sync_token, watch_expiry |
| `calendar_attendee_responses` | 참석자 응답 | email, response |

### 연락처
| 테이블 | 용도 | 주요 컬럼 |
|--------|------|-----------|
| `contacts` | 연락처 | name, email, company, relationship_score |
| `companies` | 회사 정보 | name, domain, logo_url |

### 동기화
| 테이블 | 용도 | 주요 컬럼 |
|--------|------|-----------|
| `sync_states` | 메일 동기화 상태 | status, history_id, watch_expiry |
| `webhook_configs` | 웹훅 설정 | subscription_id, expires_at, status |

### 오프라인 우선
| 테이블 | 용도 | 주요 컬럼 |
|--------|------|-----------|
| `modifiers` | 수정 작업 큐 | type, status, email_id |
| `modifier_batches` | 배치 작업 | email_ids, status |
| `conflicts` | 충돌 관리 | client_state, server_state, resolution |

### 알림
| 테이블 | 용도 | 주요 컬럼 |
|--------|------|-----------|
| `notifications` | 알림 | type, title, body, is_read |

---

## emails 테이블 상세

### 기본 정보
```
id, user_id, connection_id, provider, account_email
external_id, external_thread_id, thread_id
message_id, in_reply_to, references
```

### 발신/수신
```
from_email, from_name, sender_photo_url (NEW)
to_emails[], cc_emails[], bcc_emails[]
```

### 내용
```
subject, snippet
embedding vector(1536) (NEW) -- 임베딩 벡터
```

### 분류
```
folder, tags[], labels[], direction
```

### 상태
```
is_read, is_draft, has_attachment, is_replied, is_forwarded
workflow_status, snooze_until
```

### AI 분석 결과
```
ai_status, ai_category, ai_priority, ai_summary
ai_intent, ai_is_urgent, ai_due_date, ai_action_item
ai_sentiment, ai_tags[]
```

### 관계
```
contact_id
```

### 시간
```
email_date, created_at, updated_at
```

---

## 삭제된 테이블 (9개)

### Neo4j로 이동 (5개)
| 테이블 | 이유 |
|--------|------|
| `email_embeddings` | → `emails.embedding`으로 통합 |
| `classification_patterns` | → Neo4j 그래프 |
| `user_writing_styles` | → Neo4j 개인화 |
| `user_profiles` | → Neo4j 개인화 |
| `ai_cache` | → Redis 캐시 |

### 미사용/중복 (4개)
| 테이블 | 이유 |
|--------|------|
| `gmail_labels` | `labels`와 중복 |
| `email_subscriptions` | 코드 미사용 |
| `contact_interactions` | 코드 미사용 |
| `notification_settings` | 코드 미사용 |

---

## 인덱스 최적화

### 삭제된 중복 인덱스 (8개)
| 테이블 | 삭제 | 대체 |
|--------|------|------|
| emails | idx_emails_folder | idx_emails_folder_date |
| emails | idx_emails_user_id | idx_emails_user_id_email_date |
| emails | idx_emails_is_read | idx_emails_user_is_read |
| emails | idx_emails_user_folder | idx_emails_folder_date |
| email_threads | idx_email_threads_user_id | idx_email_threads_user_date |
| email_threads | idx_email_threads_user_id_latest | idx_email_threads_user_date |
| calendar_sync_states | idx_calendar_sync_states_connection | idx_calendar_sync_states_connection_id |
| contacts | idx_contacts_relationship_score | idx_contacts_relationship |

### 추가된 인덱스
```sql
-- 임베딩 벡터 검색용 HNSW 인덱스
CREATE INDEX idx_emails_embedding ON emails 
    USING hnsw (embedding vector_cosine_ops) 
    WITH (m = 24, ef_construction = 100);
```

---

## 발송자 프로필 처리

```
1. sender_photo_url 있으면 → 이미지 표시
2. 없으면 → from_name 첫 글자로 아바타 생성
3. from_name도 없으면 → from_email 첫 글자 사용
```

---

## 데이터 흐름

### 이메일 조회
```
1. Supabase (emails) → 메타데이터 + 임베딩
2. MongoDB → 원본 본문 (필요시)
```

### 이메일 검색 (벡터)
```
1. 쿼리 임베딩 생성
2. Supabase pgvector → 유사도 검색
3. 결과 반환
```

### AI 개인화
```
1. Neo4j → 사용자 스타일, 관계 그래프
2. LLM → 분류, 요약, 답장 생성
```
