package grpc

import (
	"context"
	"regexp"
	"testing"

	"aria/internal/auth"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestUnaryAuthInterceptorRejectsStaleRuntimeTokenVersion(t *testing.T) {
	auth.SetRuntimeSecret("test-runtime-secret")
	t.Cleanup(func() { auth.SetRuntimeSecret("") })

	nodeID := uuid.New()
	tenantID := uuid.New()
	token, _, err := auth.GenerateRuntimeTokenWithVersion(nodeID.String(), tenantID.String(), 1)
	if err != nil {
		t.Fatalf("GenerateRuntimeTokenWithVersion failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT n.id, n.tenant_id, COALESCE(n.status, 'online'), COALESCE(n.runtime_token_version, 0), COALESCE(t.status, '')
		FROM nodes n
		JOIN tenants t ON t.id = n.tenant_id
		WHERE n.id = $1
	`)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "node_status", "runtime_token_version", "tenant_status"}).
			AddRow(nodeID, tenantID, "online", 2, "active"))

	interceptor := UnaryAuthInterceptor(controllerstorage.NewStorageWithDB(db))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
	_, err = interceptor(ctx, nil, &googlegrpc.UnaryServerInfo{FullMethod: "/aria.agent.ControllerService/Sync"}, func(context.Context, interface{}) (interface{}, error) {
		t.Fatalf("handler should not be called for stale runtime token")
		return nil, nil
	})

	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated for stale runtime token, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
