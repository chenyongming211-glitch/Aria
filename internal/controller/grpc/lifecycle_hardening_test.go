package grpc

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"aria/internal/auth"
	controllerstorage "aria/pkg/controllerstorage"
	"aria/pkg/grpc/agentpb"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestRegisterFailsWhenRuntimeTokenCannotBeIssued(t *testing.T) {
	auth.SetRuntimeSecret("")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	expectNodeByPublicKey(mock, "node-runtime-key", nodeID, tenantID, now)
	expectNodeByID(mock, nodeID, "node-runtime-key", tenantID, now)

	server := NewControllerServer(
		func(interface{}) (string, string, error) {
			return "100.64.0.2", "http://metrics", nil
		},
		nil,
		controllerstorage.NewStorageWithDB(db),
	)

	_, err = server.Register(context.Background(), &agentpb.RegisterRequest{
		PublicKey: "node-runtime-key",
		Hostname:  "node-1",
	})
	if err == nil {
		t.Fatalf("expected Register to fail when runtime token cannot be issued")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestReportMetricsUsesHeartbeatOnlyUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)

	tenantID := uuid.New()
	nodeID := uuid.New()
	publicKey := "metrics-node-key"
	now := time.Now()

	expectNodeByID(mock, nodeID, publicKey, tenantID, now)
	expectNodeByPublicKey(mock, publicKey, nodeID, tenantID, now)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes
		SET last_seen = $2,
		    status = 'online',
		    offline_since = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND status NOT IN ('deleted', 'suspended', 'banned')`)).
		WithArgs(nodeID, sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, nodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	resp, err := server.ReportMetrics(ctx, &agentpb.MetricsReportRequest{
		NodeId:    nodeID.String(),
		PublicKey: publicKey,
	})
	if err == nil {
		t.Fatalf("expected ReportMetrics to return persistence error")
	}
	if resp == nil || resp.Success {
		t.Fatalf("expected unsuccessful response, got %#v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSyncPolicyRuleLoadErrorsPropagate(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*ControllerServer) error
	}{
		{
			name: "qos",
			call: func(server *ControllerServer) error {
				_, err := server.getQoSRules(context.Background(), "node-policy-key")
				return err
			},
		},
		{
			name: "blacklist",
			call: func(server *ControllerServer) error {
				_, err := server.getBlacklistRules(context.Background(), "node-policy-key")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1`)).
				WithArgs("node-policy-key").
				WillReturnError(errors.New("node lookup failed"))

			server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
			if err := tc.call(server); err == nil {
				t.Fatalf("expected %s load error to propagate", tc.name)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}
