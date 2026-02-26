# Database Design Document

> Polyglot Persistence — PostgreSQL + MongoDB + Neo4j + Redis 4-DB 아키텍처

---

## Storage Strategy

| DB | Role | Data |
|----|------|------|
| **PostgreSQL** (Supabase) | Primary RDBMS | 사용자, 이메일 메타데이터, 캘린더, 연락처, 설정, 분류 규칙 |
| **MongoDB** | Document Store | 이메일 본문 (HTML/Text), gzip 압축, 30-day TTL |
| **Neo4j** | Graph DB | 사용자 문체, 톤 선호도, 연락처 관계 (RAG 개인화) |
| **Redis** | Queue + Cache | Job Queue (Streams), L2 Cache, 세션, Rate Limit |
| **pgvector** | Vector Store | 이메일 임베딩 (1536-dim, OpenAI ada-002) |

---

## 1. PostgreSQL Schema

### 1.1 Migrations

31개 마이그레이션 파일로 스키마를 관리합니다.

| # | File | Description |
|---|------|-------------|
| 001 | `initial.sql` | users, oauth_connections |
| 002 | `emails.sql` | emails 테이블 + AI 분류 컬럼 |
| 003 | `labels.sql` | labels, email_labels 조인 테이블 |
| 004 | `calendar.sql` | calendars, calendar_events |
| 005 | `contacts.sql` | contacts, companies |
| 006 | `settings.sql` | user_settings (이메일, AI, 알림 설정) |
| 007 | `classification.sql` | classification_rules (legacy) |
| 008 | `embeddings.sql` | email_embeddings + pgvector (1536-dim) |
| 009 | `profiles.sql` | user_profiles (개인화, 현재 Neo4j로 이관) |
| 010 | `reports.sql` | email_reports (일간/주간/월간 분석) |
| 011 | `classification_rules.sql` | 도메인/키워드/VIP 기반 분류 규칙 |
| 012 | `notifications.sql` | notifications + notification_settings + 트리거 |
| 013 | `sync_states.sql` | Gmail Watch historyId 추적 |
| 014 | `performance_indexes.sql` | 공통 쿼리 복합 인덱스 |
| 015 | `calendar_sync_states.sql` | 캘린더 동기화 + watch token |
| 016 | `update_contacts_schema.sql` | 컬럼 리네임 (title→job_title, avatar_url→photo_url) |
| 017 | `email_templates.sql` | 이메일 템플릿 + RLS |
| 018 | `schema_optimization.sql` | 테이블 9개 삭제, 5개 신규, embedding 컬럼 추가 |
| 019 | `attachments.sql` | 이메일 첨부파일 메타데이터 |
| 020 | `folder_label_category_system.sql` | folders, smart_folders, sender_profiles, known_domains, enum |
| 021 | `ai_status_column.sql` | ai_status enum (pending/processing/completed/failed) |
| 022 | `add_label_mapping_type.sql` | provider mapping type (label vs category) |
| 023 | `enable_rls_and_add_columns.sql` | attachments/notifications/known_domains RLS |
| 024 | `score_based_classification.sql` | label_rules, classification_cache (HNSW), classification_rules_v2 |
| 025 | `performance_indexes.sql` | 추가 최적화 인덱스 |
| 026 | `priority_score_float.sql` | ai_priority INT → FLOAT (0.0-1.0) 전환 |
| 027 | `add_missing_ai_columns.sql` | ai_score, classification_source 추가 |
| 028 | `inbox_category_indexes.sql` | category/subcategory 필터링 인덱스 |
| 029 | `add_sub_category_enums.sql` | notification, alert, developer 서브카테고리 추가 |
| 030 | `todos.sql` | todo_projects, todos (Snowflake ID) |
| 031 | `naming_unification.sql` | contacts.title → contacts.job_title 통합 |

### 1.2 Enums

