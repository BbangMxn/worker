-- +migrate Up

-- Add mapping_type column to label_provider_mappings
ALTER TABLE label_provider_mappings
ADD COLUMN IF NOT EXISTS mapping_type VARCHAR(20) DEFAULT 'label';

-- Add updated_at column if not exists
ALTER TABLE label_provider_mappings
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW();

COMMENT ON COLUMN label_provider_mappings.mapping_type IS 'label (Gmail) or category (Outlook)';

-- +migrate Down

ALTER TABLE label_provider_mappings DROP COLUMN IF EXISTS mapping_type;
ALTER TABLE label_provider_mappings DROP COLUMN IF EXISTS updated_at;
