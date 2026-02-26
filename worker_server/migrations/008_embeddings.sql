-- +migrate Up

-- Email embeddings for RAG
CREATE TABLE IF NOT EXISTS email_embeddings (
    id BIGSERIAL PRIMARY KEY,
    email_id BIGINT NOT NULL UNIQUE REFERENCES emails(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    direction VARCHAR(20) NOT NULL, -- inbound, outbound
    embedding vector(1536), -- OpenAI ada-002 dimension
    content TEXT,
    metadata JSONB DEFAULT '{}',

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for vector search
CREATE INDEX idx_embeddings_user ON email_embeddings(user_id);
CREATE INDEX idx_embeddings_direction ON email_embeddings(user_id, direction);
CREATE INDEX idx_embeddings_vector ON email_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- +migrate Down
DROP TABLE IF EXISTS email_embeddings;