```sql
-- 이메일 카테고리
email_category ENUM (
  'primary', 'work', 'personal',
  'updates', 'social', 'promotion'
)

-- 이메일 서브카테고리
email_sub_category ENUM (
  'receipt', 'invoice', 'shipping', 'order', 'travel', 'calendar',
  'account', 'security',
  'sns', 'comment',
  'newsletter', 'marketing', 'deal',
  'notification', 'alert', 'developer'
)

-- 분류 소스
classification_source ENUM ('header', 'domain', 'llm', 'user')

-- AI 처리 상태
ai_status ENUM ('none', 'pending', 'processing', 'completed', 'failed')
```

### 1.3 Core Tables

#### users

```sql
users
├── id              UUID PK
├── email           VARCHAR(255) UNIQUE
├── name            VARCHAR(255)
├── avatar_url      TEXT
├── created_at      TIMESTAMPTZ
├── updated_at      TIMESTAMPTZ
├── deleted_at      TIMESTAMPTZ
└── Index: idx_users_email
```

#### oauth_connections

```sql
oauth_connections
├── id              BIGINT PK
├── user_id         UUID FK → users
├── provider        VARCHAR(50)  -- 'gmail' | 'outlook'
├── email           VARCHAR(255)
├── access_token    TEXT
├── refresh_token   TEXT
├── expires_at      TIMESTAMPTZ
├── is_connected    BOOLEAN
├── created_at      TIMESTAMPTZ
├── updated_at      TIMESTAMPTZ
├── UNIQUE(user_id, provider, email)
└── Indexes: idx_oauth_user, idx_oauth_provider
```

### 1.4 Email System

#### emails (Primary Table)

```sql
emails
├── Core
│   ├── id              BIGINT PK
│   ├── user_id         UUID FK → users
│   ├── connection_id   BIGINT FK → oauth_connections
│   ├── provider        VARCHAR(50)
│   ├── provider_id     VARCHAR(255)  -- UNIQUE(user_id, provider, provider_id)
│   ├── thread_id       VARCHAR(255)
│
├── Headers
│   ├── subject         TEXT
│   ├── from_email      VARCHAR(255)
│   ├── from_name       VARCHAR(255)
│   ├── to_emails       TEXT[]
│   ├── cc_emails       TEXT[]
│   ├── bcc_emails      TEXT[]
│   ├── reply_to        VARCHAR(255)
│   ├── date            TIMESTAMPTZ
│
├── Folder & Labels
│   ├── folder          VARCHAR(50) DEFAULT 'inbox'
│   ├── labels          TEXT[]
│   ├── folder_id       BIGINT FK → folders
│
├── Flags
│   ├── is_read         BOOLEAN
│   ├── is_starred      BOOLEAN
│   ├── has_attachments  BOOLEAN
│
├── AI Classification
│   ├── ai_category            VARCHAR(50)
│   ├── ai_sub_category        email_sub_category
│   ├── ai_priority            NUMERIC(4,3)  -- 0.0 ~ 1.0 (Eisenhower Matrix)
│   ├── ai_summary             TEXT
│   ├── ai_tags                TEXT[]
│   ├── ai_score               FLOAT
│   ├── ai_status              ai_status
│   ├── ai_confidence          DECIMAL(3,2)
│   ├── classification_score   FLOAT
│   ├── classification_stage   VARCHAR(20)  -- rfc | sender | rule | cache | llm
│   ├── classification_source  VARCHAR(20)
│
├── Workflow
│   ├── workflow_status  VARCHAR(50)  -- todo | in_progress | done
│   ├── snoozed_until    TIMESTAMPTZ
│
├── Embedding
│   ├── embedding        vector(1536)  -- pgvector, OpenAI ada-002
│
├── Metadata
│   ├── received_at      TIMESTAMPTZ
│   ├── created_at       TIMESTAMPTZ
│   ├── updated_at       TIMESTAMPTZ
│   ├── deleted_at       TIMESTAMPTZ
│
└── Indexes
    ├── idx_emails_user_date           (user_id, email_date DESC)
    ├── idx_emails_user_unread         (user_id) WHERE is_read = false
    ├── idx_emails_inbox_view          (user_id, email_date DESC)
    │   WHERE folder = 'inbox' AND ai_category IN (...)
    ├── idx_emails_category_date       (user_id, ai_category, email_date DESC)
    ├── idx_emails_subcategory_date    (user_id, ai_sub_category, email_date DESC)
    ├── idx_emails_embedding           HNSW (embedding vector_cosine_ops) WITH (m=24, ef=100)
    ├── idx_emails_ai_priority         (ai_priority DESC NULLS LAST)
    └── idx_emails_todo_priority       (user_id, workflow_status, ai_priority DESC, email_date DESC)
```

