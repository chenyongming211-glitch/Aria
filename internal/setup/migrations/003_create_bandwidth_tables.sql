-- Aria Bandwidth Management Schema
-- Migration: 003_create_bandwidth_tables.sql

-- ==========================================
-- 1. 带宽限制表
-- ==========================================
CREATE TABLE IF NOT EXISTS bandwidth_limits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) NOT NULL,
    src_ip INET,
    dst_ip INET,
    src_port INTEGER,
    dst_port INTEGER,
    protocol SMALLINT,
    bandwidth_mbps INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Index on bandwidth_limits for fast lookups
CREATE INDEX IF NOT EXISTS idx_bandwidth_limits_tenant_id ON bandwidth_limits(tenant_id);
CREATE INDEX IF NOT EXISTS idx_bandwidth_limits_src_ip ON bandwidth_limits(src_ip);
CREATE INDEX IF NOT EXISTS idx_bandwidth_limits_dst_ip ON bandwidth_limits(dst_ip);

-- ==========================================
-- 2. 策略规则表
-- ==========================================
CREATE TABLE IF NOT EXISTS policy_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT true,
    priority INTEGER DEFAULT 100,
    action VARCHAR(16) NOT NULL,  -- 'allow', 'deny', 'limit'

    -- Source
    src_ip INET,
    src_port INTEGER,
    src_region VARCHAR(50),

    -- Destination
    dst_ip INET,
    dst_port INTEGER,
    dst_region VARCHAR(50),

    -- Protocol
    protocol SMALLINT,
    protocol_name VARCHAR(32),

    -- Limit if action is 'limit'
    limit_bandwidth INTEGER,
    limit_type VARCHAR(16),  -- 'absolute', 'relative'

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Index on policy_rules for fast lookups
CREATE INDEX IF NOT EXISTS idx_policy_rules_tenant_id ON policy_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_policy_rules_enabled ON policy_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_policy_rules_priority ON policy_rules(priority);

-- Comments
COMMENT ON TABLE bandwidth_limits IS '带宽限制配置表';
COMMENT ON TABLE policy_rules IS '网络策略规则表';
