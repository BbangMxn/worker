-- +migrate Up

-- =============================================================================
-- Folder / Label / Category 통합 시스템
-- =============================================================================
--
-- 개념 분리:
-- - Folder: 위치 (inbox, sent, trash, 사용자폴더) - 1개만
-- - Label: 주제/태그 (여행, 프로젝트A) - 여러 개 가능
-- - Category: 메일 타입 (receipt, newsletter) - AI 자동 분류
--
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 1. SubCategory enum 생성
-- -----------------------------------------------------------------------------
DO $$ BEGIN
    CREATE TYPE email_sub_category AS ENUM (
        -- Updates 하위
        'receipt',      -- 결제 영수증
        'invoice',      -- 청구서
        'shipping',     -- 배송
        'order',        -- 주문 확인
        'travel',       -- 여행 예약
        'calendar',     -- 캘린더 초대
        'account',      -- 계정 알림
        'security',     -- 보안 알림
        -- Social 하위
        'sns',          -- SNS 알림
        'comment',      -- 댓글/멘션
        -- Promotion 하위
        'newsletter',   -- 뉴스레터
        'marketing',    -- 마케팅
        'deal'          -- 할인/딜
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- ClassificationSource enum 생성
DO $$ BEGIN
    CREATE TYPE classification_source AS ENUM (
        'header',   -- 헤더 규칙 기반
        'domain',   -- 발신자 도메인 기반
        'llm',      -- LLM 분류
        'user'      -- 사용자 수동 지정
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- -----------------------------------------------------------------------------
-- 2. folders 테이블 (사용자 정의 폴더)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS folders (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,

    -- 폴더 정보
    name VARCHAR(100) NOT NULL,
    type VARCHAR(20) NOT NULL DEFAULT 'user',  -- 'system' | 'user'
    system_key VARCHAR(20),  -- inbox, sent, drafts, trash, spam, archive (system일 때만)

    -- 메타데이터 (Workspace 기준, Provider에 동기화 X)
    color VARCHAR(20),
    icon VARCHAR(50),
    position INT DEFAULT 0,

    -- 통계 (캐시, 주기적 업데이트)
    total_count INT DEFAULT 0,
    unread_count INT DEFAULT 0,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT uq_folders_user_name UNIQUE (user_id, name),
    CONSTRAINT uq_folders_user_system_key UNIQUE (user_id, system_key)
);

CREATE INDEX idx_folders_user ON folders(user_id);
CREATE INDEX idx_folders_system ON folders(user_id, type) WHERE type = 'system';

COMMENT ON TABLE folders IS '사용자 정의 폴더 (위치)';
COMMENT ON COLUMN folders.type IS 'system: 시스템 폴더, user: 사용자 생성 폴더';
COMMENT ON COLUMN folders.system_key IS '시스템 폴더 키 (inbox, sent, drafts, trash, spam, archive)';

-- -----------------------------------------------------------------------------
-- 3. folder_provider_mappings 테이블 (Provider별 폴더 매핑)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS folder_provider_mappings (
    id BIGSERIAL PRIMARY KEY,
    folder_id BIGINT NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES oauth_connections(id) ON DELETE CASCADE,

    -- Provider 매핑
    provider VARCHAR(20) NOT NULL,        -- 'google' | 'outlook'
    external_id VARCHAR(255),             -- Gmail Label ID / Outlook Folder ID
    mapping_type VARCHAR(20) NOT NULL,    -- 'label' | 'folder' | 'category'

    -- 동기화 상태
    synced_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT uq_folder_provider_mapping UNIQUE (folder_id, connection_id)
);

CREATE INDEX idx_folder_provider_mappings_folder ON folder_provider_mappings(folder_id);
CREATE INDEX idx_folder_provider_mappings_connection ON folder_provider_mappings(connection_id);
CREATE INDEX idx_folder_provider_mappings_external ON folder_provider_mappings(connection_id, external_id);

COMMENT ON TABLE folder_provider_mappings IS 'Gmail Label / Outlook Folder와 폴더 매핑';

-- -----------------------------------------------------------------------------
-- 4. label_provider_mappings 테이블 (Provider별 라벨 매핑)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS label_provider_mappings (
    id BIGSERIAL PRIMARY KEY,
    label_id BIGINT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES oauth_connections(id) ON DELETE CASCADE,

    -- Provider 매핑
    provider VARCHAR(20) NOT NULL,        -- 'google' | 'outlook'
    external_id VARCHAR(255),             -- Gmail Label ID / Outlook Category Name

    synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT uq_label_provider_mapping UNIQUE (label_id, connection_id)
);

CREATE INDEX idx_label_provider_mappings_label ON label_provider_mappings(label_id);
CREATE INDEX idx_label_provider_mappings_connection ON label_provider_mappings(connection_id);

COMMENT ON TABLE label_provider_mappings IS 'Gmail Label / Outlook Category와 라벨 매핑';

-- -----------------------------------------------------------------------------
-- 5. sender_profiles 테이블 (발신자 학습)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sender_profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,

    -- 발신자 정보
    email VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    name VARCHAR(255),

    -- 학습된 분류
    learned_category email_category,
    learned_sub_category email_sub_category,

    -- 사용자 설정
    is_vip BOOLEAN DEFAULT FALSE,
    is_muted BOOLEAN DEFAULT FALSE,

    -- 통계
    email_count INT DEFAULT 0,
    read_rate FLOAT DEFAULT 0,         -- 읽음 비율 (0~1)
    reply_rate FLOAT DEFAULT 0,        -- 답장 비율 (0~1)
    avg_reply_time_minutes INT,        -- 평균 답장 시간 (분)

    last_email_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT uq_sender_profiles_user_email UNIQUE (user_id, email)
);

CREATE INDEX idx_sender_profiles_user ON sender_profiles(user_id);
CREATE INDEX idx_sender_profiles_domain ON sender_profiles(user_id, domain);
CREATE INDEX idx_sender_profiles_vip ON sender_profiles(user_id, is_vip) WHERE is_vip = TRUE;
CREATE INDEX idx_sender_profiles_muted ON sender_profiles(user_id, is_muted) WHERE is_muted = TRUE;

COMMENT ON TABLE sender_profiles IS '발신자별 학습 데이터 및 사용자 설정';
COMMENT ON COLUMN sender_profiles.read_rate IS '읽음 비율 (0~1) - 해당 발신자 메일 중 읽은 비율';
COMMENT ON COLUMN sender_profiles.reply_rate IS '답장 비율 (0~1) - 해당 발신자 메일 중 답장한 비율';

-- -----------------------------------------------------------------------------
-- 6. smart_folders 테이블 (가상 폴더 - 검색 기반)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS smart_folders (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,

    -- 폴더 정보
    name VARCHAR(100) NOT NULL,
    icon VARCHAR(50),
    color VARCHAR(20),

    -- 쿼리 조건 (JSON)
    query JSONB NOT NULL DEFAULT '{}',

    -- 정렬
    sort_by VARCHAR(50) DEFAULT 'email_date',
    sort_order VARCHAR(10) DEFAULT 'desc',

    -- 메타데이터
    is_system BOOLEAN DEFAULT FALSE,
    is_visible BOOLEAN DEFAULT TRUE,
    position INT DEFAULT 0,

    -- 통계 (캐시, 주기적 업데이트)
    total_count INT DEFAULT 0,
    unread_count INT DEFAULT 0,
    last_calculated_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_smart_folders_user ON smart_folders(user_id);
CREATE INDEX idx_smart_folders_visible ON smart_folders(user_id, is_visible) WHERE is_visible = TRUE;
CREATE INDEX idx_smart_folders_system ON smart_folders(user_id, is_system) WHERE is_system = TRUE;

COMMENT ON TABLE smart_folders IS '검색 기반 가상 폴더 (Smart Folder)';
COMMENT ON COLUMN smart_folders.query IS 'JSON 쿼리 조건: {categories, sub_categories, labels, folders, is_read, is_starred, from_domains, date_range}';

-- -----------------------------------------------------------------------------
-- 7. known_domains 테이블 (자동 분류용 도메인 DB)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS known_domains (
    id SERIAL PRIMARY KEY,

    domain VARCHAR(255) NOT NULL UNIQUE,
    category email_category NOT NULL,
    sub_category email_sub_category,

    -- 메타데이터
    confidence FLOAT DEFAULT 1.0,
    source VARCHAR(20) DEFAULT 'system',  -- 'system' | 'community' | 'verified'

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_known_domains_domain ON known_domains(domain);
CREATE INDEX idx_known_domains_category ON known_domains(category);

COMMENT ON TABLE known_domains IS '자동 분류용 알려진 도메인 목록';

-- 초기 데이터 삽입
INSERT INTO known_domains (domain, category, sub_category, source) VALUES
-- 결제/영수증
('paypal.com', 'finance', 'receipt', 'system'),
('stripe.com', 'finance', 'receipt', 'system'),
('square.com', 'finance', 'receipt', 'system'),
('venmo.com', 'finance', 'receipt', 'system'),
('wise.com', 'finance', 'receipt', 'system'),
-- 배송
('fedex.com', 'shopping', 'shipping', 'system'),
('ups.com', 'shopping', 'shipping', 'system'),
('dhl.com', 'shopping', 'shipping', 'system'),
('usps.com', 'shopping', 'shipping', 'system'),
-- 쇼핑/주문
('amazon.com', 'shopping', 'order', 'system'),
('ebay.com', 'shopping', 'order', 'system'),
('etsy.com', 'shopping', 'order', 'system'),
('aliexpress.com', 'shopping', 'order', 'system'),
('coupang.com', 'shopping', 'order', 'system'),
-- 여행
('booking.com', 'travel', 'travel', 'system'),
('expedia.com', 'travel', 'travel', 'system'),
('airbnb.com', 'travel', 'travel', 'system'),
('tripadvisor.com', 'travel', 'travel', 'system'),
('agoda.com', 'travel', 'travel', 'system'),
('kayak.com', 'travel', 'travel', 'system'),
-- SNS
('facebookmail.com', 'social', 'sns', 'system'),
('linkedin.com', 'social', 'sns', 'system'),
('twitter.com', 'social', 'sns', 'system'),
('instagram.com', 'social', 'sns', 'system'),
('tiktok.com', 'social', 'sns', 'system'),
('youtube.com', 'social', 'sns', 'system'),
-- 마케팅/뉴스레터 플랫폼
('mailchimp.com', 'marketing', 'newsletter', 'system'),
('sendgrid.net', 'marketing', 'marketing', 'system'),
('constantcontact.com', 'marketing', 'newsletter', 'system'),
('hubspot.com', 'marketing', 'marketing', 'system'),
('substack.com', 'newsletter', 'newsletter', 'system')
ON CONFLICT (domain) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 8. emails 테이블 수정
-- -----------------------------------------------------------------------------
-- ai_sub_category 컬럼 추가
ALTER TABLE emails
ADD COLUMN IF NOT EXISTS ai_sub_category email_sub_category;

-- classification_source 컬럼 추가
ALTER TABLE emails
ADD COLUMN IF NOT EXISTS classification_source classification_source;

-- folder_id 컬럼 추가 (사용자 정의 폴더 참조)
ALTER TABLE emails
ADD COLUMN IF NOT EXISTS folder_id BIGINT REFERENCES folders(id) ON DELETE SET NULL;

-- 인덱스 추가
CREATE INDEX IF NOT EXISTS idx_emails_sub_category ON emails(user_id, ai_sub_category) WHERE ai_sub_category IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_emails_classification_source ON emails(classification_source);
CREATE INDEX IF NOT EXISTS idx_emails_folder_id ON emails(folder_id) WHERE folder_id IS NOT NULL;

COMMENT ON COLUMN emails.ai_sub_category IS '세부 분류 (receipt, shipping, travel 등) - AI 자동 분류';
COMMENT ON COLUMN emails.classification_source IS '분류 소스 (header, domain, llm, user)';
COMMENT ON COLUMN emails.folder_id IS '사용자 정의 폴더 참조';

-- -----------------------------------------------------------------------------
-- 9. labels 테이블 수정 (connection_id 옵셔널화 - 사용자 라벨 지원)
-- -----------------------------------------------------------------------------
-- labels.connection_id를 nullable로 변경 (이미 nullable이면 무시됨)
-- 사용자가 직접 만든 라벨은 connection_id가 NULL

-- position 컬럼 추가
ALTER TABLE labels
ADD COLUMN IF NOT EXISTS position INT DEFAULT 0;

COMMENT ON COLUMN labels.connection_id IS 'NULL이면 사용자 생성 라벨, 값이 있으면 Provider 동기화 라벨';
COMMENT ON COLUMN labels.position IS '라벨 정렬 순서';

-- -----------------------------------------------------------------------------
-- 10. 시스템 기본 폴더 생성 함수
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION create_default_folders_for_user(p_user_id UUID)
RETURNS void AS $$
BEGIN
    INSERT INTO folders (user_id, name, type, system_key, icon, position)
    VALUES
        (p_user_id, 'Inbox', 'system', 'inbox', 'inbox', 0),
        (p_user_id, 'Sent', 'system', 'sent', 'send', 1),
        (p_user_id, 'Drafts', 'system', 'drafts', 'file-text', 2),
        (p_user_id, 'Archive', 'system', 'archive', 'archive', 3),
        (p_user_id, 'Trash', 'system', 'trash', 'trash', 4),
        (p_user_id, 'Spam', 'system', 'spam', 'alert-circle', 5)
    ON CONFLICT (user_id, system_key) DO NOTHING;
END;
$$ LANGUAGE plpgsql;

-- -----------------------------------------------------------------------------
-- 11. 시스템 기본 Smart Folders 생성 함수
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION create_default_smart_folders_for_user(p_user_id UUID)
RETURNS void AS $$
BEGIN
    INSERT INTO smart_folders (user_id, name, icon, query, is_system, position)
    VALUES
        (p_user_id, '영수증', 'receipt', '{"sub_categories": ["receipt", "invoice"]}', TRUE, 0),
        (p_user_id, '배송', 'package', '{"sub_categories": ["shipping", "order"]}', TRUE, 1),
        (p_user_id, '여행', 'plane', '{"sub_categories": ["travel"]}', TRUE, 2),
        (p_user_id, '뉴스레터', 'newspaper', '{"sub_categories": ["newsletter"]}', TRUE, 3),
        (p_user_id, '이번 주', 'calendar', '{"date_range": "7_days"}', TRUE, 4),
        (p_user_id, '중요 미읽음', 'star', '{"is_read": false, "priorities": ["high", "urgent"]}', TRUE, 5)
    ON CONFLICT DO NOTHING;
END;
$$ LANGUAGE plpgsql;

-- -----------------------------------------------------------------------------
-- 12. 신규 사용자 트리거 (폴더 + Smart Folders 자동 생성)
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION handle_new_user_folders()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM create_default_folders_for_user(NEW.id);
    PERFORM create_default_smart_folders_for_user(NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- 기존 트리거 삭제 후 재생성
DROP TRIGGER IF EXISTS on_user_created_folders ON users;
CREATE TRIGGER on_user_created_folders
    AFTER INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION handle_new_user_folders();

-- -----------------------------------------------------------------------------
-- 13. 기존 사용자에게 기본 폴더 생성
-- -----------------------------------------------------------------------------
DO $$
DECLARE
    user_record RECORD;
BEGIN
    FOR user_record IN SELECT id FROM users LOOP
        PERFORM create_default_folders_for_user(user_record.id);
        PERFORM create_default_smart_folders_for_user(user_record.id);
    END LOOP;
END $$;

-- -----------------------------------------------------------------------------
-- 14. RLS 정책
-- -----------------------------------------------------------------------------
ALTER TABLE folders ENABLE ROW LEVEL SECURITY;
ALTER TABLE folder_provider_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE label_provider_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE sender_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE smart_folders ENABLE ROW LEVEL SECURITY;

-- folders RLS
CREATE POLICY folders_select ON folders FOR SELECT USING (auth.uid() = user_id);
CREATE POLICY folders_insert ON folders FOR INSERT WITH CHECK (auth.uid() = user_id);
CREATE POLICY folders_update ON folders FOR UPDATE USING (auth.uid() = user_id);
CREATE POLICY folders_delete ON folders FOR DELETE USING (auth.uid() = user_id AND type = 'user');

-- folder_provider_mappings RLS
CREATE POLICY folder_provider_mappings_select ON folder_provider_mappings FOR SELECT
    USING (EXISTS (SELECT 1 FROM folders WHERE folders.id = folder_id AND folders.user_id = auth.uid()));
CREATE POLICY folder_provider_mappings_insert ON folder_provider_mappings FOR INSERT
    WITH CHECK (EXISTS (SELECT 1 FROM folders WHERE folders.id = folder_id AND folders.user_id = auth.uid()));
CREATE POLICY folder_provider_mappings_update ON folder_provider_mappings FOR UPDATE
    USING (EXISTS (SELECT 1 FROM folders WHERE folders.id = folder_id AND folders.user_id = auth.uid()));
CREATE POLICY folder_provider_mappings_delete ON folder_provider_mappings FOR DELETE
    USING (EXISTS (SELECT 1 FROM folders WHERE folders.id = folder_id AND folders.user_id = auth.uid()));

-- label_provider_mappings RLS
CREATE POLICY label_provider_mappings_select ON label_provider_mappings FOR SELECT
    USING (EXISTS (SELECT 1 FROM labels WHERE labels.id = label_id AND labels.user_id = auth.uid()));
CREATE POLICY label_provider_mappings_insert ON label_provider_mappings FOR INSERT
    WITH CHECK (EXISTS (SELECT 1 FROM labels WHERE labels.id = label_id AND labels.user_id = auth.uid()));
CREATE POLICY label_provider_mappings_update ON label_provider_mappings FOR UPDATE
    USING (EXISTS (SELECT 1 FROM labels WHERE labels.id = label_id AND labels.user_id = auth.uid()));
CREATE POLICY label_provider_mappings_delete ON label_provider_mappings FOR DELETE
    USING (EXISTS (SELECT 1 FROM labels WHERE labels.id = label_id AND labels.user_id = auth.uid()));

-- sender_profiles RLS
CREATE POLICY sender_profiles_select ON sender_profiles FOR SELECT USING (auth.uid() = user_id);
CREATE POLICY sender_profiles_insert ON sender_profiles FOR INSERT WITH CHECK (auth.uid() = user_id);
CREATE POLICY sender_profiles_update ON sender_profiles FOR UPDATE USING (auth.uid() = user_id);
CREATE POLICY sender_profiles_delete ON sender_profiles FOR DELETE USING (auth.uid() = user_id);

-- smart_folders RLS
CREATE POLICY smart_folders_select ON smart_folders FOR SELECT USING (auth.uid() = user_id);
CREATE POLICY smart_folders_insert ON smart_folders FOR INSERT WITH CHECK (auth.uid() = user_id);
CREATE POLICY smart_folders_update ON smart_folders FOR UPDATE USING (auth.uid() = user_id);
CREATE POLICY smart_folders_delete ON smart_folders FOR DELETE USING (auth.uid() = user_id AND is_system = FALSE);

-- +migrate Down

-- RLS 정책 삭제
DROP POLICY IF EXISTS folders_select ON folders;
DROP POLICY IF EXISTS folders_insert ON folders;
DROP POLICY IF EXISTS folders_update ON folders;
DROP POLICY IF EXISTS folders_delete ON folders;

DROP POLICY IF EXISTS folder_provider_mappings_select ON folder_provider_mappings;
DROP POLICY IF EXISTS folder_provider_mappings_insert ON folder_provider_mappings;
DROP POLICY IF EXISTS folder_provider_mappings_update ON folder_provider_mappings;
DROP POLICY IF EXISTS folder_provider_mappings_delete ON folder_provider_mappings;

DROP POLICY IF EXISTS label_provider_mappings_select ON label_provider_mappings;
DROP POLICY IF EXISTS label_provider_mappings_insert ON label_provider_mappings;
DROP POLICY IF EXISTS label_provider_mappings_update ON label_provider_mappings;
DROP POLICY IF EXISTS label_provider_mappings_delete ON label_provider_mappings;

DROP POLICY IF EXISTS sender_profiles_select ON sender_profiles;
DROP POLICY IF EXISTS sender_profiles_insert ON sender_profiles;
DROP POLICY IF EXISTS sender_profiles_update ON sender_profiles;
DROP POLICY IF EXISTS sender_profiles_delete ON sender_profiles;

DROP POLICY IF EXISTS smart_folders_select ON smart_folders;
DROP POLICY IF EXISTS smart_folders_insert ON smart_folders;
DROP POLICY IF EXISTS smart_folders_update ON smart_folders;
DROP POLICY IF EXISTS smart_folders_delete ON smart_folders;

-- 트리거 삭제
DROP TRIGGER IF EXISTS on_user_created_folders ON users;
DROP FUNCTION IF EXISTS handle_new_user_folders();

-- 함수 삭제
DROP FUNCTION IF EXISTS create_default_folders_for_user(UUID);
DROP FUNCTION IF EXISTS create_default_smart_folders_for_user(UUID);

-- emails 컬럼 삭제
ALTER TABLE emails DROP COLUMN IF EXISTS ai_sub_category;
ALTER TABLE emails DROP COLUMN IF EXISTS classification_source;
ALTER TABLE emails DROP COLUMN IF EXISTS folder_id;

-- labels 컬럼 삭제
ALTER TABLE labels DROP COLUMN IF EXISTS position;

-- 인덱스 삭제
DROP INDEX IF EXISTS idx_emails_sub_category;
DROP INDEX IF EXISTS idx_emails_classification_source;
DROP INDEX IF EXISTS idx_emails_folder_id;

-- 테이블 삭제
DROP TABLE IF EXISTS known_domains;
DROP TABLE IF EXISTS smart_folders;
DROP TABLE IF EXISTS sender_profiles;
DROP TABLE IF EXISTS label_provider_mappings;
DROP TABLE IF EXISTS folder_provider_mappings;
DROP TABLE IF EXISTS folders;

-- enum 삭제
DROP TYPE IF EXISTS classification_source;
DROP TYPE IF EXISTS email_sub_category;