#### Priority Score (Eisenhower Matrix)

| Score | Level | Description |
|-------|-------|-------------|
| 0.80 ~ 1.00 | Urgent | 즉각 조치 필요 |
| 0.60 ~ 0.79 | High | 중요, 빠른 대응 필요 |
| 0.40 ~ 0.59 | Normal | 관련성 있음 |
| 0.20 ~ 0.39 | Low | 지연 가능 |
| 0.00 ~ 0.19 | Lowest | 배경 노이즈 |

#### email_attachments

```sql
email_attachments
├── id              BIGINT PK
├── email_id        BIGINT FK → emails
├── external_id     VARCHAR(255)
├── filename        VARCHAR(500)
├── mime_type       VARCHAR(255)
├── size            BIGINT
├── content_id      VARCHAR(255)  -- inline images
├── is_inline       BOOLEAN
├── created_at      TIMESTAMPTZ
├── UNIQUE(email_id, external_id)
└── Indexes: idx_attachments_email, idx_attachments_filename, idx_attachments_mime_type
```

#### labels & email_labels

```sql
labels
├── id              BIGINT PK
├── user_id         UUID FK
├── connection_id   BIGINT FK  -- NULL = user-created
├── provider_id     VARCHAR(255)
├── name            VARCHAR(255)
├── color           VARCHAR(7)
├── is_system       BOOLEAN
├── is_visible      BOOLEAN
├── email_count     INT  -- cached
├── unread_count    INT  -- cached
└── Indexes: idx_labels_user, idx_labels_connection

email_labels
├── email_id        BIGINT FK → emails
├── label_id        BIGINT FK → labels
├── created_at      TIMESTAMPTZ
└── PK: (email_id, label_id)
```

### 1.5 Folder System

#### folders & smart_folders

```sql
folders
├── id              BIGINT PK
├── user_id         UUID FK
├── name            VARCHAR(100)
├── type            VARCHAR(20)  -- system | user
├── system_key      VARCHAR(20)  -- inbox | sent | drafts | trash | spam | archive
├── color, icon     VARCHAR
├── position        INT
├── total_count     INT  -- cached
├── unread_count    INT  -- cached
├── UNIQUE(user_id, name)
├── UNIQUE(user_id, system_key)
└── Indexes: idx_folders_user, idx_folders_system

smart_folders
├── id              BIGINT PK
├── user_id         UUID FK
├── name, icon, color
├── query           JSONB  -- {categories, sub_categories, labels, folders, is_read, date_range}
├── sort_by         VARCHAR
├── sort_order      VARCHAR
├── is_system       BOOLEAN
├── is_visible      BOOLEAN
├── position        INT
├── total_count     INT  -- cached
├── unread_count    INT  -- cached
└── Indexes: idx_smart_folders_user, idx_smart_folders_visible
```

### 1.6 Classification System

#### sender_profiles (발신자 학습)

```sql
sender_profiles
├── id                      BIGINT PK
├── user_id                 UUID FK
├── email, domain, name     VARCHAR
│
├── Learned Classification
│   ├── learned_category       email_category
│   ├── learned_sub_category   email_sub_category
│
├── User Settings
│   ├── is_vip              BOOLEAN
│   ├── is_muted            BOOLEAN
│
├── Engagement Metrics
│   ├── email_count         INT
│   ├── read_rate           FLOAT (0~1)
│   ├── reply_rate          FLOAT (0~1)
│   ├── delete_rate         FLOAT (0~1)
│   ├── avg_reply_time_minutes  INT
│   ├── interaction_count   INT
│   ├── importance_score    FLOAT (0.0-1.0)  -- computed
│   ├── is_contact          BOOLEAN
│   ├── confirmed_labels    BIGINT[]
│   ├── first_seen_at       TIMESTAMPTZ
│   ├── last_interacted_at  TIMESTAMPTZ
│
├── UNIQUE(user_id, email)
└── Indexes: idx_sender_profiles_user, idx_sender_profiles_vip,
             idx_sender_profiles_importance (user_id, importance_score DESC)
```

