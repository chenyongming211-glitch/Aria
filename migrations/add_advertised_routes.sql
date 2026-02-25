-- Migration: Add advertised_routes column to nodes table
-- Date: 2026-02-08
-- Description: Support Site-to-Site VPN - allow nodes to advertise their local networks

-- Add advertised_routes column (TEXT[] array type)
ALTER TABLE nodes
ADD COLUMN IF NOT EXISTS advertised_routes TEXT[] DEFAULT '{}';

-- Add index for better query performance
CREATE INDEX IF NOT EXISTS idx_nodes_advertised_routes
ON nodes USING GIN (advertised_routes);

-- Add comment
COMMENT ON COLUMN nodes.advertised_routes IS 'Site-to-Site VPN: List of CIDR blocks advertised by this node (e.g., ["172.16.0.0/24", "10.0.0.0/16"])';
