package controllerstorage

import (
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func testNodeSelectColumns() []string {
	return []string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}
}

func TestSaveNodeBackfillsDatabaseIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	nodeID := uuid.New()
	tenantID := uuid.New()
	createdAt := time.Now().Add(-time.Minute).UTC()
	updatedAt := time.Now().UTC()

	mock.ExpectQuery(`(?s)INSERT INTO nodes .*RETURNING id, created_at, updated_at`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(nodeID, createdAt, updatedAt))

	node := &Node{
		PublicKey:         "pub-key-1",
		MachineID:         "machine-1",
		TenantID:          tenantID,
		Endpoint:          "1.1.1.1:51820",
		PrivateIP:         "10.0.0.1",
		PublicIP:          "1.1.1.1",
		Region:            "sh",
		VPCID:             "vpc-1",
		Hostname:          "node-1",
		AssignedIP:        "100.64.0.1",
		IPOffset:          1,
		LastSeen:          time.Now().Unix(),
		RegisteredAt:      time.Now().Unix(),
		Role:              "agent",
		RuntimeMode:       "kernel",
		KernelVersion:     "6.0",
		HasAESNI:          true,
		AdvertisedRoutes:  []string{"10.1.0.0/16"},
		EnrolledWithToken: "enroll-token",
	}

	if err := store.SaveNode(node); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}
	if node.ID != nodeID {
		t.Fatalf("expected node ID %s, got %s", nodeID, node.ID)
	}
	if node.Status != "online" {
		t.Fatalf("expected status online, got %q", node.Status)
	}
	if !node.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created_at %s, got %s", createdAt, node.CreatedAt)
	}
	if !node.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected updated_at %s, got %s", updatedAt, node.UpdatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSaveNodeRejectsNilNode(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	if err := store.SaveNode(nil); err == nil {
		t.Fatal("expected error for nil node")
	}
}

func TestUpdateNodeHeartbeatDoesNotReviveInactiveNode(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	nodeID := uuid.New()
	lastSeen := time.Now().Unix()

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes
		SET last_seen = $2,
		    status = 'online',
		    offline_since = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND status NOT IN ('deleted', 'suspended', 'banned')`)).
		WithArgs(nodeID, lastSeen).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = NewStorageWithDB(db).UpdateNodeHeartbeat(nodeID, lastSeen)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for inactive node heartbeat, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateNodePublicIdentityClearsPrivateIP(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	nodeID := uuid.New()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes
		SET public_ip = COALESCE(NULLIF($2, ''), public_ip),
		    endpoint = COALESCE(NULLIF($3, ''), endpoint),
		    private_ip = '',
		    updated_at = NOW()
		WHERE id = $1
		  AND status NOT IN ('deleted', 'suspended', 'banned')
		  AND (
		      ($2 <> '' AND COALESCE(public_ip, '') <> $2)
		   OR ($3 <> '' AND COALESCE(endpoint, '') <> $3)
		   OR COALESCE(private_ip, '') <> ''
		  )`)).
		WithArgs(nodeID, "82.156.48.111", "82.156.48.111:51820").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewStorageWithDB(db).UpdateNodePublicIdentity(nodeID, "82.156.48.111", "82.156.48.111:51820")
	if err != nil {
		t.Fatalf("UpdateNodePublicIdentity failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGetNodesByTenantReturnsRowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	rowsErr := errors.New("cursor failed")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND status != 'deleted' ORDER BY last_seen DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows(testNodeSelectColumns()).AddRow(
			nodeID, "pub-key-1", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			now.Unix(), now.Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		).RowError(0, rowsErr))

	_, err = NewStorageWithDB(db).GetNodesByTenant(tenantID)
	if !errors.Is(err, rowsErr) {
		t.Fatalf("expected rows error %v, got %v", rowsErr, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
