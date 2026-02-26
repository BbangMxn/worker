-- +migrate Up

-- User profiles for personalization
CREATE TABLE IF NOT EXISTS user_profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,

    tone_profile JSONB,
    writing_patterns JSONB,

    preferred_topics TEXT[],
    common_phrases TEXT[],
    signature_style TEXT,

    total_emails_analyzed INTEGER DEFAULT 0,
    last_analyzed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- +migrate Down
DROP TABLE IF EXISTS user_profiles;
