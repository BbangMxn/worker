-- +migrate Up

-- Add ai_score column if it doesn't exist
ALTER TABLE emails ADD COLUMN IF NOT EXISTS ai_score FLOAT;

-- Add classification_source column if it doesn't exist
ALTER TABLE emails ADD COLUMN IF NOT EXISTS classification_source VARCHAR(20);

-- +migrate Down

ALTER TABLE emails DROP COLUMN IF EXISTS ai_score;
ALTER TABLE emails DROP COLUMN IF EXISTS classification_source;
