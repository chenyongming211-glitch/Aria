package controllerstorage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// CountNodesByTenantAndStatus returns total, online, and offline node counts for a tenant.
// Online = last_seen within 60 seconds of now. Offline = last_seen older than 60 seconds.
func (s *Storage) CountNodesByTenantAndStatus(tenantID uuid.UUID) (total, online, offline int, err error) {
	err = s.db.QueryRow(`
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (
					WHERE COALESCE(status, 'online') NOT IN ('deleted', 'suspended', 'banned')
					  AND last_seen >= EXTRACT(EPOCH FROM NOW()) - 60
				) AS online,
				COUNT(*) FILTER (
					WHERE COALESCE(status, 'online') IN ('suspended', 'banned')
					   OR last_seen < EXTRACT(EPOCH FROM NOW()) - 60
					   OR last_seen IS NULL
				) AS offline
			FROM nodes
			WHERE tenant_id = $1 AND COALESCE(status, 'online') != 'deleted'
		`, tenantID).Scan(&total, &online, &offline)
	return
}

// CalcSyncSuccessRate computes the sync success rate for a tenant's nodes.
// Returns (synced / total_with_desired) * 100, or 100 if no nodes have a desired version.
func (s *Storage) CalcSyncSuccessRate(tenantID uuid.UUID) (float64, error) {
	var synced, totalDesired int
	err := s.db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE ncs.desired_state_version != '' AND ncs.desired_state_version = ncs.applied_state_version) AS synced,
			COUNT(*) FILTER (WHERE ncs.desired_state_version != '') AS total
		FROM node_control_states ncs
		JOIN nodes n ON n.id = ncs.node_id
		WHERE n.tenant_id = $1
	`, tenantID).Scan(&synced, &totalDesired)
	if err != nil {
		return 100, err
	}
	if totalDesired == 0 {
		return 100, nil
	}
	return float64(synced) * 100.0 / float64(totalDesired), nil
}

// CountACLRulesByTenant returns the number of ACL rules for a tenant.
func (s *Storage) CountACLRulesByTenant(tenantID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM acl_rules WHERE tenant_id = $1`, tenantID).Scan(&count)
	return count, err
}

// CountQoSRulesByTenant returns the number of QoS rules for a tenant.
func (s *Storage) CountQoSRulesByTenant(tenantID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM qos_rules WHERE tenant_id = $1`, tenantID).Scan(&count)
	return count, err
}

// CountFailedCommandsByTenant returns the number of failed agent commands for a tenant.
func (s *Storage) CountFailedCommandsByTenant(tenantID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM agent_commands ac
		JOIN nodes n ON n.public_key = ac.node_public_key
		WHERE n.tenant_id = $1 AND ac.status = 'failed'
	`, tenantID).Scan(&count)
	return count, err
}

