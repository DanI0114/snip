CREATE TABLE IF NOT EXISTS short_links (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(16) NOT NULL UNIQUE,
    target_url  TEXT NOT NULL,
    clicks      BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_short_links_created_at
    ON short_links (created_at DESC);
