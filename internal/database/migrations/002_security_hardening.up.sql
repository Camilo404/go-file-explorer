-- ══════════════════════════════════════════════════════════════
-- Security hardening: force_password_change + account lockout
-- ══════════════════════════════════════════════════════════════

-- Flag to force password change on first login (seed admin).
ALTER TABLE users ADD COLUMN IF NOT EXISTS force_password_change BOOLEAN NOT NULL DEFAULT false;

-- Account lockout: track consecutive failed login attempts.
ALTER TABLE users ADD COLUMN IF NOT EXISTS failed_login_attempts INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;
