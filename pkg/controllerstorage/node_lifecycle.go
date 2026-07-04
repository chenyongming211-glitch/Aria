package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type NodeLifecycleTransition struct {
	TargetStatus   string
	RevokeReason   string
	AuditEventType string
	AuditActor     string
	AuditSummary   string
	AuditDetail    map[string]interface{}
}

type TenantLifecycleTransition struct {
	TargetStatus   string
	AuditEventType string
	AuditActor     string
	AuditSummary   string
	AuditDetail    map[string]interface{}
}

type tenantLifecycleNode struct {
	ID        uuid.UUID
	PublicKey string
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

func (s *Storage) ApplyTenantLifecycleTransition(tenantID uuid.UUID, transition TenantLifecycleTransition) error {
	targetStatus := strings.ToLower(strings.TrimSpace(transition.TargetStatus))
	if targetStatus == "" {
		return fmt.Errorf("target status is required")
	}
	switch targetStatus {
	case "active", "suspended", "deleted":
	default:
		return fmt.Errorf("unsupported tenant status %q", targetStatus)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer rollbackTx(tx, "ApplyTenantLifecycleTransition")

	var previousStatus string
	if err := tx.QueryRow(`SELECT status FROM tenants WHERE id = $1 FOR UPDATE`, tenantID).Scan(&previousStatus); err != nil {
		return err
	}

	result, err := tx.Exec(`
		UPDATE tenants
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, tenantID, targetStatus)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	if tenantStatusStopsRuntime(targetStatus) {
		if _, err := tx.Exec(`
		UPDATE users
		SET token_version = COALESCE(token_version, 0) + 1,
		    updated_at = NOW()
		WHERE tenant_id = $1
	`, tenantID); err != nil {
			return err
		}

		nodes, err := selectActiveTenantNodesForLifecycleTx(tx, tenantID)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(`
		UPDATE nodes
		SET status = $2,
		    runtime_token_version = COALESCE(runtime_token_version, 0) + 1,
		    updated_at = NOW()
		WHERE tenant_id = $1
		  AND COALESCE(status, 'online') NOT IN ('deleted', 'suspended', 'banned')
	`, tenantID, targetStatus); err != nil {
			return err
		}

		revokeReason := fmt.Sprintf("tenant %s", targetStatus)
		commandMessage := fmt.Sprintf("tenant status changed to %s", targetStatus)
		for _, node := range nodes {
			if _, err := revokeNodeCertificatesTx(tx, node.ID, revokeReason); err != nil {
				return err
			}
			if err := failIncompleteAgentCommandsForNodeTx(tx, node.PublicKey, commandMessage); err != nil {
				return err
			}
		}
	}

	if transition.AuditEventType != "" {
		if err := createTenantLifecycleAuditEventTx(tx, tenantID, previousStatus, targetStatus, transition); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func selectActiveTenantNodesForLifecycleTx(tx *sql.Tx, tenantID uuid.UUID) ([]tenantLifecycleNode, error) {
	rows, err := tx.Query(`
		SELECT id, public_key
		FROM nodes
		WHERE tenant_id = $1
		  AND COALESCE(status, 'online') NOT IN ('deleted', 'suspended', 'banned')
		FOR UPDATE
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make([]tenantLifecycleNode, 0)
	for rows.Next() {
		var node tenantLifecycleNode
		if err := rows.Scan(&node.ID, &node.PublicKey); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func tenantStatusStopsRuntime(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "suspended", "deleted":
		return true
	default:
		return false
	}
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

func createTenantLifecycleAuditEventTx(
	tx roleExec,
	tenantID uuid.UUID,
	previousStatus string,
	targetStatus string,
	transition TenantLifecycleTransition,
) error {
	actor := strings.TrimSpace(transition.AuditActor)
	if actor == "" {
		actor = "system"
	}
	summary := strings.TrimSpace(transition.AuditSummary)
	if summary == "" {
		summary = fmt.Sprintf("Tenant %s", targetStatus)
	}
	detail := map[string]interface{}{
		"tenant_id":       tenantID.String(),
		"previous_status": strings.TrimSpace(previousStatus),
		"target_status":   targetStatus,
	}
	for k, v := range transition.AuditDetail {
		detail[k] = v
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal tenant lifecycle audit detail: %w", err)
	}
	_, err = tx.Exec(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, NULL, $2, $3, $4, $5)
	`, tenantID, transition.AuditEventType, actor, summary, detailJSON)
	return err
}
