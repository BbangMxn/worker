-- +migrate Up

-- User settings table
CREATE TABLE IF NOT EXISTS user_settings (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,

    -- Email settings
    default_signature TEXT,
    auto_reply_enabled BOOLEAN DEFAULT FALSE,
    auto_reply_message TEXT,

    -- AI settings
    ai_enabled BOOLEAN DEFAULT TRUE,
    ai_auto_classify BOOLEAN DEFAULT TRUE,
    ai_auto_summarize BOOLEAN DEFAULT FALSE,
    ai_tone VARCHAR(50) DEFAULT 'professional',

    -- Notification settings
    notify_new_email BOOLEAN DEFAULT TRUE,
    notify_important_only BOOLEAN DEFAULT FALSE,
    notify_calendar BOOLEAN DEFAULT TRUE,

    -- UI preferences
    theme VARCHAR(20) DEFAULT 'auto',
    language VARCHAR(10) DEFAULT 'en',
    timezone VARCHAR(50) DEFAULT 'UTC',

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- +migrate Down
DROP TABLE IF EXISTS user_settings;
