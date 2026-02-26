-- Calendar Sync States: 캘린더 동기화 상태 관리
CREATE TABLE IF NOT EXISTS calendar_sync_states (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES oauth_connections(id) ON DELETE CASCADE,
    calendar_id VARCHAR(255) NOT NULL,
    provider VARCHAR(20) NOT NULL DEFAULT 'google',

    -- Sync token for incremental sync
    sync_token TEXT,

    -- Watch 상태 (Google Calendar Push Notification)
    watch_id VARCHAR(255),
    watch_expiry TIMESTAMP WITH TIME ZONE,
    watch_resource_id VARCHAR(255),

    -- 동기화 상태
    status VARCHAR(20) NOT NULL DEFAULT 'idle',
    last_sync_at TIMESTAMP WITH TIME ZONE,
    last_error TEXT,

    -- 타임스탬프
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(connection_id, calendar_id)
);

-- 인덱스
CREATE INDEX IF NOT EXISTS idx_calendar_sync_states_user_id ON calendar_sync_states(user_id);
CREATE INDEX IF NOT EXISTS idx_calendar_sync_states_connection_id ON calendar_sync_states(connection_id);
CREATE INDEX IF NOT EXISTS idx_calendar_sync_states_status ON calendar_sync_states(status);
CREATE INDEX IF NOT EXISTS idx_calendar_sync_states_watch_expiry ON calendar_sync_states(watch_expiry);
CREATE INDEX IF NOT EXISTS idx_calendar_sync_states_watch_id ON calendar_sync_states(watch_id);
