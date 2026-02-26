-- Sync States: 사용자별 메일 동기화 상태 관리
CREATE TABLE IF NOT EXISTS sync_states (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES oauth_connections(id) ON DELETE CASCADE,
    provider VARCHAR(20) NOT NULL DEFAULT 'gmail',

    -- Gmail History tracking
    history_id BIGINT DEFAULT 0,

    -- Watch 상태 (Gmail Push Notification)
    watch_expiry TIMESTAMP WITH TIME ZONE,
    watch_resource_id VARCHAR(255),

    -- 동기화 상태
    status VARCHAR(20) NOT NULL DEFAULT 'idle',
    last_sync_at TIMESTAMP WITH TIME ZONE,
    last_error TEXT,

    -- 통계
    total_synced BIGINT DEFAULT 0,
    last_sync_count INT DEFAULT 0,

    -- 타임스탬프
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(connection_id)
);

-- 인덱스
CREATE INDEX idx_sync_states_user_id ON sync_states(user_id);
CREATE INDEX idx_sync_states_status ON sync_states(status);
CREATE INDEX idx_sync_states_watch_expiry ON sync_states(watch_expiry);

-- emails 테이블에 AI 분류 필드 추가 (없으면)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'emails' AND column_name = 'ai_category') THEN
        ALTER TABLE emails ADD COLUMN ai_category VARCHAR(20);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'emails' AND column_name = 'ai_priority') THEN
        ALTER TABLE emails ADD COLUMN ai_priority VARCHAR(20);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'emails' AND column_name = 'ai_intent') THEN
        ALTER TABLE emails ADD COLUMN ai_intent VARCHAR(30);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'emails' AND column_name = 'ai_summary') THEN
        ALTER TABLE emails ADD COLUMN ai_summary TEXT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'emails' AND column_name = 'ai_confidence') THEN
        ALTER TABLE emails ADD COLUMN ai_confidence DECIMAL(3,2);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'emails' AND column_name = 'ai_processed_at') THEN
        ALTER TABLE emails ADD COLUMN ai_processed_at TIMESTAMP WITH TIME ZONE;
    END IF;
END $$;

-- AI 필드 인덱스
CREATE INDEX IF NOT EXISTS idx_emails_ai_category ON emails(ai_category);
CREATE INDEX IF NOT EXISTS idx_emails_ai_priority ON emails(ai_priority);
CREATE INDEX IF NOT EXISTS idx_emails_ai_status ON emails(ai_status);
