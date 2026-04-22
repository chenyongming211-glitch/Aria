package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type NodeLifecycleTransition struct {
	TargetStatus   string
	RevokeReason   string
	AuditEventType string
	AuditActor     string
	AuditSummary   string
	AuditDetail    map[string]interface{}
}

func (s *Storage) ApplyNodeLifecycleTransition(publicKey string, transition NodeLifecycleTransition) (*Node, error) {
	targetStatus := strings.ToLower(strings.TrimSpace(transition.TargetStatus))
	if targetStatus == "" {
		return nil, fmt.Errorf("target status is required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(`SELECT `+nodeSelectColumns+` FROM nodes WHERE public_key = $1 FOR UPDATE`, publicKey)
	node, err := s.scanNode(row)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, sql.ErrNoRows
	}

	if _, err := tx.Exec(`
		UPDATE nodes
		SET status = $2, updated_at = NOW()
		WHERE public_key = $1
	`, publicKey, targetStatus); err != nil {
		return nil, err
	}
	node.Status = targetStatus

	if nodeStatusRequiresCertificateRevoke(targetStatus) {
		if _, err := tx.Exec(`
			UPDATE node_certificates
			SET status = $2,
			    revoked_at = NOW(),
			    revoke_reason = $3,
			    updated_at = NOW()
			WHERE node_id = $1
		`, node.ID, CertStatusRevoked, transition.RevokeReason); err != nil {
			return nil, err
		}
	}

	if transition.AuditEventType != "" {
		detail := map[string]interface{}{
			"public_key":    node.PublicKey,
			"hostname":      node.Hostname,
			"target_status": targetStatus,
		}
		for k, v := range transition.AuditDetail {
			detail[k] = v
		}
		detailJSON, err := json.Marshal(detail)
		if err != nil {
			return nil, fmt.Errorf("marshal audit detail: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, node.TenantID, node.ID, transition.AuditEventType, transition.AuditActor, transition.AuditSummary, detailJSON); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return node, nil
}

func nodeStatusRequiresCertificateRevoke(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "deleted", "suspended", "banned":
		return true
	default:
		return false
	}
}
