-- +migrate Up

-- Labels table
CREATE TABLE IF NOT EXISTS labels (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connection_id BIGINT REFERENCES oauth_connections(id) ON DELETE CASCADE,
    provider_id VARCHAR(255),

    name VARCHAR(255) NOT NULL,
    color VARCHAR(7),

    is_system BOOLEAN DEFAULT FALSE,
    is_visible BOOLEAN DEFAULT TRUE,

    email_count INTEGER DEFAULT 0,
    unread_count INTEGER DEFAULT 0,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_labels_user ON labels(user_id);
CREATE INDEX idx_labels_connection ON labels(connection_id);

-- Email-Label junction table
CREATE TABLE IF NOT EXISTS email_labels (
    email_id BIGINT NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    label_id BIGINT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    PRIMARY KEY (email_id, label_id)
);

-- +migrate Down
DROP TABLE IF EXISTS email_labels;
DROP TABLE IF EXISTS labels;
