package controllerstorage

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func expectLifecycleStopSideEffects(mock sqlmock.Sqlmock, nodeID uuid.UUID, publicKey, reason, message string) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE node_certificates
		SET status = $2,
		    revoked_at = NOW(),
		    revoke_reason = $3,
		    updated_at = NOW()
		WHERE node_id = $1`)).
		WithArgs(nodeID, CertStatusRevoked, reason).
		WillReturnResult(sqlmock.NewResult(0, 1))
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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, assigned_ip, ip_offset FROM nodes WHERE hostname = $1 AND tenant_id = $2 AND status != 'deleted' FOR UPDATE LIMIT 1`)).
		WithArgs("edge-1", tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "public_key", "assigned_ip", "ip_offset"}).
			AddRow(nodeID, publicKey, "100.64.0.2", 2))
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