#### known_domains (글로벌 도메인 DB)

```sql
known_domains
├── id              SERIAL PK
├── domain          VARCHAR(255) UNIQUE
├── category        email_category
├── sub_category    email_sub_category
├── confidence      FLOAT DEFAULT 1.0
├── source          VARCHAR(20)  -- system | community | verified
└── Initial data: 50+ 도메인 (PayPal, Amazon, Booking.com, GitHub 등)
```

#### classification_cache (시맨틱 캐시)

```sql
classification_cache
├── id              BIGINT PK
├── user_id         UUID FK
├── embedding       vector(1536)
├── category        VARCHAR(50)
├── sub_category    VARCHAR(50)
├── priority        VARCHAR(20)
├── labels          BIGINT[]
├── score           FLOAT (0.0-1.0)
├── usage_count     INT
├── last_used_at    TIMESTAMPTZ
├── expires_at      TIMESTAMPTZ  -- NOW() + 30 days
├── Index: HNSW (embedding) WITH (m=16, ef=64)
└── Index: idx_classification_cache_expires
```

#### classification_rules_v2 (Score 기반 v2)

```sql
classification_rules_v2
├── id              BIGINT PK
├── user_id         UUID FK
├── type            VARCHAR(50)  -- exact_sender(0.99) | sender_domain(0.95) | subject_keyword(0.90) | body_keyword(0.85) | ai_prompt
├── pattern         TEXT
├── action          VARCHAR(50)  -- assign_category | assign_priority | assign_label | mark_important | mark_spam
├── value           TEXT
├── score           FLOAT
├── position        INT
├── is_active       BOOLEAN
├── hit_count       INT
├── last_hit_at     TIMESTAMPTZ
├── UNIQUE(user_id, type, pattern, action)
└── Indexes: idx_classification_rules_v2_user, idx_classification_rules_v2_action
```

### 1.7 Calendar System

```sql
calendars
├── id, user_id, connection_id
├── provider, provider_id
├── name, description, color
├── is_default, is_read_only
├── UNIQUE(connection_id, provider_id)
└── Index: idx_calendars_user

calendar_events
├── id              BIGINT PK
├── calendar_id     BIGINT FK → calendars
├── user_id         UUID FK
├── provider_id, title, description, location
├── start_time      TIMESTAMPTZ
├── end_time        TIMESTAMPTZ
├── is_all_day      BOOLEAN
├── timezone        VARCHAR
├── status, organizer, attendees[]
├── is_recurring    BOOLEAN
├── recurrence_rule TEXT
├── reminders       INT[]
├── meeting_url     TEXT
├── UNIQUE(calendar_id, provider_id)
└── Indexes: idx_events_calendar, idx_events_user, idx_events_time

calendar_sync_states
├── id, user_id, connection_id, calendar_id
├── provider        -- google | outlook
├── sync_token      TEXT  -- incremental sync
├── watch_id, watch_expiry, watch_resource_id
├── status, last_sync_at, last_error
├── UNIQUE(connection_id, calendar_id)
└── Indexes: idx_calendar_sync_states_user_id, idx_calendar_sync_states_status
```

### 1.8 Contacts

