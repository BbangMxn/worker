-- contacts 테이블 스키마 업데이트: 도메인 모델과 일치시키기

-- 1. 컬럼명 변경
ALTER TABLE contacts RENAME COLUMN title TO job_title;
ALTER TABLE contacts RENAME COLUMN avatar_url TO photo_url;
ALTER TABLE contacts RENAME COLUMN email_count TO interaction_count;
ALTER TABLE contacts RENAME COLUMN last_email_at TO last_interaction_at;

-- 2. 새 컬럼 추가
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS department VARCHAR(255);
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS groups TEXT[];
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS relationship_score SMALLINT DEFAULT 0;
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS interaction_frequency VARCHAR(50);
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS last_contact_date TIMESTAMPTZ;
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS is_favorite BOOLEAN DEFAULT FALSE;
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS synced_at TIMESTAMPTZ;

-- 3. 인덱스 추가
CREATE INDEX IF NOT EXISTS idx_contacts_company ON contacts(company);
CREATE INDEX IF NOT EXISTS idx_contacts_is_favorite ON contacts(is_favorite);
CREATE INDEX IF NOT EXISTS idx_contacts_relationship_score ON contacts(relationship_score);
CREATE INDEX IF NOT EXISTS idx_contacts_last_interaction ON contacts(last_interaction_at);