// GetAlertByID retrieves a single alert by its ID.
func (s *Storage) GetAlertByID(alertID uuid.UUID) (*Alert, error) {
	row := s.db.QueryRow(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE id = $1
	`, alertID)
	return scanAlertRow(row)
}

// ListRecentPolicyDeliveriesByNode returns the most recent policy deliveries for a node.
func (s *Storage) ListRecentPolicyDeliveriesByNode(tenantID, nodeID uuid.UUID, limit int) ([]*PolicyDelivery, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
		SELECT id, tenant_id, node_id, policy_domain, policy_ref, COALESCE(policy_name, ''), action, command_id, command_status,
		       COALESCE(last_error, ''), metadata, created_at, updated_at, completed_at
		FROM policy_deliveries
		WHERE tenant_id = $1 AND node_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, tenantID, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deliveries := make([]*PolicyDelivery, 0, limit)
	for rows.Next() {
		delivery, err := scanPolicyDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Storage) GetLatestNodeCertificateActivity(tenantID, nodeID uuid.UUID) (map[string]*AuditEvent, error) {
	eventTypes := []string{"certificate_renewed", "certificate_renew_failed", AuditCertRevoked}
	rows, err := s.db.Query(`
		SELECT DISTINCT ON (event_type) id, tenant_id, node_id, event_type, actor, summary, detail, created_at
		FROM audit_events
		WHERE tenant_id = $1 AND node_id = $2 AND event_type = ANY($3)
		ORDER BY event_type, created_at DESC
	`, tenantID, nodeID, pq.Array(eventTypes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make(map[string]*AuditEvent, len(eventTypes))
	for rows.Next() {
		event, err := scanAuditEventRow(rows)
		if err != nil {
			return nil, err
		}
		events[event.EventType] = event
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// EventFeedFilter defines filtering options for the unified event feed.
type EventFeedFilter struct {
	NodeID    *uuid.UUID
	EventType string
	Severity  string
	Since     *time.Time
	Limit     int
	Offset    int
}

// EventFeedItem represents a unified event from either alerts or audit_events.
type EventFeedItem struct {
	ID        string                 `json:"id"`
	Source    string                 `json:"source"`
	EventType string                 `json:"event_type"`
	Severity  string                 `json:"severity"`
	NodeID    string                 `json:"node_id"`
	Title     string                 `json:"title"`
	Detail    map[string]interface{} `json:"detail"`
	CreatedAt time.Time              `json:"created_at"`
}

// ListEventFeed queries alerts and audit_events with UNION ALL, returning a merged event feed.
func (s *Storage) ListEventFeed(tenantID uuid.UUID, filter EventFeedFilter) ([]EventFeedItem, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	// Build alert conditions
	alertConds := []string{"tenant_id = $1"}
	auditConds := []string{"tenant_id = $1"}
	args := []interface{}{tenantID}
	argIdx := 2

	if filter.NodeID != nil {
		alertConds = append(alertConds, fmt.Sprintf("node_id = $%d", argIdx))
		auditConds = append(auditConds, fmt.Sprintf("node_id = $%d", argIdx))
		args = append(args, *filter.NodeID)
		argIdx++
	}
	if filter.EventType != "" {
		alertConds = append(alertConds, fmt.Sprintf("alert_type = $%d", argIdx))
		auditConds = append(auditConds, fmt.Sprintf("event_type = $%d", argIdx))
		args = append(args, filter.EventType)
		argIdx++
	}
	if filter.Severity != "" {
		alertConds = append(alertConds, fmt.Sprintf("severity = $%d", argIdx))
		// Audit events don't have severity, exclude them when filtering by severity
		auditConds = append(auditConds, "FALSE")
		args = append(args, filter.Severity)
		argIdx++
	}
	if filter.Since != nil {
		alertConds = append(alertConds, fmt.Sprintf("created_at >= $%d", argIdx))
		auditConds = append(auditConds, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filter.Since)
		argIdx++
	}

	alertWhere := strings.Join(alertConds, " AND ")
	auditWhere := strings.Join(auditConds, " AND ")

	// Count total
	countQuery := fmt.Sprintf(`
		SELECT (
			SELECT COUNT(*) FROM alerts WHERE %s
		) + (
			SELECT COUNT(*) FROM audit_events WHERE %s
		)
	`, alertWhere, auditWhere)

	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count event feed: %w", err)
	}

	// Query data with UNION ALL
	dataQuery := fmt.Sprintf(`
		SELECT id, source, event_type, severity, node_id, title, detail, created_at FROM (
			SELECT id::text, 'alert' AS source, alert_type AS event_type, severity,
			       COALESCE(node_id::text, '') AS node_id, title, context AS detail, created_at
			FROM alerts WHERE %s
			UNION ALL
			SELECT id::text, 'audit' AS source, event_type, '' AS severity,
			       COALESCE(node_id::text, '') AS node_id, summary AS title, detail, created_at
			FROM audit_events WHERE %s
		) combined
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, alertWhere, auditWhere, argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.Query(dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query event feed: %w", err)
	}
	defer rows.Close()

	items := make([]EventFeedItem, 0, filter.Limit)
	for rows.Next() {
		var (
			item      EventFeedItem
			rawDetail []byte
		)
		if err := rows.Scan(&item.ID, &item.Source, &item.EventType, &item.Severity,
			&item.NodeID, &item.Title, &rawDetail, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.Detail = map[string]interface{}{}
		if len(rawDetail) > 0 {
			if err := json.Unmarshal(rawDetail, &item.Detail); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal event detail: %w", err)
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