```sql
contacts
├── id              BIGINT PK
├── user_id         UUID FK
├── email           VARCHAR(255)
├── name, company, job_title
├── phone, photo_url
├── notes           TEXT
├── tags            TEXT[]
├── department, groups TEXT[]
├── interaction_count       INT
├── last_interaction_at     TIMESTAMPTZ
├── relationship_score      SMALLINT
├── interaction_frequency   VARCHAR
├── is_favorite     BOOLEAN
├── UNIQUE(user_id, email)
└── Indexes: idx_contacts_user, idx_contacts_email, idx_contacts_company, idx_contacts_is_favorite

companies
├── id              BIGINT PK
├── user_id         UUID FK
├── name, domain, industry, size
├── website, description, logo_url
├── UNIQUE(user_id, domain)
└── Indexes: idx_companies_user, idx_companies_domain
```

### 1.9 Settings & Notifications

```sql
user_settings
├── id, user_id (UNIQUE)
├── default_signature, auto_reply_enabled, auto_reply_message
├── ai_enabled, ai_auto_classify, ai_auto_summarize
├── ai_tone          VARCHAR(50)  -- professional | casual | formal
├── notify_new_email, notify_important_only, notify_calendar
├── theme            VARCHAR(50)  -- system | light | dark
├── language         VARCHAR(10)  -- en | ko
├── timezone         VARCHAR(100)

notifications
├── id              BIGINT PK
├── user_id         UUID FK
├── type            VARCHAR(50)  -- email_received | email_classified | calendar_event | system
├── title, body     TEXT
├── entity_type, entity_id
├── is_read         BOOLEAN
├── priority        VARCHAR(20)  -- low | normal | high | urgent
├── metadata        JSONB
├── expires_at      TIMESTAMPTZ
└── Indexes: idx_notifications_user_unread, idx_notifications_type

notification_settings
├── id, user_id (UNIQUE)
├── email_new_mail, email_important_only, email_digest
├── email_digest_frequency   -- daily | weekly
├── push_enabled, push_new_mail, push_calendar, push_mentions
├── inapp_enabled
├── quiet_hours_enabled, quiet_hours_start, quiet_hours_end, quiet_hours_timezone
└── TRIGGER: 사용자 생성 시 자동으로 기본값 생성
```

### 1.10 Todo System

```sql
todo_projects  (Snowflake ID)
├── id              BIGINT PK
├── user_id         UUID FK
├── name            VARCHAR(200)
├── description     TEXT
├── area            VARCHAR(100)  -- Work | Personal | Finance
├── color, icon
├── status          VARCHAR(20)  -- active | completed | archived
├── sort_order      INT
└── Indexes: idx_todo_projects_user, idx_todo_projects_area

todos  (Snowflake ID)
├── id              BIGINT PK
├── user_id         UUID FK
├── project_id      BIGINT FK → todo_projects (optional)
├── parent_id       BIGINT FK → todos (subtask)
├── area            VARCHAR(100)
├── title           VARCHAR(500)
├── description     TEXT
├── status          VARCHAR(20)  -- inbox | pending | in_progress | waiting | completed | cancelled
├── priority        INT  -- 1=urgent, 2=high, 3=normal, 4=low
├── due_date        DATE
├── due_datetime    TIMESTAMPTZ
├── start_date      DATE
├── source_type     VARCHAR  -- email | calendar | agent | manual | jira | github | linear
├── source_id, source_url, source_metadata JSONB
├── related_email_id    BIGINT
├── related_event_id    BIGINT
├── tags            TEXT[]
├── sort_order      INT
├── completed_at    TIMESTAMPTZ
└── Indexes:
    ├── idx_todos_inbox     WHERE project_id IS NULL AND area IS NULL AND status = 'inbox'
    ├── idx_todos_today     (user_id, due_date, priority)
    ├── idx_todos_upcoming  (user_id, due_date) WHERE status NOT IN (...)
    ├── idx_todos_project   (project_id, status, sort_order)
    └── idx_todos_source    (source_type, source_id)
```

### 1.11 Email Templates & Reports

