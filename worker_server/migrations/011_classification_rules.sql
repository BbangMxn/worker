-- +migrate Up

-- Classification rules for personalized email classification
CREATE TABLE IF NOT EXISTS classification_rules (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Domain/Sender rules
    important_domains TEXT[] DEFAULT '{}',      -- "@company.com" → high priority
    important_keywords TEXT[] DEFAULT '{}',     -- "긴급", "마감" → high priority
    ignore_senders TEXT[] DEFAULT '{}',         -- "noreply@" → low priority
    ignore_keywords TEXT[] DEFAULT '{}',        -- "광고" → low priority

    -- Custom rules (natural language for LLM)
    high_priority_rules TEXT,                   -- "CEO 메일은 항상 긴급"
    low_priority_rules TEXT,                    -- "뉴스레터는 나중에"
    category_rules TEXT,                        -- "HR팀은 admin 카테고리"

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_classification_rules_user ON classification_rules(user_id);

-- +migrate Down
DROP TABLE IF EXISTS classification_rules;
