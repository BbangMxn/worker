-- Email Templates table
CREATE TABLE IF NOT EXISTS email_templates (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(50) NOT NULL DEFAULT 'custom',
    subject TEXT,
    body TEXT NOT NULL,
    html_body TEXT,
    variables JSONB DEFAULT '[]'::jsonb,
    tags TEXT[] DEFAULT '{}',
    is_default BOOLEAN DEFAULT false,
    is_archived BOOLEAN DEFAULT false,
    usage_count INTEGER DEFAULT 0,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_email_templates_user_id ON email_templates(user_id);
CREATE INDEX IF NOT EXISTS idx_email_templates_category ON email_templates(user_id, category);
CREATE INDEX IF NOT EXISTS idx_email_templates_is_default ON email_templates(user_id, is_default) WHERE is_default = true;
CREATE INDEX IF NOT EXISTS idx_email_templates_tags ON email_templates USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_email_templates_name_search ON email_templates USING GIN(to_tsvector('english', name || ' ' || COALESCE(body, '')));

-- Unique constraint: only one default template per category per user
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_templates_unique_default
ON email_templates(user_id, category)
WHERE is_default = true;

-- Enable RLS
ALTER TABLE email_templates ENABLE ROW LEVEL SECURITY;

-- RLS Policies
CREATE POLICY "Users can view own templates" ON email_templates
    FOR SELECT USING (auth.uid() = user_id);

CREATE POLICY "Users can create own templates" ON email_templates
    FOR INSERT WITH CHECK (auth.uid() = user_id);

CREATE POLICY "Users can update own templates" ON email_templates
    FOR UPDATE USING (auth.uid() = user_id);

CREATE POLICY "Users can delete own templates" ON email_templates
    FOR DELETE USING (auth.uid() = user_id);

-- Updated at trigger
CREATE OR REPLACE FUNCTION update_email_templates_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_email_templates_updated_at
    BEFORE UPDATE ON email_templates
    FOR EACH ROW
    EXECUTE FUNCTION update_email_templates_updated_at();

-- Comments
COMMENT ON TABLE email_templates IS 'User email templates for quick compose';
COMMENT ON COLUMN email_templates.category IS 'Template category: signature, reply, follow_up, intro, thank_you, meeting, custom';
COMMENT ON COLUMN email_templates.variables IS 'JSON array of template variables with name, placeholder, default_value, description';
COMMENT ON COLUMN email_templates.is_default IS 'Only one default template per category per user';
