package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	AuditNodeRegistered   = "node.registered"
	AuditNodeReregistered = "node.reregistered"
	AuditNodeSuspended    = "node.suspended"
	AuditNodeDeleted      = "node.deleted"
	AuditTenantSuspended  = "tenant.suspended"
	AuditTenantDeleted    = "tenant.deleted"
	AuditCertIssued       = "cert.issued"
	AuditCertRevoked      = "cert.revoked"
	AuditPolicyChanged    = "policy.changed"
	AuditCommandQueued    = "command.queued"
	AuditCommandResult    = "command.result"
)

// AuditEvent represents an audit event record in the audit_events table.
type AuditEvent struct {
	ID        uuid.UUID              `json:"id"`
	TenantID  uuid.UUID              `json:"tenant_id"`
	NodeID    *uuid.UUID             `json:"node_id,omitempty"`
	EventType string                 `json:"event_type"`
	Actor     string                 `json:"actor"`
	Summary   string                 `json:"summary"`
	Detail    map[string]interface{} `json:"detail"`
	CreatedAt time.Time              `json:"created_at"`
}

// AuditEventFilter defines filtering options for listing audit events.
type AuditEventFilter struct {
	NodeID    *uuid.UUID
	EventType string
	Since     *time.Time
	Limit     int
	Offset    int
}

// CreateAuditEvent inserts a new audit event record and returns the created row.
func (s *Storage) CreateAuditEvent(event *AuditEvent) (*AuditEvent, error) {
	if event.Detail == nil {
		event.Detail = map[string]interface{}{}
	}

	detailJSON, err := json.Marshal(event.Detail)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal audit event detail: %w", err)
	}

	row := s.db.QueryRow(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`, event.TenantID, event.NodeID, event.EventType, event.Actor, event.Summary, detailJSON)

	return scanAuditEventRow(row)
}

// ListAuditEvents queries audit events for a tenant with dynamic filtering.
// Returns the list of audit events, total count, and any error.
func (s *Storage) ListAuditEvents(tenantID uuid.UUID, filter AuditEventFilter) ([]*AuditEvent, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	// Build dynamic WHERE clauses
	conditions := []string{"tenant_id = $1"}
	args := []interface{}{tenantID}
	argIdx := 2

	if filter.NodeID != nil {
		conditions = append(conditions, fmt.Sprintf("node_id = $%d", argIdx))
		args = append(args, *filter.NodeID)
		argIdx++
	}
	if filter.EventType != "" {
		conditions = append(conditions, fmt.Sprintf("event_type = $%d", argIdx))
		args = append(args, filter.EventType)
		argIdx++
	}
	if filter.Since != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filter.Since)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total matching records
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_events WHERE %s", whereClause)
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit events: %w", err)
	}

	// Query the actual rows
	dataQuery := fmt.Sprintf(`
		SELECT id, tenant_id, node_id, event_type, actor, summary, detail, created_at
		FROM audit_events
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.Query(dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer rows.Close()

	events := make([]*AuditEvent, 0, filter.Limit)
	for rows.Next() {
		event, err := scanAuditEventRow(rows)
		if err != nil {
			return nil, 0, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// scanAuditEventRow scans a single audit event row from a query result.
func scanAuditEventRow(row interface {
	Scan(dest ...interface{}) error
}) (*AuditEvent, error) {
	var (
		event     AuditEvent
		nodeID    sql.NullString
		rawDetail []byte
	)

	if err := row.Scan(
		&event.ID,
		&event.TenantID,
		&nodeID,
		&event.EventType,
		&event.Actor,
		&event.Summary,
		&rawDetail,
		&event.CreatedAt,
	); err != nil {
		return nil, err
	}

	if nodeID.Valid {
		parsed, err := uuid.Parse(nodeID.String)
		if err == nil {
			event.NodeID = &parsed
		}
	}

	event.Detail = map[string]interface{}{}
	if len(rawDetail) > 0 {
		if err := json.Unmarshal(rawDetail, &event.Detail); err != nil {
			return nil, fmt.Errorf("failed to unmarshal audit event detail: %w", err)
		}
	}

	return &event, nil
}
