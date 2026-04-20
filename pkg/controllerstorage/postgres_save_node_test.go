package controllerstorage

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

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
