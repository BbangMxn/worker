-- +migrate Up

-- Email attachments metadata table
-- 실제 파일은 Provider API에서 직접 다운로드 (저장하지 않음)
CREATE TABLE IF NOT EXISTS email_attachments (
    id BIGSERIAL PRIMARY KEY,
    email_id BIGINT NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    external_id VARCHAR(255) NOT NULL,  -- Provider attachment ID (Gmail/Outlook)

    -- Metadata
    filename VARCHAR(500) NOT NULL,
    mime_type VARCHAR(255) NOT NULL,
    size BIGINT NOT NULL DEFAULT 0,

    -- Inline attachment (CID for embedded images)
    content_id VARCHAR(255),
    is_inline BOOLEAN DEFAULT FALSE,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(email_id, external_id)
);

-- Indexes
CREATE INDEX idx_attachments_email ON email_attachments(email_id);
CREATE INDEX idx_attachments_filename ON email_attachments(filename);
CREATE INDEX idx_attachments_mime_type ON email_attachments(mime_type);

-- +migrate Down
DROP TABLE IF EXISTS email_attachments;
