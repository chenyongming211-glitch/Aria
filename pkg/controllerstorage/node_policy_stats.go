package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type NodePolicyStats struct {
	TenantID  uuid.UUID              `json:"tenant_id"`
	NodeID    uuid.UUID              `json:"node_id"`
	Stats     map[string]interface{} `json:"stats"`
	UpdatedAt time.Time              `json:"updated_at"`
}

func (s *NodePolicyStats) ACLRuleStats(ruleID uuid.UUID) map[string]interface{} {
	return nestedPolicyRuleStats(s, "acl_rules", ruleID)
}

func (s *NodePolicyStats) QoSRuleStats(ruleID uuid.UUID) map[string]interface{} {
	return nestedPolicyRuleStats(s, "qos_rules", ruleID)
}

func nestedPolicyRuleStats(stats *NodePolicyStats, domain string, ruleID uuid.UUID) map[string]interface{} {
	if stats == nil || stats.Stats == nil {
		return nil
	}
	raw, ok := stats.Stats[domain]
	if !ok {
		return nil
	}
	rules, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	value, ok := rules[ruleID.String()]
	if !ok {
		return nil
	}
	ruleStats, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return ruleStats
}

func (s *Storage) UpsertNodePolicyStats(tenantID, nodeID uuid.UUID, stats map[string]interface{}) error {
	if stats == nil {
		stats = map[string]interface{}{}
	}
	raw, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("marshal policy stats: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO node_policy_stats (tenant_id, node_id, stats, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (node_id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			stats = EXCLUDED.stats,
			updated_at = NOW()
	`, tenantID, nodeID, raw)
	return err
}

func (s *Storage) GetNodePolicyStats(tenantID, nodeID uuid.UUID) (*NodePolicyStats, error) {
	row := s.db.QueryRow(`
		SELECT tenant_id, node_id, stats, updated_at
		FROM node_policy_stats
		WHERE tenant_id = $1 AND node_id = $2
	`, tenantID, nodeID)

	var result NodePolicyStats
	var raw []byte
	if err := row.Scan(&result.TenantID, &result.NodeID, &raw, &result.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result.Stats); err != nil {
			return nil, fmt.Errorf("unmarshal policy stats: %w", err)
		}
	}
	if result.Stats == nil {
		result.Stats = map[string]interface{}{}
	}
	return &result, nil
}
