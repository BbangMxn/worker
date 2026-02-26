-- Migration: Add missing sub_category enum values
-- Version: 029
-- Description: Add developer, notification, alert to email_sub_category enum

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Add new enum values to email_sub_category
-- PostgreSQL requires ALTER TYPE ... ADD VALUE for enum extension
ALTER TYPE email_sub_category ADD VALUE IF NOT EXISTS 'notification';
ALTER TYPE email_sub_category ADD VALUE IF NOT EXISTS 'alert';
ALTER TYPE email_sub_category ADD VALUE IF NOT EXISTS 'developer';

-- ============================================================================
-- DOWN Migration (manual rollback - enum values cannot be easily removed)
-- ============================================================================
-- Note: PostgreSQL does not support removing enum values directly.
-- To rollback, you would need to:
-- 1. Create a new enum type without the values
-- 2. Update all columns to use the new type
-- 3. Drop the old type
-- 4. Rename the new type
--
-- This is generally not recommended in production.
-- Instead, unused enum values can simply be ignored.
