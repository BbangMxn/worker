-- Migration: Convert ai_priority from INTEGER to FLOAT (0.0 - 1.0)
-- Based on research from Gmail Priority Inbox, Superhuman, and SIGIR papers
--
-- Score ranges (Eisenhower Matrix inspired):
--   0.80 ~ 1.00: Urgent (requires immediate action)
--   0.60 ~ 0.79: High (important, should address soon)
--   0.40 ~ 0.59: Normal (relevant, worth reading)
--   0.20 ~ 0.39: Low (can be deferred)
--   0.00 ~ 0.19: Lowest (background noise)

-- Step 1: Drop old indexes
DROP INDEX IF EXISTS idx_emails_ai_priority;
DROP INDEX IF EXISTS idx_emails_high_priority;

-- Step 2: Change column type from INTEGER to NUMERIC(4,3)
-- Also migrate existing values: 1→0.10, 2→0.30, 3→0.50, 4→0.70, 5→0.90
ALTER TABLE emails
ALTER COLUMN ai_priority TYPE NUMERIC(4,3)
USING CASE
    WHEN ai_priority = 5 THEN 0.90
    WHEN ai_priority = 4 THEN 0.70
    WHEN ai_priority = 3 THEN 0.50
    WHEN ai_priority = 2 THEN 0.30
    WHEN ai_priority = 1 THEN 0.10
    ELSE 0.50
END;

-- Step 3: Create index for priority-based sorting
CREATE INDEX idx_emails_ai_priority ON emails(ai_priority DESC NULLS LAST);

-- Step 4: Create composite index for TODO list (workflow_status + priority + date)
CREATE INDEX IF NOT EXISTS idx_emails_todo_priority
ON emails (user_id, workflow_status, ai_priority DESC NULLS LAST, email_date DESC)
WHERE workflow_status = 'todo';

-- Step 5: Create index for high priority emails (>= 0.60)
CREATE INDEX idx_emails_high_priority
ON emails (user_id, ai_priority DESC)
WHERE ai_priority >= 0.60;

-- Add comment for documentation
COMMENT ON COLUMN emails.ai_priority IS 'Priority score from 0.0 to 1.0. Based on sender engagement, content signals, and AI classification. Higher = more important.';
