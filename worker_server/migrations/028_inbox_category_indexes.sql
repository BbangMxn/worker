-- Inbox and Category view optimization indexes
-- These indexes optimize the new filtering system for Inbox (personal mail) and Category tabs

-- =============================================================================
-- INBOX VIEW INDEXES
-- Inbox shows only personal mail: ai_category IN ('primary', 'work', 'personal')
-- =============================================================================

-- 1. Main Inbox index: user + folder + category + date
-- Optimizes: GET /mail/inbox (most frequent query)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_inbox_view
ON emails(user_id, email_date DESC)
WHERE folder = 'inbox'
  AND ai_category IN ('primary', 'work', 'personal');

-- 2. Inbox with priority sorting (for important-first view)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_inbox_priority
ON emails(user_id, ai_priority DESC NULLS LAST, email_date DESC)
WHERE folder = 'inbox'
  AND ai_category IN ('primary', 'work', 'personal');

-- 3. Unread Inbox (badge count, unread-first sorting)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_inbox_unread
ON emails(user_id, email_date DESC)
WHERE folder = 'inbox'
  AND ai_category IN ('primary', 'work', 'personal')
  AND is_read = false;

-- =============================================================================
-- CATEGORY TAB INDEXES
-- Each category has its own tab: newsletter, notification, marketing, etc.
-- =============================================================================

-- 4. Category filter index (for category tabs)
-- Optimizes: GET /mail?category=newsletter
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_category_date
ON emails(user_id, ai_category, email_date DESC)
WHERE ai_category IS NOT NULL;

-- 5. SubCategory filter index (for sub-tabs like receipts, shipping)
-- Optimizes: GET /mail?sub_category=receipt
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_subcategory_date
ON emails(user_id, ai_sub_category, email_date DESC)
WHERE ai_sub_category IS NOT NULL;

-- =============================================================================
-- COMBINED FILTERS
-- =============================================================================

-- 6. Connection + Category (multi-account users)
-- Optimizes: GET /mail?connection_id=1&category=newsletter
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_connection_category
ON emails(user_id, connection_id, ai_category, email_date DESC)
WHERE ai_category IS NOT NULL;

-- 7. Folder + Category combined (for All Mail with category filter)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_folder_category
ON emails(user_id, folder, ai_category, email_date DESC);

-- =============================================================================
-- STATISTICS / COUNT INDEXES
-- =============================================================================

-- 8. Category count per user (for sidebar badges)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_category_count
ON emails(user_id, ai_category)
WHERE ai_category IS NOT NULL AND folder = 'inbox';

-- 9. Unread count per category
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_emails_category_unread_count
ON emails(user_id, ai_category)
WHERE ai_category IS NOT NULL AND is_read = false AND folder = 'inbox';