```sql
email_templates
├── id              BIGINT PK
├── user_id         UUID FK
├── name            VARCHAR(255)
├── category        VARCHAR(50)  -- signature | reply | follow_up | intro | thank_you | meeting | custom
├── subject, body   TEXT
├── html_body       TEXT
├── variables       JSONB[]  -- [{name, placeholder, default_value, description}]
├── tags            TEXT[]
├── is_default      BOOLEAN
├── is_archived     BOOLEAN
├── usage_count     INT
├── UNIQUE(user_id, category) WHERE is_default = true
└── Indexes: idx_email_templates_tags GIN, idx_email_templates_name_search GIN (tsvector)

email_reports
├── id              BIGINT PK
├── user_id         UUID FK
├── type            VARCHAR(20)  -- daily | weekly | monthly
├── period_start, period_end TIMESTAMPTZ
├── total_received, sent, read, unread  INT
├── category_breakdown, priority_breakdown  JSONB
├── top_senders     JSONB[]
├── avg_response_time_minutes  FLOAT
├── ai_classified_count, ai_replies_generated  INT
└── Indexes: idx_reports_user, idx_reports_type, idx_reports_period
```

### 1.12 Sync States

```sql
sync_states
├── id              BIGINT PK
├── user_id         UUID FK
├── connection_id   BIGINT FK UNIQUE
├── provider        VARCHAR(20)  -- gmail | outlook
├── history_id      BIGINT  -- Gmail historyId for delta sync
├── watch_expiry    TIMESTAMPTZ
├── watch_resource_id  VARCHAR
├── status          VARCHAR(20)  -- idle | syncing | error
├── last_sync_at    TIMESTAMPTZ
├── last_error      TEXT
├── total_synced    INT
├── last_sync_count INT
└── Indexes: idx_sync_states_user_id, idx_sync_states_status, idx_sync_states_watch_expiry
```

### 1.13 Row-Level Security (RLS)

다음 테이블에 RLS가 적용되어 있습니다:

- `email_templates`, `calendars`, `calendar_events`, `companies`
- `user_settings`, `classification_rules`
- `folders`, `folder_provider_mappings`, `label_provider_mappings`
- `sender_profiles`, `smart_folders`
- `email_attachments`, `notifications`
- `known_domains` (public read, service_role write)
- `label_rules`, `classification_cache`, `classification_rules_v2`

---

## 2. MongoDB Schema

### mail_bodies Collection

```javascript
{
  _id:              ObjectId,
  email_id:         int64,           // UNIQUE, FK to PostgreSQL emails.id
  connection_id:    int64,
  external_id:      string,

  html:             bytes,           // gzip compressed if > 1KB
  text:             bytes,           // gzip compressed if > 1KB
  is_compressed:    boolean,

  attachments: [{
    id:             string,
    name:           string,
    mime_type:      string,
    size:           int64,
    content_id:     string,          // for inline images
    is_inline:      boolean,
    url:            string
  }],

  original_size:    int64,
  compressed_size:  int64,

  cached_at:        timestamp,
  expires_at:       timestamp,       // TTL: 30 days
  ttl_days:         int
}

// Indexes
{ email_id: 1 }       // UNIQUE
{ connection_id: 1 }
{ expires_at: 1 }     // TTL index (MongoDB 자동 삭제)
{ cached_at: 1 }
```

**설계 의도:**
- 이메일 본문은 수백 KB에 달할 수 있으므로 PostgreSQL에서 분리
- gzip 압축으로 저장 공간 50-70% 절약
- 30-day TTL로 스토리지 비용 자동 관리

---

## 3. Neo4j Schema

### Graph Model

```
(:User {user_id, email, name, job_title, company})
    ├─ [:HAS_WRITING_STYLE] → (:WritingStyle {
    │       embedding, avg_sentence_length, formality_score, emoji_frequency
    │   })
    ├─ [:HAS_TONE_PREF] → (:TonePreference {context, style, formality})
    ├─ [:HAS_PATTERN] → (:ClassificationPattern {
    │       email_id, from_addr, subject, category, priority, intent, embedding
    │   })
    ├─ [:USES_PHRASE] → (:Phrase {text, count, category, last_used})
    └─ [:HAS_SIGNATURE] → (:Signature {id, text, is_default})
```

### Constraints & Indexes

