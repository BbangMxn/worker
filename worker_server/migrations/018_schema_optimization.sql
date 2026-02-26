-- +migrate Up
-- ============================================================================
-- Schema Optimization Migration
-- 목적: 테이블 정리 및 최적화
-- 날짜: 2026-01-14
-- ============================================================================

-- ============================================================================
-- 1. 삭제 대상 테이블 (Neo4j로 이동 또는 미사용)
-- ============================================================================

-- 1.1 AI/개인화 관련 (Neo4j로 이동)
--   - email_embeddings: 벡터 임베딩 → Neo4j
--   - classification_patterns: 분류 패턴 → Neo4j
--   - user_writing_styles: 작문 스타일 → Neo4j
--   - user_profiles: 개인화 프로필 → Neo4j (users 테이블과 중복)
--   - ai_cache: AI 캐시 → Redis로 대체

-- 1.2 미사용 테이블
--   - gmail_labels: labels 테이블과 중복
--   - email_subscriptions: 코드에서 미사용
--   - contact_interactions: 코드에서 미사용
--   - notification_settings: 코드에서 미사용

-- 삭제 순서: FK 의존성 고려
DROP TABLE IF EXISTS email_embeddings CASCADE;
DROP TABLE IF EXISTS classification_patterns CASCADE;
DROP TABLE IF EXISTS user_writing_styles CASCADE;
DROP TABLE IF EXISTS user_profiles CASCADE;
DROP TABLE IF EXISTS ai_cache CASCADE;
DROP TABLE IF EXISTS gmail_labels CASCADE;
DROP TABLE IF EXISTS email_subscriptions CASCADE;
DROP TABLE IF EXISTS contact_interactions CASCADE;
DROP TABLE IF EXISTS notification_settings CASCADE;

-- ============================================================================
-- 2. 누락 테이블 생성
-- ============================================================================

-- 2.1 calendars: 사용자 캘린더 목록
CREATE TABLE IF NOT EXISTS calendars (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES oauth_connections(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL DEFAULT 'google',
    provider_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    color VARCHAR(50),
    is_default BOOLEAN DEFAULT FALSE,
    is_primary BOOLEAN DEFAULT FALSE,
    is_read_only BOOLEAN DEFAULT FALSE,
    is_visible BOOLEAN DEFAULT TRUE,
    timezone VARCHAR(100),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(connection_id, provider_id)
);

CREATE INDEX idx_calendars_user ON calendars(user_id);
CREATE INDEX idx_calendars_connection ON calendars(connection_id);

-- 2.2 companies: 회사 정보
CREATE TABLE IF NOT EXISTS companies (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255),
    industry VARCHAR(100),
    size VARCHAR(50),
    website TEXT,
    description TEXT,
    logo_url TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(user_id, domain)
);

CREATE INDEX idx_companies_user ON companies(user_id);
CREATE INDEX idx_companies_domain ON companies(domain);

