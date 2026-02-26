-- Migration: sync_states Phase 1 enhancements
-- Adds checkpoint, retry, and phase columns for robust sync handling

-- =============================================================================
-- Phase column: tracks current sync phase
-- =============================================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'phase') THEN
        ALTER TABLE sync_states ADD COLUMN phase VARCHAR(30);
        COMMENT ON COLUMN sync_states.phase IS 'Current sync phase: initial_first_batch, initial_remaining, delta, gap, full_resync';
    END IF;
END $$;

-- =============================================================================
-- Retry columns: exponential backoff for failed syncs
-- =============================================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'retry_count') THEN
        ALTER TABLE sync_states ADD COLUMN retry_count INT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'max_retries') THEN
        ALTER TABLE sync_states ADD COLUMN max_retries INT DEFAULT 5;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'next_retry_at') THEN
        ALTER TABLE sync_states ADD COLUMN next_retry_at TIMESTAMP WITH TIME ZONE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'failed_at') THEN
        ALTER TABLE sync_states ADD COLUMN failed_at TIMESTAMP WITH TIME ZONE;
    END IF;
END $$;

-- =============================================================================
-- Checkpoint columns: resume interrupted syncs
-- =============================================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'checkpoint_page_token') THEN
        ALTER TABLE sync_states ADD COLUMN checkpoint_page_token TEXT;
        COMMENT ON COLUMN sync_states.checkpoint_page_token IS 'Gmail API page token for resuming interrupted sync';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'checkpoint_synced_count') THEN
        ALTER TABLE sync_states ADD COLUMN checkpoint_synced_count INT DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'checkpoint_total_count') THEN
        ALTER TABLE sync_states ADD COLUMN checkpoint_total_count INT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'checkpoint_updated_at') THEN
        ALTER TABLE sync_states ADD COLUMN checkpoint_updated_at TIMESTAMP WITH TIME ZONE;
    END IF;
END $$;

-- =============================================================================
-- First sync completion tracking
-- =============================================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'first_sync_completed_at') THEN
        ALTER TABLE sync_states ADD COLUMN first_sync_completed_at TIMESTAMP WITH TIME ZONE;
        COMMENT ON COLUMN sync_states.first_sync_completed_at IS 'When initial sync completed (enables delta sync)';
    END IF;
END $$;

-- =============================================================================
-- Performance tracking columns
-- =============================================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'avg_sync_duration_ms') THEN
        ALTER TABLE sync_states ADD COLUMN avg_sync_duration_ms INT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sync_states' AND column_name = 'last_sync_duration_ms') THEN
        ALTER TABLE sync_states ADD COLUMN last_sync_duration_ms INT;
    END IF;
END $$;

-- =============================================================================
-- Indexes for efficient queries
-- =============================================================================
CREATE INDEX IF NOT EXISTS idx_sync_states_retry_scheduled
    ON sync_states(next_retry_at)
    WHERE status = 'retry_scheduled' AND next_retry_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sync_states_checkpoint
    ON sync_states(checkpoint_updated_at)
    WHERE checkpoint_page_token IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sync_states_first_sync
    ON sync_states(first_sync_completed_at)
    WHERE first_sync_completed_at IS NOT NULL;

-- =============================================================================
-- Update status enum values comment
-- =============================================================================
COMMENT ON COLUMN sync_states.status IS 'Sync status: none, pending, syncing, idle, error, retry_scheduled, watch_expired';