```cypher
CREATE CONSTRAINT user_id_unique FOR (u:User) REQUIRE u.user_id IS UNIQUE
CREATE INDEX user_email_idx FOR (u:User) ON (u.email)
CREATE VECTOR INDEX pattern_embedding_index FOR (p:ClassificationPattern) ON (p.embedding)
CREATE INDEX pattern_user_idx FOR (p:ClassificationPattern) ON (p.user_id)
```

> **Note**: Retrospective에서 언급한 대로, Neo4j 제거를 검토 중. 개인화 데이터는 MongoDB로 통합하는 방향

---

## 4. Redis

### Streams (Job Queue)

```
# Email
mail:send            이메일 전송
mail:sync            이메일 동기화
mail:batch           배치 이메일 작업
mail:save            비동기 메타데이터 저장
mail:modify          비동기 Provider 상태 동기화
mail:priority        고우선순위 이메일 작업

# Calendar
calendar:sync        캘린더 동기화
calendar:event       캘린더 이벤트 처리
calendar:priority    고우선순위 캘린더 작업

# AI
ai:classify          AI 이메일 분류
ai:summarize         AI 이메일 요약
ai:translate         AI 번역
ai:autocomplete      AI 자동완성
ai:chat              AI 대화
ai:generate_reply    AI 답장 생성
ai:priority          고우선순위 AI 작업

# RAG
rag:index            단건 벡터 인덱싱
rag:batch            배치 벡터 인덱싱
rag:search           벡터 검색

# Profile
profile:analyze      사용자 프로필 분석
```

### Cache Key Patterns

```
cache:emails:{userId}:*       L2 이메일 목록 캐시 (1분 TTL)
cache:session:{sessionId}     AI 세션 캐시 (24시간 TTL)
rate_limit:{userId}           Rate Limit 토큰
sync:{connectionId}           동기화 상태 추적
```

### Consumer Group

```
group:{stream}:workers        Worker Consumer Group
```

- **Delivery**: at-least-once (Consumer Group + ACK)
- **Retry**: 실패 시 최대 3회 재시도
- **DLQ**: 3회 실패 후 로그 기록 (DB 저장 미구현)

---

## 5. ER Diagram (Simplified)

```
users ──┬── oauth_connections ──┬── emails ──── email_labels ──── labels
        │                       │     │
        │                       │     ├── email_attachments
        │                       │     └── embedding (pgvector)
        │                       │
        │                       ├── sync_states
        │                       └── calendars ──── calendar_events
        │                                    └── calendar_sync_states
        │
        ├── contacts
        ├── companies
        ├── user_settings
        ├── notification_settings
        ├── notifications
        ├── classification_rules
        ├── classification_rules_v2
        ├── classification_cache
        ├── sender_profiles
        ├── known_domains (global)
        ├── label_rules
        ├── folders
        ├── smart_folders
        ├── email_templates
        ├── email_reports
        ├── todo_projects ──── todos
        └── user_profiles (→ Neo4j 이관)
```

---

## 6. Index Strategy

### pgvector HNSW Index

```sql
-- 이메일 임베딩 검색 (RAG)
CREATE INDEX idx_emails_embedding ON emails
  USING hnsw (embedding vector_cosine_ops) WITH (m=24, ef=100);

-- 분류 캐시 유사도 검색
CREATE INDEX idx_classification_cache_embedding ON classification_cache
  USING hnsw (embedding vector_cosine_ops) WITH (m=16, ef=64);
```

### Composite Indexes (주요 쿼리 최적화)

```sql
-- 인박스 뷰 (가장 빈번한 쿼리)
CREATE INDEX idx_emails_inbox_view ON emails (user_id, email_date DESC)
  WHERE folder = 'inbox' AND ai_category IN ('primary', 'work', 'personal');

-- 미읽음 필터
CREATE INDEX idx_emails_user_unread ON emails (user_id) WHERE is_read = false;

-- 카테고리 필터링
CREATE INDEX idx_emails_category_date ON emails (user_id, ai_category, email_date DESC);

-- Todo 우선순위
CREATE INDEX idx_emails_todo_priority ON emails
  (user_id, workflow_status, ai_priority DESC, email_date DESC);
```
