-- 001_creative_review.up.sql
-- LLM 素材审核服务数据表

CREATE TABLE creative_reviews_ai (
    id              BIGSERIAL PRIMARY KEY,
    creative_id     BIGINT NOT NULL,
    provider        VARCHAR(30) NOT NULL,
    review_type     VARCHAR(20) NOT NULL CHECK (review_type IN ('text','image','video','landing','aggregate')),
    decision        VARCHAR(20) NOT NULL CHECK (decision IN ('pass','warn','reject','needs_human')),
    risk_score      DECIMAL(3,2),
    categories      TEXT[],
    reasons         TEXT[],
    confidence      DECIMAL(3,2),
    tokens_used     INT,
    cost_usd        DECIMAL(10,4),
    raw_response    JSONB,
    reviewed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_creative_rev ON creative_reviews_ai (creative_id, reviewed_at DESC);
CREATE INDEX idx_creative_rev_decision ON creative_reviews_ai (decision, reviewed_at DESC);

CREATE TABLE human_review_queue (
    id              BIGSERIAL PRIMARY KEY,
    creative_id     BIGINT NOT NULL,
    ai_review_id    BIGINT REFERENCES creative_reviews_ai(id),
    priority        INT NOT NULL DEFAULT 5,
    reason          TEXT,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','assigned','resolved')),
    assigned_to     BIGINT,
    resolved_at     TIMESTAMPTZ,
    decision        VARCHAR(20),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_human_queue_status ON human_review_queue (status, priority DESC, created_at ASC);
CREATE INDEX idx_human_queue_creative ON human_review_queue (creative_id);
