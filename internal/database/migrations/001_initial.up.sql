CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ══════════════════════════════════════════════════════════════
-- users
-- ══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'viewer'
                  CHECK (role IN ('admin', 'editor', 'viewer')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_lower ON users (lower(username));

-- ══════════════════════════════════════════════════════════════
-- refresh_tokens
-- ══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS refresh_tokens (
    token      TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires ON refresh_tokens(expires_at);

-- ══════════════════════════════════════════════════════════════
-- audit_entries
-- ══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS audit_entries (
    id             BIGSERIAL PRIMARY KEY,
    action         TEXT NOT NULL,
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_user_id  TEXT NOT NULL DEFAULT '',
    actor_username TEXT NOT NULL DEFAULT '',
    actor_role     TEXT NOT NULL DEFAULT '',
    actor_ip       TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT '',
    resource       TEXT NOT NULL DEFAULT '',
    before_data    JSONB,
    after_data     JSONB,
    error_text     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_audit_action      ON audit_entries(action);
CREATE INDEX IF NOT EXISTS idx_audit_actor       ON audit_entries(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_occurred    ON audit_entries(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_status      ON audit_entries(status);
CREATE INDEX IF NOT EXISTS idx_audit_resource    ON audit_entries USING gin (to_tsvector('simple', resource));

-- ══════════════════════════════════════════════════════════════
-- shares
-- ══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS shares (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token      UUID NOT NULL DEFAULT gen_random_uuid(),
    path       TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_shares_token ON shares(token);
CREATE INDEX IF NOT EXISTS idx_shares_created_by    ON shares(created_by);
CREATE INDEX IF NOT EXISTS idx_shares_expires       ON shares(expires_at);

-- ══════════════════════════════════════════════════════════════
-- trash_records
-- ══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS trash_records (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_path      TEXT NOT NULL,
    trash_name         TEXT NOT NULL,
    deleted_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_by_user_id TEXT NOT NULL DEFAULT '',
    deleted_by_username TEXT NOT NULL DEFAULT '',
    deleted_by_role    TEXT NOT NULL DEFAULT '',
    deleted_by_ip      TEXT NOT NULL DEFAULT '',
    restored_at        TIMESTAMPTZ,
    restored_by_user_id TEXT NOT NULL DEFAULT '',
    restored_by_username TEXT NOT NULL DEFAULT '',
    restored_by_role   TEXT NOT NULL DEFAULT '',
    restored_by_ip     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_trash_original_path ON trash_records(original_path);
CREATE INDEX IF NOT EXISTS idx_trash_deleted_at    ON trash_records(deleted_at DESC);
CREATE INDEX IF NOT EXISTS idx_trash_restored      ON trash_records(restored_at) WHERE restored_at IS NULL;

-- ══════════════════════════════════════════════════════════════
-- jobs & job_items
-- ══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation       TEXT NOT NULL CHECK (operation IN ('copy', 'move', 'delete')),
    status          TEXT NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued', 'running', 'completed', 'partial', 'failed')),
    conflict_policy TEXT NOT NULL DEFAULT '',
    total_items     INT NOT NULL DEFAULT 0,
    processed_items INT NOT NULL DEFAULT 0,
    success_items   INT NOT NULL DEFAULT 0,
    failed_items    INT NOT NULL DEFAULT 0,
    progress        INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_jobs_status     ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at DESC);

CREATE TABLE IF NOT EXISTS job_items (
    id     BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    source TEXT NOT NULL DEFAULT '',
    dest   TEXT NOT NULL DEFAULT '',
    path   TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_job_items_job_id ON job_items(job_id);