-- 2.3 webhook_configs: 웹훅 설정
CREATE TABLE IF NOT EXISTS webhook_configs (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES oauth_connections(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    resource_type VARCHAR(50) NOT NULL DEFAULT 'mail',
    subscription_id VARCHAR(255),
    resource_uri TEXT,
    channel_id VARCHAR(255),
    channel_token TEXT,
    expires_at TIMESTAMPTZ,
    status VARCHAR(50) DEFAULT 'active',
    failure_count INT DEFAULT 0,
    last_triggered_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(connection_id, resource_type)
);

CREATE INDEX idx_webhook_configs_user ON webhook_configs(user_id);
CREATE INDEX idx_webhook_configs_connection ON webhook_configs(connection_id);
CREATE INDEX idx_webhook_configs_expires ON webhook_configs(expires_at) WHERE status = 'active';
CREATE INDEX idx_webhook_configs_status ON webhook_configs(status);

-- 2.4 user_settings: 사용자 설정
CREATE TABLE IF NOT EXISTS user_settings (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE UNIQUE,

    -- 기본 설정
    default_signature TEXT,
    auto_reply_enabled BOOLEAN DEFAULT FALSE,
    auto_reply_message TEXT,

    -- AI 설정
    ai_enabled BOOLEAN DEFAULT TRUE,
    ai_auto_classify BOOLEAN DEFAULT TRUE,
    ai_auto_summarize BOOLEAN DEFAULT TRUE,
    ai_tone VARCHAR(50) DEFAULT 'professional',

    -- 알림 설정
    notify_new_email BOOLEAN DEFAULT TRUE,
    notify_important_only BOOLEAN DEFAULT FALSE,
    notify_calendar BOOLEAN DEFAULT TRUE,

    -- UI 설정
    theme VARCHAR(50) DEFAULT 'system',
    language VARCHAR(10) DEFAULT 'ko',
    timezone VARCHAR(100) DEFAULT 'Asia/Seoul',

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_user_settings_user ON user_settings(user_id);

-- 2.5 classification_rules: 분류 규칙
CREATE TABLE IF NOT EXISTS classification_rules (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE UNIQUE,

    -- 중요 판단 규칙
    important_domains TEXT[] DEFAULT '{}',
    important_keywords TEXT[] DEFAULT '{}',

    -- 무시 규칙
    ignore_senders TEXT[] DEFAULT '{}',
    ignore_keywords TEXT[] DEFAULT '{}',

    -- 우선순위 규칙 (JSON)
    high_priority_rules JSONB DEFAULT '[]',
    low_priority_rules JSONB DEFAULT '[]',

    -- 카테고리 규칙 (JSON)
    category_rules JSONB DEFAULT '[]',

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_classification_rules_user ON classification_rules(user_id);

-- ============================================================================
-- 3. emails 테이블 컬럼 추가
-- ============================================================================

-- 3.1 발송자 프로필 사진 URL (없으면 프론트에서 이름 첫 글자로 아바타 생성)
ALTER TABLE emails ADD COLUMN IF NOT EXISTS sender_photo_url TEXT;

-- 3.2 임베딩 벡터 (email_embeddings 테이블 대체)
ALTER TABLE emails ADD COLUMN IF NOT EXISTS embedding vector(1536);

-- 임베딩 인덱스 (HNSW)
CREATE INDEX IF NOT EXISTS idx_emails_embedding ON emails
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 24, ef_construction = 100);

-- ============================================================================
-- 4. 중복 인덱스 정리
-- ============================================================================

-- emails 테이블
DROP INDEX IF EXISTS idx_emails_folder;           -- idx_emails_folder_date가 커버
DROP INDEX IF EXISTS idx_emails_user_id;          -- idx_emails_user_id_email_date가 커버
DROP INDEX IF EXISTS idx_emails_is_read;          -- idx_emails_user_is_read가 커버
DROP INDEX IF EXISTS idx_emails_user_folder;      -- idx_emails_folder_date와 중복

-- email_threads 테이블
DROP INDEX IF EXISTS idx_email_threads_user_id;           -- idx_email_threads_user_date가 커버
DROP INDEX IF EXISTS idx_email_threads_user_id_latest;    -- idx_email_threads_user_date와 동일

-- calendar_sync_states 테이블
DROP INDEX IF EXISTS idx_calendar_sync_states_connection; -- idx_calendar_sync_states_connection_id와 중복

-- contacts 테이블
DROP INDEX IF EXISTS idx_contacts_relationship_score;    -- idx_contacts_relationship가 커버

-- ============================================================================
-- 5. RLS 정책 설정
-- ============================================================================

-- calendars RLS
ALTER TABLE calendars ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view own calendars" ON calendars
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Users can insert own calendars" ON calendars
    FOR INSERT WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update own calendars" ON calendars
    FOR UPDATE USING (auth.uid() = user_id);

CREATE POLICY "Users can delete own calendars" ON calendars
    FOR DELETE USING (auth.uid() = user_id);

-- companies RLS
ALTER TABLE companies ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view own companies" ON companies
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Users can insert own companies" ON companies
    FOR INSERT WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update own companies" ON companies
    FOR UPDATE USING (auth.uid() = user_id);

CREATE POLICY "Users can delete own companies" ON companies
    FOR DELETE USING (auth.uid() = user_id);

-- webhook_configs RLS
ALTER TABLE webhook_configs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view own webhook_configs" ON webhook_configs
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Users can insert own webhook_configs" ON webhook_configs
    FOR INSERT WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update own webhook_configs" ON webhook_configs
    FOR UPDATE USING (auth.uid() = user_id);

CREATE POLICY "Users can delete own webhook_configs" ON webhook_configs
    FOR DELETE USING (auth.uid() = user_id);

-- user_settings RLS
ALTER TABLE user_settings ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view own settings" ON user_settings
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Users can insert own settings" ON user_settings
    FOR INSERT WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update own settings" ON user_settings
    FOR UPDATE USING (auth.uid() = user_id);

-- classification_rules RLS
ALTER TABLE classification_rules ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view own rules" ON classification_rules
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Users can insert own rules" ON classification_rules
    FOR INSERT WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update own rules" ON classification_rules
    FOR UPDATE USING (auth.uid() = user_id);

-- +migrate Down
-- 롤백: 생성된 테이블 삭제
DROP TABLE IF EXISTS classification_rules CASCADE;
DROP TABLE IF EXISTS user_settings CASCADE;
DROP TABLE IF EXISTS webhook_configs CASCADE;
DROP TABLE IF EXISTS companies CASCADE;
DROP TABLE IF EXISTS calendars CASCADE;

-- 롤백: 삭제된 인덱스 복원 (필요시)
-- CREATE INDEX idx_emails_folder ON emails(user_id, folder);
-- CREATE INDEX idx_emails_user_id ON emails(user_id);
-- CREATE INDEX idx_emails_is_read ON emails(user_id, is_read);
-- CREATE INDEX idx_emails_user_folder ON emails(user_id, folder, email_date DESC);
-- CREATE INDEX idx_email_threads_user_id ON email_threads(user_id);
-- CREATE INDEX idx_email_threads_user_id_latest ON email_threads(user_id, latest_date DESC);
-- CREATE INDEX idx_calendar_sync_states_connection ON calendar_sync_states(connection_id);
-- CREATE INDEX idx_contacts_relationship_score ON contacts(relationship_score);

-- 롤백: 삭제된 테이블 복원은 별도 마이그레이션 필요
