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
	defer rollbackTx(tx, "ApplyNodeLifecycleTransition")

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
		revokeReason := strings.TrimSpace(transition.RevokeReason)
		if revokeReason == "" {
			revokeReason = fmt.Sprintf("node %s", targetStatus)
		}
		revokedCount, err := revokeNodeCertificatesTx(tx, node.ID, revokeReason)
		if err != nil {
			return nil, err
		}
		if revokedCount > 0 {
			if err := createLifecycleCertificateRevokedAuditEventTx(tx, node, transition, targetStatus, revokeReason, revokedCount); err != nil {
				return nil, err
			}
		}
	}

	if nodeStatusStopsCommands(targetStatus) {
		if err := failIncompleteAgentCommandsForNodeTx(tx, publicKey, fmt.Sprintf("node status changed to %s", targetStatus)); err != nil {
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
	return nodeStatusStopsCommands(status)
}

func nodeStatusStopsCommands(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "deleted", "suspended", "banned":
		return true
	default:
		return false
	}
}

func createLifecycleCertificateRevokedAuditEventTx(
	tx roleExec,
	node *Node,
	transition NodeLifecycleTransition,
	targetStatus string,
	revokeReason string,
	revokedCount int64,
) error {
	actor := strings.TrimSpace(transition.AuditActor)
	if actor == "" {
		actor = "system"
	}
	detail := map[string]interface{}{
		"public_key":         node.PublicKey,
		"hostname":           node.Hostname,
		"node_status":        targetStatus,
		"reason":             revokeReason,
		"revoked_cert_count": revokedCount,
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal certificate revoked audit detail: %w", err)
	}
	_, err = tx.Exec(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, node.TenantID, node.ID, AuditCertRevoked, actor, "Node certificate revoked due to node lifecycle change", detailJSON)
	return err
}
