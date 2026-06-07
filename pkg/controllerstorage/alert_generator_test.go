package controllerstorage

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGeneratePolicyFailedAlertIncludesCommandContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	alertID := uuid.New()
	auditID := uuid.New()
	commandID := uuid.New().String()
	now := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE tenant_id = $1 AND node_id = $2 AND alert_type = $3 AND status = 'active'
		LIMIT 1
	`)).
		WithArgs(tenantID, nodeID, "policy_failed").
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO alerts (tenant_id, node_id, alert_type, severity, title, message, context, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		          context, status, created_at, resolved_at
	`)).
		WithArgs(
			tenantID,
			sqlmock.AnyArg(),
			"policy_failed",
			"warning",
			"策略下发失败",
			"",
			jsonContextContains{
				"policy_domain": "acl",
				"policy_ref":    "acl-1",
				"command_id":    commandID,
				"error":         "iptables apply failed",
			},
			"active",
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title", "message",
			"context", "status", "created_at", "resolved_at",
		}).AddRow(
			alertID, tenantID, nodeID, "policy_failed", "warning", "策略下发失败", "",
			[]byte(`{"policy_domain":"acl","policy_ref":"acl-1","command_id":"`+commandID+`"}`), "active", now, nil,
		))

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, "alert_created", "system", "告警创建: 策略下发失败", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(auditID, tenantID, nodeID, "alert_created", "system", "告警创建: 策略下发失败", []byte(`{}`), now))

	store := NewStorageWithDB(db)
	if err := store.GeneratePolicyFailedAlert(tenantID, nodeID, "acl", "acl-1", commandID, "iptables apply failed"); err != nil {
		t.Fatalf("GeneratePolicyFailedAlert returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

type jsonContextContains map[string]string

func (m jsonContextContains) Match(value driver.Value) bool {
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return false
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	for key, expected := range m {
		if data[key] != expected {
			return false
		}
	}
	return true
}
