package controllerstorage

import (
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func expectLifecycleStopSideEffects(mock sqlmock.Sqlmock, nodeID uuid.UUID, publicKey, reason, message string) {
	expectLifecycleCertificateRevokeUpdate(mock, nodeID, reason)
	expectLifecycleCommandFailures(mock, publicKey, message)
}

func expectLifecycleCertificateRevokeUpdate(mock sqlmock.Sqlmock, nodeID uuid.UUID, reason string) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE node_certificates
		SET status = $2,
		    revoked_at = NOW(),
		    revoke_reason = $3,
		    updated_at = NOW()
		WHERE node_id = $1 AND status = $4`)).
		WithArgs(nodeID, CertStatusRevoked, reason, CertStatusIssued).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectLifecycleCommandFailures(mock sqlmock.Sqlmock, publicKey, message string) {
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(publicKey, AgentCommandStatusFailed, message, AgentCommandStatusPending, AgentCommandStatusSent, AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(publicKey, AgentCommandStatusFailed, message, AgentCommandStatusPending, AgentCommandStatusSent, AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestMarkNodeDeletedRevokesCertificatesAndFailsCommands(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	nodeID := uuid.New()
	publicKey := "node-key"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM nodes WHERE public_key = $1 FOR UPDATE`)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(nodeID))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes
		SET status = 'deleted', updated_at = NOW()
		WHERE public_key = $1`)).
		WithArgs(publicKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLifecycleStopSideEffects(mock, nodeID, publicKey, "node_deleted", "node deleted")
	mock.ExpectCommit()

	if err := NewStorageWithDB(db).MarkNodeDeleted(publicKey); err != nil {
		t.Fatalf("MarkNodeDeleted returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

type lifecycleCertRevokedAuditDetailMatcher struct {
	status string
	reason string
}

func (m lifecycleCertRevokedAuditDetailMatcher) Match(value driver.Value) bool {
	raw, ok := value.([]byte)
	if !ok {
		text, ok := value.(string)
		if !ok {
			return false
		}
		raw = []byte(text)
	}
	var detail map[string]interface{}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return false
	}
	return detail["node_status"] == m.status &&
		detail["reason"] == m.reason &&
		detail["revoked_cert_count"] == float64(1)
}

func expectLifecycleCertificateRevokedAudit(
	mock sqlmock.Sqlmock,
	tenantID, nodeID uuid.UUID,
	actor, status, reason string,
) {
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
	`)).
		WithArgs(
			tenantID,
			nodeID,
			AuditCertRevoked,
			actor,
			"Node certificate revoked due to node lifecycle change",
			lifecycleCertRevokedAuditDetailMatcher{status: status, reason: reason},
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectLifecycleNodeLookup(mock sqlmock.Sqlmock, nodeID, tenantID uuid.UUID, publicKey, status string) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ` + nodeSelectColumns + ` FROM nodes WHERE public_key = $1 FOR UPDATE`)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, publicKey, "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "edge-1", "10.0.0.10", 10,
			now.Unix(), now.Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, status, int64(0), "{}", "", now, now,
		))
}

func expectLifecycleStatusUpdate(mock sqlmock.Sqlmock, publicKey, targetStatus string) {
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE nodes
		SET status = $2, updated_at = NOW()
		WHERE public_key = $1
	`)).
		WithArgs(publicKey, targetStatus).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestSuspendNodeRevokesIssuedCertificate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	publicKey := "node-key"

	mock.ExpectBegin()
	expectLifecycleNodeLookup(mock, nodeID, tenantID, publicKey, "online")
	expectLifecycleStatusUpdate(mock, publicKey, "suspended")
	expectLifecycleCertificateRevokeUpdate(mock, nodeID, "node suspended")
	expectLifecycleCertificateRevokedAudit(mock, tenantID, nodeID, "operator", "suspended", "node suspended")
	expectLifecycleCommandFailures(mock, publicKey, "node status changed to suspended")
	mock.ExpectExec(regexp.QuoteMeta(`
			INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
			VALUES ($1, $2, $3, $4, $5, $6)
		`)).
		WithArgs(tenantID, nodeID, AuditNodeSuspended, "operator", "Node suspended", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	node, err := NewStorageWithDB(db).ApplyNodeLifecycleTransition(publicKey, NodeLifecycleTransition{
		TargetStatus:   "suspended",
		AuditEventType: AuditNodeSuspended,
		AuditActor:     "operator",
		AuditSummary:   "Node suspended",
	})
	if err != nil {
		t.Fatalf("ApplyNodeLifecycleTransition returned error: %v", err)
	}
	if node.Status != "suspended" {
		t.Fatalf("expected node status suspended, got %q", node.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestBanNodeRevokesIssuedCertificate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	publicKey := "node-key"

	mock.ExpectBegin()
	expectLifecycleNodeLookup(mock, nodeID, tenantID, publicKey, "online")
	expectLifecycleStatusUpdate(mock, publicKey, "banned")
	expectLifecycleCertificateRevokeUpdate(mock, nodeID, "node banned")
	expectLifecycleCertificateRevokedAudit(mock, tenantID, nodeID, "admin", "banned", "node banned")
	expectLifecycleCommandFailures(mock, publicKey, "node status changed to banned")
	mock.ExpectExec(regexp.QuoteMeta(`
			INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
			VALUES ($1, $2, $3, $4, $5, $6)
		`)).
		WithArgs(tenantID, nodeID, "node_banned", "admin", "Node banned", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	node, err := NewStorageWithDB(db).ApplyNodeLifecycleTransition(publicKey, NodeLifecycleTransition{
		TargetStatus:   "banned",
		AuditEventType: "node_banned",
		AuditActor:     "admin",
		AuditSummary:   "Node banned",
	})
	if err != nil {
		t.Fatalf("ApplyNodeLifecycleTransition returned error: %v", err)
	}
	if node.Status != "banned" {
		t.Fatalf("expected node status banned, got %q", node.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestReuseHostnameIPRevokesCertificatesAndFailsCommands(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	publicKey := "old-node-key"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, assigned_ip, ip_offset, COALESCE(status, 'online') FROM nodes WHERE hostname = $1 AND tenant_id = $2 AND status != 'deleted' FOR UPDATE LIMIT 1`)).
		WithArgs("edge-1", tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "public_key", "assigned_ip", "ip_offset", "status"}).
			AddRow(nodeID, publicKey, "100.64.0.2", 2, "online"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes SET status = 'deleted', updated_at = NOW() WHERE public_key = $1`)).
		WithArgs(publicKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLifecycleStopSideEffects(mock, nodeID, publicKey, "hostname_reused", "node deleted for hostname reuse")
	mock.ExpectCommit()

	assignedIP, ipOffset, err := NewStorageWithDB(db).ReuseHostnameIP("edge-1", tenantID)
	if err != nil {
		t.Fatalf("ReuseHostnameIP returned error: %v", err)
	}
	if assignedIP != "100.64.0.2" || ipOffset != 2 {
		t.Fatalf("unexpected reused address: %s/%d", assignedIP, ipOffset)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestReuseHostnameIPRejectsSuspendedOrBannedNode(t *testing.T) {
	for _, status := range []string{"suspended", "banned"} {
		t.Run(status, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			tenantID := uuid.New()
			nodeID := uuid.New()

			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, assigned_ip, ip_offset, COALESCE(status, 'online') FROM nodes WHERE hostname = $1 AND tenant_id = $2 AND status != 'deleted' FOR UPDATE LIMIT 1`)).
				WithArgs("edge-1", tenantID).
				WillReturnRows(sqlmock.NewRows([]string{"id", "public_key", "assigned_ip", "ip_offset", "status"}).
					AddRow(nodeID, "old-node-key", "100.64.0.2", 2, status))
			mock.ExpectRollback()

			assignedIP, ipOffset, err := NewStorageWithDB(db).ReuseHostnameIP("edge-1", tenantID)
			if err == nil {
				t.Fatalf("expected %s hostname reuse to fail", status)
			}
			if assignedIP != "" || ipOffset != 0 {
				t.Fatalf("expected no reused address, got %s/%d", assignedIP, ipOffset)
			}
			if !strings.Contains(err.Error(), "inactive hostname") {
				t.Fatalf("expected inactive hostname error, got %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}
