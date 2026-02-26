-- Performance Optimization Indexes
-- This migration adds indexes to improve query performance for common operations.

-- =============================================================================
-- Email Table Indexes
-- =============================================================================

-- Composite index for email listing (user + folder + date)
-- Most common query: SELECT * FROM emails WHERE user_id = ? AND folder = ? ORDER BY email_date DESC
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_user_folder_date
ON emails (user_id, folder, email_date DESC);

-- Index for unread email count
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_user_unread
ON emails (user_id, is_read) WHERE is_read = false;

-- Index for starred emails
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_user_starred
ON emails (user_id) WHERE 'starred' = ANY(tags);

-- Index for AI pending emails
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_ai_pending
ON emails (user_id, ai_status) WHERE ai_status = 'pending';

-- Index for email search (full-text)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_search
ON emails USING gin(to_tsvector('english', subject || ' ' || snippet));

-- Index for thread lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_thread
ON emails (thread_id) WHERE thread_id IS NOT NULL;

-- Index for connection sync
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_connection_date
ON emails (connection_id, email_date DESC);

-- Index for external ID lookups (sync)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_external
ON emails (connection_id, external_id);

-- Index for priority filtering
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_priority
ON emails (user_id, ai_priority) WHERE ai_priority >= 4;

-- Index for category filtering
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_category
ON emails (user_id, ai_category) WHERE ai_category IS NOT NULL;

-- Index for workflow status (snoozed, etc.)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_workflow
ON emails (user_id, workflow_status, snooze_until)
WHERE workflow_status != 'none';

-- =============================================================================
-- Thread Table Indexes
-- =============================================================================

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_threads_user_latest
ON email_threads (user_id, latest_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_threads_user_unread
ON email_threads (user_id, has_unread) WHERE has_unread = true;

-- =============================================================================
-- Contact Table Indexes
-- =============================================================================

-- Index for contact lookup by email
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_contacts_user_email
ON contacts (user_id, email);

-- Index for VIP contacts
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_contacts_vip
ON contacts (user_id) WHERE is_vip = true;

-- Index for contact search
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_contacts_search
ON contacts USING gin(to_tsvector('english', name || ' ' || COALESCE(company, '')));

-- =============================================================================
-- OAuth Connections Indexes
-- =============================================================================

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_oauth_user_provider
ON oauth_connections (user_id, provider);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_oauth_token_expiry
ON oauth_connections (token_expires_at)
WHERE token_expires_at IS NOT NULL;

-- =============================================================================
-- Email Embeddings (Vector) Indexes
-- =============================================================================

-- IVFFlat index for faster vector search (requires pgvector)
-- Note: Run ANALYZE after bulk inserts for optimal performance
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_embeddings_vector
ON email_embeddings USING ivfflat (embedding vector_cosine_ops)
WITH (lists = 100);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_embeddings_user_direction
ON email_embeddings (user_id, direction);

-- =============================================================================
-- Sync States Indexes
-- =============================================================================

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_states_connection
ON sync_states (connection_id, sync_type);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_states_next_sync
ON sync_states (next_sync_at) WHERE status = 'idle';

-- =============================================================================
-- Classification Rules Indexes
-- =============================================================================

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rules_user_active
ON classification_rules (user_id, priority DESC)
WHERE is_active = true;

-- =============================================================================
-- Analyze Tables for Query Planner
-- =============================================================================

ANALYZE emails;
ANALYZE email_threads;
ANALYZE contacts;
ANALYZE oauth_connections;
ANALYZE email_embeddings;
