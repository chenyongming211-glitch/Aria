-- Aria Token Management Schema
-- Migration: 001_create_tokens.sql

-- Create tokens table for enrollment token management
CREATE TABLE IF NOT EXISTS tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token       VARCHAR(64) UNIQUE NOT NULL,    -- tk_xxxxxxxx format
    tag         VARCHAR(128),                   -- description tag, e.g., "shanghai-office"
    max_uses    INTEGER NOT NULL DEFAULT 1,     -- maximum usage count
    used_count  INTEGER NOT NULL DEFAULT 0,     -- current usage count
    expires_at  TIMESTAMP NOT NULL,             -- expiration time
    created_at  TIMESTAMP DEFAULT NOW(),        -- creation time
    created_by  VARCHAR(64),                    -- creator identifier
    status      VARCHAR(16) DEFAULT 'active',   -- active, exhausted, revoked, expired
    last_used_at TIMESTAMP,                     -- last usage time
    last_used_by VARCHAR(64)                    -- last user device ID (public key)
);

-- Index on token for fast lookups during registration
CREATE INDEX IF NOT EXISTS idx_tokens_token ON tokens(token);

-- Index on status for filtering
CREATE INDEX IF NOT EXISTS idx_tokens_status ON tokens(status);

-- Index on expires_at for cleanup queries
CREATE INDEX IF NOT EXISTS idx_tokens_expires_at ON tokens(expires_at);

-- Comments
COMMENT ON TABLE tokens IS 'Enrollment tokens for agent registration';
COMMENT ON COLUMN tokens.token IS 'Token string in tk_xxxxxxxx format';
COMMENT ON COLUMN tokens.tag IS 'Human-readable description tag';
COMMENT ON COLUMN tokens.max_uses IS 'Maximum number of times this token can be used';
COMMENT ON COLUMN tokens.used_count IS 'Number of times this token has been used';
COMMENT ON COLUMN tokens.expires_at IS 'Token expiration timestamp';
COMMENT ON COLUMN tokens.status IS 'Token status: active, exhausted, revoked, expired';
