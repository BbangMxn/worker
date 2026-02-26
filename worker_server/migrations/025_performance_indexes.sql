-- Performance optimization indexes
-- These indexes improve query performance for common operations

-- 1. Index for email resync by external_id with attachment filter
CREATE INDEX IF NOT EXISTS idx_emails_connection_external_id_attachment
ON emails(connection_id, external_id)
WHERE has_attachment = true;

-- 2. Index for inline attachments lookup by email_id
CREATE INDEX IF NOT EXISTS idx_email_attachments_email_id_inline
ON email_attachments(email_id)
WHERE is_inline = true;

-- 3. Index for regular attachments lookup by email_id
CREATE INDEX IF NOT EXISTS idx_email_attachments_email_id
ON email_attachments(email_id);

-- 4. Index for email labels junction table (cascade delete performance)
CREATE INDEX IF NOT EXISTS idx_email_labels_email_id
ON email_labels(email_id);

-- 5. Index for faster email lookup by thread_id
CREATE INDEX IF NOT EXISTS idx_emails_thread_id
ON emails(thread_id)
WHERE thread_id IS NOT NULL;

-- 6. Index for user's emails sorted by date (common query pattern)
CREATE INDEX IF NOT EXISTS idx_emails_user_date
ON emails(user_id, email_date DESC);

-- 7. Index for unread emails count
CREATE INDEX IF NOT EXISTS idx_emails_user_unread
ON emails(user_id)
WHERE is_read = false;

-- 8. Index for sender profiles importance score lookup
CREATE INDEX IF NOT EXISTS idx_sender_profiles_user_importance
ON sender_profiles(user_id, read_rate DESC);

-- 9. Index for emails by folder_id
CREATE INDEX IF NOT EXISTS idx_emails_folder_id
ON emails(folder_id)
WHERE folder_id IS NOT NULL;

-- 10. Index for email external_id lookup (for sync operations)
CREATE INDEX IF NOT EXISTS idx_emails_external_id
ON emails(external_id);
