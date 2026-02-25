-- Aria Runtime Mode Schema Extension
-- Migration: 002_add_runtime_mode.sql
-- Description: Add runtime mode and capability detection fields to nodes table

-- Add runtime mode field for WireGuard operation mode
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS runtime_mode VARCHAR(16) DEFAULT 'kernel';

-- Add kernel version field
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS kernel_version VARCHAR(32);

-- Add AES-NI hardware acceleration support flag
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS has_aesni BOOLEAN DEFAULT false;

-- Index on runtime_mode for statistics and filtering
CREATE INDEX IF NOT EXISTS idx_nodes_runtime_mode ON nodes(runtime_mode);

-- Comments
COMMENT ON COLUMN nodes.runtime_mode IS 'WireGuard runtime mode: kernel or userspace';
COMMENT ON COLUMN nodes.kernel_version IS 'Node kernel version string';
COMMENT ON COLUMN nodes.has_aesni IS 'Whether CPU supports AES-NI hardware acceleration';
