-- +migrate Up

-- =============================================================================
-- Score-Based Classification System
-- =============================================================================
--
-- 점수 기반 5-Stage 분류 파이프라인:
-- Stage 0: RFC Headers      (~55%)  → List-Unsubscribe, Precedence
-- Stage 1: Sender Profile   (~15%)  → 행동 기반 점수 (read/reply/delete)
-- Stage 2: User Rules       (~12%)  → 도메인/키워드/VIP
-- Stage 3: Semantic Cache   (~10%)  → Embedding 유사도 검색
-- Stage 4: LLM Fallback     (~8%)   → 최후 수단
--
-- 목표: LLM 비용 85-95% 절감
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 1. sender_profiles 테이블 확장
-- -----------------------------------------------------------------------------

-- delete_rate: 삭제 비율 (중요도 점수 계산용)
ALTER TABLE sender_profiles
ADD COLUMN IF NOT EXISTS delete_rate FLOAT DEFAULT 0;

-- is_contact: 연락처에 있는지 (보너스 점수)
ALTER TABLE sender_profiles
ADD COLUMN IF NOT EXISTS is_contact BOOLEAN DEFAULT FALSE;

-- interaction_count: 총 상호작용 수 (읽기+답장+클릭)
ALTER TABLE sender_profiles
ADD COLUMN IF NOT EXISTS interaction_count INT DEFAULT 0;

-- last_interacted_at: 마지막 상호작용 시간
ALTER TABLE sender_profiles
ADD COLUMN IF NOT EXISTS last_interacted_at TIMESTAMPTZ;

-- importance_score: 캐시된 중요도 점수 (0.0-1.0)
ALTER TABLE sender_profiles
ADD COLUMN IF NOT EXISTS importance_score FLOAT DEFAULT 0;

-- confirmed_labels: 사용자가 확정한 라벨 ID들
ALTER TABLE sender_profiles
ADD COLUMN IF NOT EXISTS confirmed_labels BIGINT[] DEFAULT '{}';

-- first_seen_at: 처음 본 시간 (RecencyBonus 계산용)
ALTER TABLE sender_profiles
ADD COLUMN IF NOT EXISTS first_seen_at TIMESTAMPTZ DEFAULT NOW();

-- 중요도 점수 인덱스 (높은 순으로 조회용)
CREATE INDEX IF NOT EXISTS idx_sender_profiles_importance
ON sender_profiles(user_id, importance_score DESC);

-- 연락처 인덱스
CREATE INDEX IF NOT EXISTS idx_sender_profiles_contact
ON sender_profiles(user_id, is_contact) WHERE is_contact = TRUE;

COMMENT ON COLUMN sender_profiles.delete_rate IS '삭제 비율 (0~1) - 해당 발신자 메일 중 삭제한 비율';
COMMENT ON COLUMN sender_profiles.is_contact IS '연락처에 등록된 발신자 여부';
COMMENT ON COLUMN sender_profiles.interaction_count IS '총 상호작용 횟수 (읽기+답장+클릭)';
COMMENT ON COLUMN sender_profiles.last_interacted_at IS '마지막 상호작용 시간';
COMMENT ON COLUMN sender_profiles.importance_score IS '계산된 중요도 점수 (0.0-1.0)';
COMMENT ON COLUMN sender_profiles.confirmed_labels IS '사용자가 확정한 라벨 ID 배열';

-- -----------------------------------------------------------------------------
-- 2. label_rules 테이블 (Auto Labeling용)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS label_rules (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    label_id BIGINT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,

    -- 규칙 타입
    type VARCHAR(50) NOT NULL,  -- exact_sender, sender_domain, subject_keyword, body_keyword, embedding, ai_prompt

    -- 매칭 패턴 (embedding의 경우 "ref:{email_id}" 형식)
    pattern TEXT NOT NULL,

    -- 적용 점수 (0.0-1.0)
    score FLOAT DEFAULT 0.90,

    -- 자동 학습으로 생성됨
    is_auto_created BOOLEAN DEFAULT FALSE,

    -- 활성화 여부
    is_active BOOLEAN DEFAULT TRUE,

    -- 사용 통계
    hit_count INT DEFAULT 0,
    last_hit_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT uq_label_rules UNIQUE (user_id, label_id, type, pattern)
);

