-- +migrate Up
-- ============================================================================
-- AI Status Column Migration
-- 목적: ai_status enum에 'pending' 값 추가 또는 VARCHAR로 변경
-- 날짜: 2026-01-15
-- ============================================================================

-- 방법 1: 기존 ai_status enum이 있으면 'pending' 값 추가
-- PostgreSQL에서 enum에 값 추가
DO $$
BEGIN
    -- Check if ai_status type exists and add 'pending' if not present
    IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ai_status') THEN
        -- Add 'pending' value to existing enum if not exists
        BEGIN
            ALTER TYPE ai_status ADD VALUE IF NOT EXISTS 'pending';
        EXCEPTION WHEN duplicate_object THEN
            NULL; -- Value already exists
        END;
        BEGIN
            ALTER TYPE ai_status ADD VALUE IF NOT EXISTS 'processing';
        EXCEPTION WHEN duplicate_object THEN
            NULL;
        END;
        BEGIN
            ALTER TYPE ai_status ADD VALUE IF NOT EXISTS 'completed';
        EXCEPTION WHEN duplicate_object THEN
            NULL;
        END;
        BEGIN
            ALTER TYPE ai_status ADD VALUE IF NOT EXISTS 'failed';
        EXCEPTION WHEN duplicate_object THEN
            NULL;
        END;
    END IF;
END $$;

-- 방법 2: enum이 없으면 컬럼 추가 (VARCHAR로)
-- emails 테이블
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'emails' AND column_name = 'ai_status') THEN
        ALTER TABLE emails ADD COLUMN ai_status VARCHAR(20) DEFAULT 'none' NOT NULL;
    END IF;
END $$;

-- email_threads 테이블
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'email_threads' AND column_name = 'ai_status') THEN
        ALTER TABLE email_threads ADD COLUMN ai_status VARCHAR(20) DEFAULT 'none' NOT NULL;
    END IF;
END $$;

-- 인덱스 (이미 존재하면 무시)
CREATE INDEX IF NOT EXISTS idx_emails_ai_status ON emails(ai_status);
CREATE INDEX IF NOT EXISTS idx_email_threads_ai_status ON email_threads(ai_status);

-- +migrate Down
-- Note: Cannot remove enum values in PostgreSQL, only drop the column
ALTER TABLE emails DROP COLUMN IF EXISTS ai_status;
ALTER TABLE email_threads DROP COLUMN IF EXISTS ai_status;
