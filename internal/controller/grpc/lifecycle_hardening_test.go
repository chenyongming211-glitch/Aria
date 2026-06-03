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

	server := NewControllerServer(
		func(*RegistrationRequest) (*RegistrationResult, error) {
			return nil, auth.ErrRuntimeTokenSecretNotConfigured
		},
		nil,
		nil,
	)

	_, err := server.Register(context.Background(), &agentpb.RegisterRequest{
		PublicKey: "node-runtime-key",
		Hostname:  "node-1",
	})
	if err == nil {
		t.Fatalf("expected Register to fail when runtime token cannot be issued")
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

func TestSyncResponseIncludesSnapshotMetadata(t *testing.T) {
	auth.SetRuntimeSecret("phase1-runtime-secret")
	t.Cleanup(func() { auth.SetRuntimeSecret("") })

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	publicKey := "phase1-node-key"

	expectNodeByID(mock, nodeID, publicKey, tenantID, now)
	expectNodeByPublicKey(mock, publicKey, nodeID, tenantID, now)
	expectReportNodeControlState(mock, tenantID, nodeID, "applied-1", "applied", "", now)
	expectNodeByPublicKey(mock, publicKey, nodeID, tenantID, now)
	expectEmptyQoSRules(mock, tenantID, nodeID)
	expectNodeByPublicKey(mock, publicKey, nodeID, tenantID, now)
	expectEmptyBlacklistRules(mock, tenantID, nodeID)
	expectGetNodeControlState(mock, tenantID, nodeID, "dsv-phase1", now)

	ctx := context.WithValue(context.Background(), RuntimeNodeIDKey, nodeID.String())
	ctx = context.WithValue(ctx, RuntimeTenantIDKey, tenantID.String())

	server := NewControllerServer(
		nil,
		func(publicKey string) (interface{}, string, interface{}, string, error) {
			return []map[string]interface{}{}, "100.64.0.2", []map[string]interface{}{}, "http://metrics", nil
		},
		controllerstorage.NewStorageWithDB(db),
	)

	resp, err := server.Sync(ctx, &agentpb.SyncRequest{
		NodeId:              nodeID.String(),
		PublicKey:           publicKey,
		AppliedStateVersion: "applied-1",
		ObservedState:       "applied",
	})
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if !resp.GetSnapshotComplete() {
		t.Fatalf("expected snapshot_complete=true")
	}
	if resp.GetDomainVersions()["acl"] != "dsv-phase1" {
		t.Fatalf("expected acl domain version from desired state, got %#v", resp.GetDomainVersions())
	}
	if resp.GetDomainVersions()["peer"] != "dsv-phase1" {
		t.Fatalf("expected peer domain version from desired state, got %#v", resp.GetDomainVersions())
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

func expectReportNodeControlState(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, appliedVersion, observedState, observedMessage string, now time.Time) {
	mock.ExpectQuery(`(?s)INSERT INTO node_control_states .*RETURNING tenant_id, node_id`).
		WithArgs(tenantID, nodeID, appliedVersion, observedState, observedMessage, sqlmock.AnyArg(), "").
		WillReturnRows(nodeControlStateRowsFor(tenantID, nodeID, "dsv-phase1", now))
}

func expectGetNodeControlState(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, desiredVersion string, now time.Time) {
	mock.ExpectQuery(`(?s)SELECT tenant_id, node_id, COALESCE\(desired_state_version, ''\).*FROM node_control_states.*WHERE tenant_id = \$1 AND node_id = \$2`).
		WithArgs(tenantID, nodeID).
		WillReturnRows(nodeControlStateRowsFor(tenantID, nodeID, desiredVersion, now))
}

func nodeControlStateRowsFor(tenantID, nodeID uuid.UUID, desiredVersion string, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
		"applied_state_version", "applied_state_updated_at", "observed_state",
		"observed_message", "observed_at", "last_sync_at", "last_sync_error",
		"created_at", "updated_at",
	}).AddRow(
		tenantID, nodeID, desiredVersion, []byte(`{}`), now,
		"", nil, "",
		"", nil, nil, "",
		now, now,
	)
}

func expectEmptyQoSRules(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectQuery(`(?s)SELECT id, tenant_id, node_id, category.*FROM qos_rules.*WHERE tenant_id = \$1 AND node_id = \$2 AND enabled = true`).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "category", "src_cidr", "dst_cidr",
			"src_port", "dst_port", "protocol", "bandwidth_mbps", "enabled", "description", "created_at", "updated_at",
		}))
}

func expectEmptyBlacklistRules(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectQuery(`(?s)SELECT id, tenant_id, node_id, scope.*FROM blacklist_rules.*WHERE tenant_id = \$1 AND node_id = \$2 AND enabled = true`).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "scope", "cidr", "port", "enabled", "description", "created_at", "updated_at",
		}))
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
