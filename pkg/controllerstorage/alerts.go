package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Alert represents an alert record in the alerts table.
type Alert struct {
	ID         uuid.UUID              `json:"id"`
	TenantID   uuid.UUID              `json:"tenant_id"`
	NodeID     *uuid.UUID             `json:"node_id,omitempty"`
	AlertType  string                 `json:"alert_type"`
	Severity   string                 `json:"severity"`
	Title      string                 `json:"title"`
	Message    string                 `json:"message"`
	Context    map[string]interface{} `json:"context"`
	Status     string                 `json:"status"`
	CreatedAt  time.Time              `json:"created_at"`
	ResolvedAt *time.Time             `json:"resolved_at,omitempty"`
}

// AlertFilter defines filtering options for listing alerts.
type AlertFilter struct {
	Status    string     // active / resolved / all
	AlertType string     // optional
	NodeID    *uuid.UUID // optional
	Limit     int
	Offset    int
}

// CreateAlert inserts a new alert record and returns the created row.
func (s *Storage) CreateAlert(alert *Alert) (*Alert, error) {
	if alert.Context == nil {
		alert.Context = map[string]interface{}{}
	}

	contextJSON, err := json.Marshal(alert.Context)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal alert context: %w", err)
	}

	row := s.db.QueryRow(`
		INSERT INTO alerts (tenant_id, node_id, alert_type, severity, title, message, context, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		          context, status, created_at, resolved_at
	`, alert.TenantID, alert.NodeID, alert.AlertType, alert.Severity,
		alert.Title, alert.Message, contextJSON, "active")

	return scanAlertRow(row)
}

// ResolveAlert updates an active alert to resolved status and returns the updated row.
func (s *Storage) ResolveAlert(alertID uuid.UUID) (*Alert, error) {
	row := s.db.QueryRow(`
		UPDATE alerts
		SET status = 'resolved', resolved_at = NOW()
		WHERE id = $1 AND status = 'active'
		RETURNING id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		          context, status, created_at, resolved_at
	`, alertID)

	return scanAlertRow(row)
}

// GetActiveAlertByNodeAndType finds an active alert for a specific node and alert type.
// Returns nil, nil if no matching alert is found.
func (s *Storage) GetActiveAlertByNodeAndType(tenantID, nodeID uuid.UUID, alertType string) (*Alert, error) {
	row := s.db.QueryRow(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE tenant_id = $1 AND node_id = $2 AND alert_type = $3 AND status = 'active'
		LIMIT 1
	`, tenantID, nodeID, alertType)

	alert, err := scanAlertRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return alert, err
}

// ListAlerts queries alerts for a tenant with dynamic filtering.
// Returns the list of alerts, total count, and any error.
func (s *Storage) ListAlerts(tenantID uuid.UUID, filter AlertFilter) ([]*Alert, int, error) {
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

	if filter.Status != "" && filter.Status != "all" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.AlertType != "" {
		conditions = append(conditions, fmt.Sprintf("alert_type = $%d", argIdx))
		args = append(args, filter.AlertType)
		argIdx++
	}
	if filter.NodeID != nil {
		conditions = append(conditions, fmt.Sprintf("node_id = $%d", argIdx))
		args = append(args, *filter.NodeID)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total matching records
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM alerts WHERE %s", whereClause)
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count alerts: %w", err)
	}

	// Query the actual rows
	dataQuery := fmt.Sprintf(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.Query(dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query alerts: %w", err)
	}
	defer rows.Close()

	alerts := make([]*Alert, 0, filter.Limit)
	for rows.Next() {
		alert, err := scanAlertRow(rows)
		if err != nil {
			return nil, 0, err
		}
		alerts = append(alerts, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return alerts, total, nil
}

// CountActiveAlerts returns the number of active alerts for a tenant.
func (s *Storage) CountActiveAlerts(tenantID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM alerts WHERE tenant_id = $1 AND status = 'active'
	`, tenantID).Scan(&count)
	return count, err
}

// scanAlertRow scans a single alert row from a query result.
func scanAlertRow(row interface {
	Scan(dest ...interface{}) error
}) (*Alert, error) {
	var (
		alert      Alert
		nodeID     sql.NullString
		rawContext  []byte
		resolvedAt sql.NullTime
	)

	if err := row.Scan(
		&alert.ID,
		&alert.TenantID,
		&nodeID,
		&alert.AlertType,
		&alert.Severity,
		&alert.Title,
		&alert.Message,
		&rawContext,
		&alert.Status,
		&alert.CreatedAt,
		&resolvedAt,
	); err != nil {
		return nil, err
	}

	if nodeID.Valid {
		parsed, err := uuid.Parse(nodeID.String)
		if err == nil {
			alert.NodeID = &parsed
		}
	}

	alert.Context = map[string]interface{}{}
	if len(rawContext) > 0 {
		if err := json.Unmarshal(rawContext, &alert.Context); err != nil {
			return nil, fmt.Errorf("failed to unmarshal alert context: %w", err)
		}
	}

	if resolvedAt.Valid {
		alert.ResolvedAt = &resolvedAt.Time
	}

	return &alert, nil
}
