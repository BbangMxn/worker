-- +migrate Up

-- Contacts table
CREATE TABLE IF NOT EXISTS contacts (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    company VARCHAR(255),
    title VARCHAR(255),
    phone VARCHAR(50),
    avatar_url TEXT,
    notes TEXT,
    tags TEXT[],

    email_count INTEGER DEFAULT 0,
    last_email_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(user_id, email)
);

CREATE INDEX idx_contacts_user ON contacts(user_id);
CREATE INDEX idx_contacts_email ON contacts(email);

-- Companies table
CREATE TABLE IF NOT EXISTS companies (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255),
    industry VARCHAR(100),
    size VARCHAR(50),
    website TEXT,
    description TEXT,
    logo_url TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(user_id, domain)
);

CREATE INDEX idx_companies_user ON companies(user_id);
CREATE INDEX idx_companies_domain ON companies(domain);

-- +migrate Down
DROP TABLE IF EXISTS companies;
DROP TABLE IF EXISTS contacts;
