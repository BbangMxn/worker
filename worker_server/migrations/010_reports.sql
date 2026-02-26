-- +migrate Up

-- Email reports
CREATE TABLE IF NOT EXISTS email_reports (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    type VARCHAR(20) NOT NULL, -- daily, weekly, monthly
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,

    total_received INTEGER DEFAULT 0,
    total_sent INTEGER DEFAULT 0,
    total_read INTEGER DEFAULT 0,
    total_unread INTEGER DEFAULT 0,

    category_breakdown JSONB DEFAULT '{}',
    priority_breakdown JSONB DEFAULT '{}',
    top_senders JSONB DEFAULT '[]',

    avg_response_time_minutes FLOAT,

    ai_classified_count INTEGER DEFAULT 0,
    ai_replies_generated INTEGER DEFAULT 0,

    generated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_reports_user ON email_reports(user_id);
CREATE INDEX idx_reports_type ON email_reports(user_id, type);
CREATE INDEX idx_reports_period ON email_reports(period_start, period_end);

-- +migrate Down
DROP TABLE IF EXISTS email_reports;
