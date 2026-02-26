# PostgreSQL Adapters (Supabase)

> 메타데이터 및 핵심 엔티티 저장을 위한 PostgreSQL 어댑터

## 역할

Supabase(PostgreSQL)는 **메타데이터와 관계형 데이터**를 저장합니다:

| 어댑터 | 테이블 | 용도 |
|--------|--------|------|
| `MailAdapter` | emails, email_threads | 이메일 메타데이터, AI 결과, 임베딩 |
| `ContactAdapter` | contacts | 연락처 관리 |
| `CalendarAdapter` | calendars, calendar_events | 캘린더, 일정 |
| `OAuthAdapter` | oauth_connections | OAuth 토큰 관리 |

**본문(HTML/Text)은 MongoDB에 저장**, **벡터 임베딩은 emails.embedding 컬럼에 저장**

## 파일 구조

```
persistence/
├── mail_adapter.go        # 이메일 CRUD, AI 결과, 상태 업데이트
├── mail_adapter_thread.go # 스레드 관리
├── contact_adapter.go     # 연락처 CRUD
├── calendar_adapter.go    # 캘린더/이벤트 CRUD
├── oauth.go               # OAuth 연결 관리
└── errors.go              # 에러 정의
```

## 테이블 스키마

### emails
```sql
emails (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL,
    connection_id BIGINT NOT NULL,
    
    -- Provider
    provider email_provider,      -- gmail, outlook
    account_email TEXT,
    external_id TEXT,             -- Provider 메시지 ID
    
    -- Threading
    thread_id BIGINT,
    message_id TEXT,
    in_reply_to TEXT,
    
    -- Content (snippet만, 본문은 MongoDB)
    subject TEXT,
    snippet TEXT,
    
    -- Participants
    from_email TEXT,
    from_name TEXT,
    sender_photo_url TEXT,        -- 발신자 프로필 사진 URL
    to_emails TEXT[],
    cc_emails TEXT[],
    
    -- Status
    is_read BOOLEAN DEFAULT false,
    folder email_folder,          -- inbox, sent, drafts, trash, archive
    
    -- AI Results
    ai_status ai_status,          -- none, light, full, failed
    ai_category email_category,   -- primary, work, personal, newsletter...
    ai_priority email_priority,   -- low, normal, high, urgent
    ai_summary TEXT,
    ai_intent TEXT,               -- action_required, fyi, urgent, follow_up
    ai_is_urgent BOOLEAN,
    ai_due_date DATE,
    ai_tags TEXT[],
    
    -- Workflow
    workflow_status workflow_status,  -- none, pending, done, snoozed
    snooze_until TIMESTAMPTZ,
    
    -- Vector Embedding (pgvector)
    embedding vector(1536),       -- OpenAI text-embedding-3-small
    
    email_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
)

-- HNSW 인덱스 (코사인 유사도)
CREATE INDEX idx_emails_embedding ON emails 
USING hnsw (embedding vector_cosine_ops) 
WITH (m = 24, ef_construction = 100);
```

## MailAdapter

### 인터페이스: `out.MailRepository`

```go
// CRUD
Create(ctx, *MailEntity) error
Update(ctx, *MailEntity) error
Delete(ctx, id int64) error
GetByID(ctx, id int64) (*MailEntity, error)
GetByExternalID(ctx, connectionID int64, externalID string) (*MailEntity, error)

// 조회
List(ctx, userID uuid.UUID, *MailListQuery) ([]*MailEntity, int, error)
Search(ctx, userID uuid.UUID, query string, limit, offset int) ([]*MailEntity, int, error)
ListByContact(ctx, userID uuid.UUID, contactID int64, limit, offset int) ([]*MailEntity, int, error)

// 상태 업데이트
UpdateReadStatus(ctx, id int64, isRead bool) error
UpdateFolder(ctx, id int64, folder string) error
UpdateTags(ctx, id int64, tags []string) error
UpdateWorkflowStatus(ctx, id int64, status string, snoozeUntil *time.Time) error

// AI 결과
UpdateAIResult(ctx, id int64, *MailAIResult) error

// 배치
BatchUpdateReadStatus(ctx, ids []int64, isRead bool) error
BatchUpdateFolder(ctx, ids []int64, folder string) error
BulkUpsert(ctx, userID uuid.UUID, connectionID int64, mails []*MailEntity) error

// 통계
GetStats(ctx, userID uuid.UUID) (*MailStats, error)
CountUnread(ctx, userID uuid.UUID, connectionID *int64) (int, error)
```

### AI 결과 업데이트

```go
result := &out.MailAIResult{
    Status:     "completed",
    Category:   "work",
    Priority:   2,  // 1=urgent, 5=low
    Summary:    "회의 일정 조율 요청",
    Intent:     "scheduling",
    IsUrgent:   false,
    Tags:       []string{"meeting", "calendar"},
}
adapter.UpdateAIResult(ctx, emailID, result)
```

## 인덱스 (주요)

```sql
-- 기본 조회
CREATE INDEX idx_emails_user_folder ON emails(user_id, folder, email_date DESC);
CREATE INDEX idx_emails_user_id_email_date ON emails(user_id, email_date DESC);

-- 읽음 상태
CREATE INDEX idx_emails_user_is_read ON emails(user_id, is_read) WHERE is_read = false;

-- AI 분류
CREATE INDEX idx_emails_classification_composite 
ON emails(user_id, ai_category, ai_priority, email_date DESC);

-- 긴급 메일
CREATE INDEX idx_emails_urgent ON emails(user_id, email_date DESC) WHERE ai_is_urgent = true;

-- 텍스트 검색 (trigram)
CREATE INDEX idx_emails_subject_trgm ON emails USING GIN(subject gin_trgm_ops);

-- 벡터 검색 (HNSW) - emails 테이블 내 임베딩
CREATE INDEX idx_emails_embedding ON emails 
USING hnsw (embedding vector_cosine_ops) 
WITH (m = 24, ef_construction = 100);
```

## RLS (Row Level Security)

모든 테이블에 RLS 활성화:

```sql
ALTER TABLE emails ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view own emails" ON emails
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Users can insert own emails" ON emails
    FOR INSERT WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update own emails" ON emails
    FOR UPDATE USING (auth.uid() = user_id);

CREATE POLICY "Users can delete own emails" ON emails
    FOR DELETE USING (auth.uid() = user_id);
```

## 데이터 흐름

```
메일 동기화
    ↓
MailAdapter.BulkUpsert() → emails 테이블 (메타데이터)
    ↓
MongoRepository.BulkSaveBody() → MongoDB (본문)
    ↓
VectorStore.Store() → emails.embedding 컬럼 (벡터)
    ↓
AIProcessor.handleClassify() → MailAdapter.UpdateAIResult() (AI 결과)
```

## 주의사항

1. **본문은 MongoDB에 저장**: emails 테이블에는 snippet만 저장
2. **임베딩은 emails.embedding 컬럼에 저장**: 별도 테이블 없이 emails 테이블에 통합
3. **BulkUpsert 사용**: 동기화 시 개별 Create 대신 BulkUpsert로 성능 최적화
4. **UpdateAIResult는 부분 업데이트**: Summary만 업데이트해도 다른 AI 필드 유지
5. **sender_photo_url**: 발신자 프로필 사진 URL 저장 (없으면 이름 첫 글자 사용)
