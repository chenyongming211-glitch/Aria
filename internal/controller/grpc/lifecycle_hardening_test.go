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

func TestRegisterUsesHandlerIssuedRuntimeIdentity(t *testing.T) {
	nodeID := uuid.New().String()
	var captured *RegistrationRequest

	server := NewControllerServer(
		func(req *RegistrationRequest) (*RegistrationResult, error) {
			captured = req
			return &RegistrationResult{
				AssignedIP:            "100.64.0.2",
				MetricsPushGateway:    "http://metrics",
				NodeID:                nodeID,
				RuntimeToken:          "runtime-token",
				RuntimeTokenExpiresAt: 1893456000,
			}, nil
		},
		nil,
		nil,
	)

	resp, err := server.Register(context.Background(), &agentpb.RegisterRequest{
		PublicKey:        "node-register-key",
		Endpoint:         "203.0.113.10:51820",
		PrivateIp:        "10.0.0.10",
		PublicIp:         "203.0.113.10",
		Hostname:         "node-register",
		RegisteredAt:     1710000000,
		Token:            "enroll-token",
		AdvertisedRoutes: []string{"10.10.0.0/16"},
		Region:           "cn-east",
		CustomerId:       "tenant-a",
		RuntimeMode:      "ebpf",
		KernelVersion:    "6.8.0",
		HasAesni:         true,
		MachineId:        "machine-1",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if resp.NodeId != nodeID {
		t.Fatalf("expected node id from registration handler, got %q", resp.NodeId)
	}
	if resp.RuntimeToken != "runtime-token" {
		t.Fatalf("expected runtime token from registration handler, got %q", resp.RuntimeToken)
	}
	if resp.RuntimeTokenExpiresAt != 1893456000 {
		t.Fatalf("expected runtime token expiry from registration handler, got %d", resp.RuntimeTokenExpiresAt)
	}
	if captured == nil {
		t.Fatalf("expected registration handler to receive request")
	}
	if captured.PublicKey != "node-register-key" ||
		captured.Endpoint != "203.0.113.10:51820" ||
		captured.PrivateIP != "10.0.0.10" ||
		captured.PublicIP != "203.0.113.10" ||
		captured.Hostname != "node-register" ||
		captured.RegisteredAt != 1710000000 ||
		captured.Token != "enroll-token" ||
		captured.Region != "cn-east" ||
		captured.CustomerID != "tenant-a" ||
		captured.RuntimeMode != "ebpf" ||
		captured.KernelVersion != "6.8.0" ||
		!captured.HasAESNI ||
		captured.MachineID != "machine-1" {
		t.Fatalf("registration request was not preserved: %#v", captured)
	}
	if len(captured.AdvertisedRoutes) != 1 || captured.AdvertisedRoutes[0] != "10.10.0.0/16" {
		t.Fatalf("advertised routes were not preserved: %#v", captured.AdvertisedRoutes)
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
