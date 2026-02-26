-- +migrate Up

-- Classification rules table
CREATE TABLE IF NOT EXISTS classification_rules (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    name VARCHAR(255) NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    priority INTEGER DEFAULT 0,

    conditions JSONB NOT NULL DEFAULT '[]',
    actions JSONB NOT NULL DEFAULT '[]',

    match_count BIGINT DEFAULT 0,
    last_match_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_rules_user ON classification_rules(user_id);
CREATE INDEX idx_rules_active ON classification_rules(user_id, is_active) WHERE is_active = TRUE;

-- +migrate Down
DROP TABLE IF EXISTS classification_rules;
