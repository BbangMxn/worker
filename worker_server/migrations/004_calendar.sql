-- +migrate Up

-- Calendars table
CREATE TABLE IF NOT EXISTS calendars (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES oauth_connections(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    provider_id VARCHAR(255) NOT NULL,

    name VARCHAR(255) NOT NULL,
    description TEXT,
    color VARCHAR(7),

    is_default BOOLEAN DEFAULT FALSE,
    is_read_only BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(connection_id, provider_id)
);

CREATE INDEX idx_calendars_user ON calendars(user_id);

-- Calendar events table
CREATE TABLE IF NOT EXISTS calendar_events (
    id BIGSERIAL PRIMARY KEY,
    calendar_id BIGINT NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id VARCHAR(255) NOT NULL,

    title VARCHAR(500) NOT NULL,
    description TEXT,
    location VARCHAR(500),

    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    is_all_day BOOLEAN DEFAULT FALSE,
    timezone VARCHAR(50) DEFAULT 'UTC',

    status VARCHAR(20) DEFAULT 'confirmed',
    organizer VARCHAR(255),
    attendees TEXT[],

    is_recurring BOOLEAN DEFAULT FALSE,
    recurrence_rule TEXT,

    reminders INTEGER[],
    meeting_url TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(calendar_id, provider_id)
);

CREATE INDEX idx_events_calendar ON calendar_events(calendar_id);
CREATE INDEX idx_events_user ON calendar_events(user_id);
CREATE INDEX idx_events_time ON calendar_events(start_time, end_time);

-- +migrate Down
DROP TABLE IF EXISTS calendar_events;
DROP TABLE IF EXISTS calendars;
