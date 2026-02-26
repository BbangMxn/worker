-- +migrate Up

-- Notifications table
CREATE TABLE IF NOT EXISTS notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Notification type
    type VARCHAR(50) NOT NULL,  -- email_received, email_classified, calendar_event, system

    -- Content
    title VARCHAR(255) NOT NULL,
    body TEXT,

    -- Related entity
    entity_type VARCHAR(50),  -- email, calendar_event, etc.
    entity_id BIGINT,

    -- Status
    is_read BOOLEAN DEFAULT FALSE,
    read_at TIMESTAMPTZ,

    -- Priority
    priority VARCHAR(20) DEFAULT 'normal',  -- low, normal, high, urgent

    -- Metadata
    metadata JSONB DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

-- Indexes
CREATE INDEX idx_notifications_user_id ON notifications(user_id);
CREATE INDEX idx_notifications_user_unread ON notifications(user_id, is_read) WHERE is_read = FALSE;
CREATE INDEX idx_notifications_created_at ON notifications(created_at DESC);
CREATE INDEX idx_notifications_type ON notifications(type);

-- Notification settings table
CREATE TABLE IF NOT EXISTS notification_settings (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,

    -- Email notifications
    email_new_mail BOOLEAN DEFAULT TRUE,
    email_important_only BOOLEAN DEFAULT FALSE,
    email_digest BOOLEAN DEFAULT FALSE,
    email_digest_frequency VARCHAR(20) DEFAULT 'daily',  -- daily, weekly

    -- Push notifications
    push_enabled BOOLEAN DEFAULT TRUE,
    push_new_mail BOOLEAN DEFAULT TRUE,
    push_calendar BOOLEAN DEFAULT TRUE,
    push_mentions BOOLEAN DEFAULT TRUE,

    -- In-app notifications
    inapp_enabled BOOLEAN DEFAULT TRUE,

    -- Quiet hours
    quiet_hours_enabled BOOLEAN DEFAULT FALSE,
    quiet_hours_start TIME DEFAULT '22:00',
    quiet_hours_end TIME DEFAULT '08:00',
    quiet_hours_timezone VARCHAR(50) DEFAULT 'UTC',

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Trigger to create default notification settings for new users
CREATE OR REPLACE FUNCTION create_default_notification_settings()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO notification_settings (user_id)
    VALUES (NEW.id)
    ON CONFLICT (user_id) DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_create_notification_settings
    AFTER INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION create_default_notification_settings();

-- Function to clean up old read notifications (30 days)
CREATE OR REPLACE FUNCTION cleanup_old_notifications()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM notifications
    WHERE is_read = TRUE
      AND read_at < NOW() - INTERVAL '30 days';

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- +migrate Down
DROP TRIGGER IF EXISTS trigger_create_notification_settings ON users;
DROP FUNCTION IF EXISTS create_default_notification_settings();
DROP FUNCTION IF EXISTS cleanup_old_notifications();
DROP TABLE IF EXISTS notification_settings;
DROP TABLE IF EXISTS notifications;
