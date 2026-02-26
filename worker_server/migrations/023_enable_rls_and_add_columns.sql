-- +migrate Up

-- ============================================================================
-- 1. Add deleted_at to folders for soft delete
-- ============================================================================
ALTER TABLE folders
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_folders_deleted_at ON folders(deleted_at) WHERE deleted_at IS NULL;

COMMENT ON COLUMN folders.deleted_at IS 'Soft delete timestamp';

-- ============================================================================
-- 2. Enable RLS on email_attachments
-- ============================================================================
ALTER TABLE email_attachments ENABLE ROW LEVEL SECURITY;

CREATE POLICY email_attachments_select ON email_attachments FOR SELECT
USING (
    EXISTS (
        SELECT 1 FROM emails e
        WHERE e.id = email_attachments.email_id
        AND e.user_id = auth.uid()
    )
);

CREATE POLICY email_attachments_insert ON email_attachments FOR INSERT
WITH CHECK (
    EXISTS (
        SELECT 1 FROM emails e
        WHERE e.id = email_attachments.email_id
        AND e.user_id = auth.uid()
    )
);

CREATE POLICY email_attachments_delete ON email_attachments FOR DELETE
USING (
    EXISTS (
        SELECT 1 FROM emails e
        WHERE e.id = email_attachments.email_id
        AND e.user_id = auth.uid()
    )
);

-- ============================================================================
-- 3. Enable RLS on notifications
-- ============================================================================
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;

CREATE POLICY notifications_select ON notifications FOR SELECT
USING (user_id = auth.uid());

CREATE POLICY notifications_insert ON notifications FOR INSERT
WITH CHECK (user_id = auth.uid());

CREATE POLICY notifications_update ON notifications FOR UPDATE
USING (user_id = auth.uid());

CREATE POLICY notifications_delete ON notifications FOR DELETE
USING (user_id = auth.uid());

-- ============================================================================
-- 4. Enable RLS on known_domains (public read, service_role write)
-- ============================================================================
ALTER TABLE known_domains ENABLE ROW LEVEL SECURITY;

CREATE POLICY known_domains_select ON known_domains FOR SELECT
TO authenticated
USING (true);

CREATE POLICY known_domains_insert ON known_domains FOR INSERT
TO service_role
WITH CHECK (true);

CREATE POLICY known_domains_update ON known_domains FOR UPDATE
TO service_role
USING (true);

CREATE POLICY known_domains_delete ON known_domains FOR DELETE
TO service_role
USING (true);

-- +migrate Down

-- Drop RLS policies
DROP POLICY IF EXISTS email_attachments_select ON email_attachments;
DROP POLICY IF EXISTS email_attachments_insert ON email_attachments;
DROP POLICY IF EXISTS email_attachments_delete ON email_attachments;
ALTER TABLE email_attachments DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS notifications_select ON notifications;
DROP POLICY IF EXISTS notifications_insert ON notifications;
DROP POLICY IF EXISTS notifications_update ON notifications;
DROP POLICY IF EXISTS notifications_delete ON notifications;
ALTER TABLE notifications DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS known_domains_select ON known_domains;
DROP POLICY IF EXISTS known_domains_insert ON known_domains;
DROP POLICY IF EXISTS known_domains_update ON known_domains;
DROP POLICY IF EXISTS known_domains_delete ON known_domains;
ALTER TABLE known_domains DISABLE ROW LEVEL SECURITY;

DROP INDEX IF EXISTS idx_folders_deleted_at;
ALTER TABLE folders DROP COLUMN IF EXISTS deleted_at;