CREATE INDEX idx_label_rules_user ON label_rules(user_id, is_active);
CREATE INDEX idx_label_rules_label ON label_rules(label_id);
CREATE INDEX idx_label_rules_type ON label_rules(user_id, type);

COMMENT ON TABLE label_rules IS '라벨 자동 적용 규칙 (Auto Labeling)';
COMMENT ON COLUMN label_rules.type IS 'exact_sender(0.99), sender_domain(0.95), subject_keyword(0.90), body_keyword(0.85), embedding, ai_prompt';
COMMENT ON COLUMN label_rules.pattern IS '매칭 패턴. embedding의 경우 "ref:{email_id}" 형식';
COMMENT ON COLUMN label_rules.is_auto_created IS '사용자가 라벨 추가 시 자동 학습으로 생성됨';

-- -----------------------------------------------------------------------------
-- 3. classification_cache 테이블 (Semantic Cache)
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS classification_cache (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,

    -- Embedding (pgvector)
    embedding vector(1536),

    -- 분류 결과
    category VARCHAR(50) NOT NULL,
    sub_category VARCHAR(50),
    priority VARCHAR(20) NOT NULL,
    labels BIGINT[] DEFAULT '{}',

    -- LLM 분류 시 신뢰도
    score FLOAT NOT NULL,

    -- 사용 통계
    usage_count INT DEFAULT 1,
    last_used_at TIMESTAMPTZ DEFAULT NOW(),

    -- 만료 시간 (30일)
    expires_at TIMESTAMPTZ DEFAULT NOW() + INTERVAL '30 days',

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- HNSW 인덱스 (cosine similarity, m=16, ef_construction=64)
CREATE INDEX IF NOT EXISTS idx_classification_cache_embedding
ON classification_cache USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

CREATE INDEX idx_classification_cache_user ON classification_cache(user_id);
CREATE INDEX idx_classification_cache_expires ON classification_cache(expires_at);

COMMENT ON TABLE classification_cache IS 'Embedding 기반 분류 결과 캐시 (Semantic Cache)';
COMMENT ON COLUMN classification_cache.embedding IS 'text-embedding-3-small 1536 dimensions';
COMMENT ON COLUMN classification_cache.usage_count IS '캐시 히트 횟수 (가중치 계산용)';
COMMENT ON COLUMN classification_cache.expires_at IS '30일 후 자동 만료';

-- -----------------------------------------------------------------------------
-- 4. classification_rules_v2 테이블 (점수 기반 규칙)
-- -----------------------------------------------------------------------------
-- 기존 classification_rules는 그대로 두고 새 테이블 생성
CREATE TABLE IF NOT EXISTS classification_rules_v2 (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,

    -- 규칙 타입
    type VARCHAR(50) NOT NULL,  -- exact_sender, sender_domain, subject_keyword, body_keyword, ai_prompt

    -- 매칭 패턴
    pattern TEXT NOT NULL,

    -- 액션
    action VARCHAR(50) NOT NULL,  -- assign_category, assign_priority, assign_label, mark_important, mark_spam

    -- 액션 값 (category 이름, priority 이름, label_id 등)
    value TEXT NOT NULL,

    -- 적용 점수 (0.0-1.0)
    score FLOAT DEFAULT 0.90,

    -- 같은 type 내 우선순위 (낮을수록 먼저)
    position INT DEFAULT 0,

    -- 활성화 여부
    is_active BOOLEAN DEFAULT TRUE,

    -- 사용 통계
    hit_count INT DEFAULT 0,
    last_hit_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT uq_classification_rules_v2 UNIQUE (user_id, type, pattern, action)
);

CREATE INDEX idx_classification_rules_v2_user ON classification_rules_v2(user_id, is_active, type);
CREATE INDEX idx_classification_rules_v2_action ON classification_rules_v2(user_id, action);

COMMENT ON TABLE classification_rules_v2 IS '점수 기반 분류 규칙 (Score-Based Classification)';
COMMENT ON COLUMN classification_rules_v2.type IS 'exact_sender(0.99), sender_domain(0.95), subject_keyword(0.90), body_keyword(0.85), ai_prompt(LLM)';
COMMENT ON COLUMN classification_rules_v2.action IS 'assign_category, assign_priority, assign_label, mark_important, mark_spam';
COMMENT ON COLUMN classification_rules_v2.score IS '기본 점수. exact_sender=0.99, sender_domain=0.95, subject_keyword=0.90, body_keyword=0.85';

-- -----------------------------------------------------------------------------
-- 5. emails 테이블에 분류 점수 컬럼 추가
-- -----------------------------------------------------------------------------

-- 분류 신뢰도 점수
ALTER TABLE emails
ADD COLUMN IF NOT EXISTS classification_score FLOAT;

-- 분류에 사용된 Stage
ALTER TABLE emails
ADD COLUMN IF NOT EXISTS classification_stage VARCHAR(20);

COMMENT ON COLUMN emails.classification_score IS '분류 신뢰도 점수 (0.0-1.0)';
COMMENT ON COLUMN emails.classification_stage IS '분류에 사용된 Stage (rfc, sender, rule, cache, llm)';

-- -----------------------------------------------------------------------------
-- 6. RLS 정책
-- -----------------------------------------------------------------------------

ALTER TABLE label_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE classification_cache ENABLE ROW LEVEL SECURITY;
ALTER TABLE classification_rules_v2 ENABLE ROW LEVEL SECURITY;

-- label_rules RLS
CREATE POLICY label_rules_select ON label_rules FOR SELECT USING (auth.uid() = user_id);
CREATE POLICY label_rules_insert ON label_rules FOR INSERT WITH CHECK (auth.uid() = user_id);
CREATE POLICY label_rules_update ON label_rules FOR UPDATE USING (auth.uid() = user_id);
CREATE POLICY label_rules_delete ON label_rules FOR DELETE USING (auth.uid() = user_id);

-- classification_cache RLS
CREATE POLICY classification_cache_select ON classification_cache FOR SELECT USING (auth.uid() = user_id);
CREATE POLICY classification_cache_insert ON classification_cache FOR INSERT WITH CHECK (auth.uid() = user_id);
CREATE POLICY classification_cache_update ON classification_cache FOR UPDATE USING (auth.uid() = user_id);
CREATE POLICY classification_cache_delete ON classification_cache FOR DELETE USING (auth.uid() = user_id);

-- classification_rules_v2 RLS
CREATE POLICY classification_rules_v2_select ON classification_rules_v2 FOR SELECT USING (auth.uid() = user_id);
CREATE POLICY classification_rules_v2_insert ON classification_rules_v2 FOR INSERT WITH CHECK (auth.uid() = user_id);
CREATE POLICY classification_rules_v2_update ON classification_rules_v2 FOR UPDATE USING (auth.uid() = user_id);
CREATE POLICY classification_rules_v2_delete ON classification_rules_v2 FOR DELETE USING (auth.uid() = user_id);

-- -----------------------------------------------------------------------------
-- 7. 만료된 캐시 자동 삭제 (pg_cron 또는 수동 실행)
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION cleanup_expired_classification_cache()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM classification_cache
    WHERE expires_at < NOW();

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_expired_classification_cache IS '만료된 분류 캐시 삭제 (매일 실행 권장)';

-- -----------------------------------------------------------------------------
-- 8. Sender Profile 중요도 점수 계산 함수
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION calculate_sender_importance_score(
    p_read_rate FLOAT,
    p_reply_rate FLOAT,
    p_delete_rate FLOAT,
    p_is_contact BOOLEAN,
    p_is_vip BOOLEAN,
    p_is_muted BOOLEAN,
    p_last_email_at TIMESTAMPTZ
)
RETURNS FLOAT AS $$
DECLARE
    engagement_score FLOAT;
    recency_bonus FLOAT;
    contact_bonus FLOAT;
    days_since_last FLOAT;
BEGIN
    -- VIP는 최고 점수
    IF p_is_vip THEN
        RETURN 0.98;
    END IF;

    -- Muted는 최저 점수
    IF p_is_muted THEN
        RETURN 0.10;
    END IF;

    -- Engagement 점수 (0-75점)
    engagement_score := (COALESCE(p_read_rate, 0) * 20) +
                        (COALESCE(p_reply_rate, 0) * 40) +
                        ((1 - COALESCE(p_delete_rate, 0)) * 15);

    -- 최근성 보너스 (0-15점)
    recency_bonus := 0;
    IF p_last_email_at IS NOT NULL THEN
        days_since_last := EXTRACT(EPOCH FROM (NOW() - p_last_email_at)) / 86400;
        IF days_since_last < 7 THEN
            recency_bonus := 15;
        ELSIF days_since_last < 30 THEN
            recency_bonus := 10;
        ELSIF days_since_last < 90 THEN
            recency_bonus := 5;
        END IF;
    END IF;

    -- 연락처 보너스 (0-10점)
    contact_bonus := CASE WHEN p_is_contact THEN 10 ELSE 0 END;

    -- 총점: 최대 100점 → 0.0-0.95 변환
    RETURN LEAST((engagement_score + recency_bonus + contact_bonus) / 100.0, 0.95);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION calculate_sender_importance_score IS 'Sender Profile 중요도 점수 계산 (0.0-0.98)';

-- -----------------------------------------------------------------------------
-- 9. Sender Profile 중요도 점수 자동 업데이트 트리거
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION update_sender_importance_score()
RETURNS TRIGGER AS $$
BEGIN
    NEW.importance_score := calculate_sender_importance_score(
        NEW.read_rate,
        NEW.reply_rate,
        NEW.delete_rate,
        NEW.is_contact,
        NEW.is_vip,
        NEW.is_muted,
        NEW.last_email_at
    );
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_sender_importance ON sender_profiles;
CREATE TRIGGER trigger_update_sender_importance
    BEFORE INSERT OR UPDATE OF read_rate, reply_rate, delete_rate, is_contact, is_vip, is_muted, last_email_at
    ON sender_profiles
    FOR EACH ROW
    EXECUTE FUNCTION update_sender_importance_score();

-- -----------------------------------------------------------------------------
-- 10. 기존 sender_profiles 중요도 점수 일괄 업데이트
-- -----------------------------------------------------------------------------
UPDATE sender_profiles
SET importance_score = calculate_sender_importance_score(
    read_rate,
    reply_rate,
    delete_rate,
    is_contact,
    is_vip,
    is_muted,
    last_email_at
);

-- +migrate Down

-- 트리거 삭제
DROP TRIGGER IF EXISTS trigger_update_sender_importance ON sender_profiles;
DROP FUNCTION IF EXISTS update_sender_importance_score();
DROP FUNCTION IF EXISTS calculate_sender_importance_score(FLOAT, FLOAT, FLOAT, BOOLEAN, BOOLEAN, BOOLEAN, TIMESTAMPTZ);
DROP FUNCTION IF EXISTS cleanup_expired_classification_cache();

-- RLS 정책 삭제
DROP POLICY IF EXISTS label_rules_select ON label_rules;
DROP POLICY IF EXISTS label_rules_insert ON label_rules;
DROP POLICY IF EXISTS label_rules_update ON label_rules;
DROP POLICY IF EXISTS label_rules_delete ON label_rules;

DROP POLICY IF EXISTS classification_cache_select ON classification_cache;
DROP POLICY IF EXISTS classification_cache_insert ON classification_cache;
DROP POLICY IF EXISTS classification_cache_update ON classification_cache;
DROP POLICY IF EXISTS classification_cache_delete ON classification_cache;

DROP POLICY IF EXISTS classification_rules_v2_select ON classification_rules_v2;
DROP POLICY IF EXISTS classification_rules_v2_insert ON classification_rules_v2;
DROP POLICY IF EXISTS classification_rules_v2_update ON classification_rules_v2;
DROP POLICY IF EXISTS classification_rules_v2_delete ON classification_rules_v2;

-- 테이블 삭제
DROP TABLE IF EXISTS classification_rules_v2;
DROP TABLE IF EXISTS classification_cache;
DROP TABLE IF EXISTS label_rules;

-- emails 컬럼 삭제
ALTER TABLE emails DROP COLUMN IF EXISTS classification_score;
ALTER TABLE emails DROP COLUMN IF EXISTS classification_stage;

-- sender_profiles 컬럼 삭제
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS delete_rate;
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS is_contact;
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS interaction_count;
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS last_interacted_at;
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS importance_score;
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS confirmed_labels;
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS first_seen_at;

-- 인덱스 삭제
DROP INDEX IF EXISTS idx_sender_profiles_importance;
DROP INDEX IF EXISTS idx_sender_profiles_contact;
