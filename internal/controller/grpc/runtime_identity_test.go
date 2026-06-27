package grpc

import (
	"context"
	"regexp"
	"testing"
	"time"

	controllerstorage "aria/pkg/controllerstorage"
	"aria/pkg/grpc/agentpb"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestRuntimeContextGettersRejectWrongTypes(t *testing.T) {
	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, 123)
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, 456)

	if _, ok := GetRuntimeNodeID(ctx); ok {
		t.Fatalf("expected GetRuntimeNodeID to reject wrong type")
	}
	if _, ok := GetRuntimeTenantID(ctx); ok {
		t.Fatalf("expected GetRuntimeTenantID to reject wrong type")
	}
}

func TestExtractBearerTokenAcceptsCaseInsensitiveScheme(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "bearer rt_test"))

	token, err := extractBearerToken(ctx)
	if err != nil {
		t.Fatalf("extractBearerToken returned error: %v", err)
	}
	if token != "rt_test" {
		t.Fatalf("expected rt_test, got %q", token)
	}
}

func TestResolveRuntimeNodeForRequestRejectsRuntimeTokenNodeMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)

	tenantID := uuid.New()
	tokenNodeID := uuid.New()
	requestNodeID := uuid.New()
	now := time.Now()

	expectNodeByID(mock, requestNodeID, "request-node-key", tenantID, now)
	expectNodeByPublicKey(mock, "request-node-key", requestNodeID, tenantID, now)

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, tokenNodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	_, err = server.resolveRuntimeNodeForRequest(ctx, requestNodeID.String(), "request-node-key")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied for runtime token node mismatch, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolveRuntimeNodeForRequestRejectsMissingRuntimeTokenTenant(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	expectNodeByID(mock, nodeID, "request-node-key", tenantID, now)
	expectNodeByPublicKey(mock, "request-node-key", nodeID, tenantID, now)

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, nodeID.String())

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	_, err = server.resolveRuntimeNodeForRequest(ctx, nodeID.String(), "request-node-key")
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated for missing runtime token tenant, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolveCommandStreamNodeForRequestRejectsRuntimeTokenNodeMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)

	tenantID := uuid.New()
	tokenNodeID := uuid.New()
	initNodeID := uuid.New()
	now := time.Now()

	expectNodeByID(mock, initNodeID, "init-node-key", tenantID, now)
	expectNodeByPublicKey(mock, "init-node-key", initNodeID, tenantID, now)

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, tokenNodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	_, err = server.resolveCommandStreamNodeForRequest(ctx, &agentpb.CommandResponse{
		CommandId: "init",
		Status:    "ready",
		NodeId:    initNodeID.String(),
		PublicKey: "init-node-key",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied for command stream token node mismatch, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolveRuntimeNodeForRequestRejectsInactiveTenant(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	expectNodeByID(mock, nodeID, "inactive-tenant-node-key", tenantID, now)
	expectNodeByPublicKey(mock, "inactive-tenant-node-key", nodeID, tenantID, now)
	expectRuntimeTenantStatus(mock, tenantID, "suspended")

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, nodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	_, err = server.resolveRuntimeNodeForRequest(ctx, nodeID.String(), "inactive-tenant-node-key")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied for inactive runtime tenant, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolveLegacyAgentIdentityRejectsInactiveNode(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	expectNodeByIDWithStatus(mock, nodeID, "legacy-node-key", tenantID, now, "suspended")

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	_, err = server.resolveLegacyAgentIdentity(nodeID.String())
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied for inactive legacy identity, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func expectNodeByID(mock sqlmock.Sqlmock, nodeID uuid.UUID, publicKey string, tenantID uuid.UUID, now time.Time) {
	expectNodeByIDWithStatus(mock, nodeID, publicKey, tenantID, now, "online")
}

func expectRuntimeTenantStatus(mock sqlmock.Sqlmock, tenantID uuid.UUID, tenantStatus string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM tenants WHERE id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(tenantStatus))
}

func expectNodeByIDWithStatus(mock sqlmock.Sqlmock, nodeID uuid.UUID, publicKey string, tenantID uuid.UUID, now time.Time, nodeStatus string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-"+publicKey, tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-"+publicKey, "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			nodeStatus, int64(0), pq.StringArray{},
			"", now, now,
		))
}

func expectNodeByPublicKey(mock sqlmock.Sqlmock, publicKey string, nodeID uuid.UUID, tenantID uuid.UUID, now time.Time) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1`)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-"+publicKey, tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-"+publicKey, "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			"online", int64(0), pq.StringArray{},
			"", now, now,
		))
}
