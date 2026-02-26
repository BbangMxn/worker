-- +migrate Up

-- Emails table
CREATE TABLE IF NOT EXISTS emails (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES oauth_connections(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    provider_id VARCHAR(255) NOT NULL,
    thread_id VARCHAR(255),

    -- Headers
    subject TEXT,
    from_email VARCHAR(255) NOT NULL,
    from_name VARCHAR(255),
    to_emails TEXT[], -- Array of emails
    cc_emails TEXT[],
    bcc_emails TEXT[],
    reply_to VARCHAR(255),
    date TIMESTAMPTZ NOT NULL,

    -- Folder & Labels
    folder VARCHAR(50) NOT NULL DEFAULT 'inbox',
    labels TEXT[],

    -- Flags
    is_read BOOLEAN DEFAULT FALSE,
    is_starred BOOLEAN DEFAULT FALSE,
    has_attachments BOOLEAN DEFAULT FALSE,

    -- AI Classification
    ai_category VARCHAR(50),
    ai_priority INTEGER,
    ai_summary TEXT,
    ai_tags TEXT[],
    ai_score FLOAT,

    -- Workflow
    workflow_status VARCHAR(50) DEFAULT 'todo',
    snoozed_until TIMESTAMPTZ,

    -- Timestamps
    received_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    UNIQUE(user_id, provider, provider_id)
);

-- Indexes
CREATE INDEX idx_emails_user ON emails(user_id);
CREATE INDEX idx_emails_connection ON emails(connection_id);
CREATE INDEX idx_emails_folder ON emails(user_id, folder);
CREATE INDEX idx_emails_thread ON emails(thread_id);
CREATE INDEX idx_emails_from ON emails(from_email);
CREATE INDEX idx_emails_date ON emails(date DESC);
CREATE INDEX idx_emails_unread ON emails(user_id, is_read) WHERE is_read = FALSE;
CREATE INDEX idx_emails_starred ON emails(user_id, is_starred) WHERE is_starred = TRUE;

-- +migrate Down
DROP TABLE IF EXISTS emails;
