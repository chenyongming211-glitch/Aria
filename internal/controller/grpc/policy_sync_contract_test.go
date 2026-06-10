package grpc

import (
	"context"
	"regexp"
	"testing"
	"time"

	controllerstorage "aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestGetQoSRulesIncludesAgentRuntimeContractFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	now := time.Now()
	publicKey := "node-policy-key"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1`)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(nodeRowColumns()).AddRow(
			nodeID, publicKey, "machine-node", tenantID,
			"1.2.3.4:51820", "", "1.2.3.4", "test-region",
			"", "node-policy", "100.64.0.2", 2,
			now.Unix(), now.Unix(), "agent",
			"ebpf", "6.8.0", true,
			"online", int64(0), pq.StringArray{},
			"", now, now,
		))

	mock.ExpectQuery(`(?s)SELECT id, tenant_id, node_id, COALESCE\(src_cidr::text, ''\).*direction.*rate_bps.*burst_bytes.*priority.*mode.*FROM qos_rules.*WHERE tenant_id = \$1 AND node_id = \$2 AND enabled = true`).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "src_cidr", "dst_cidr",
			"src_port", "dst_port", "protocol", "bandwidth_mbps", "direction",
			"rate_bps", "burst_bytes", "priority", "mode", "enabled", "description",
			"created_at", "updated_at",
		}).AddRow(
			ruleID, tenantID, nodeID, "10.0.0.0/24", "192.0.2.0/24",
			0, 443, 6, 100, "egress",
			uint64(250_000_000), uint64(4_000_000), 7, "shaping", true, "shape web",
			now, now,
		))

	server := NewControllerServer(nil, nil, controllerstorage.NewStorageWithDB(db))
	rules, err := server.getQoSRules(context.Background(), publicKey)
	if err != nil {
		t.Fatalf("getQoSRules failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one QoS rule, got %d", len(rules))
	}

	rule := rules[0]
	if rule.GetDirection() != "egress" ||
		rule.GetRateBps() != 250_000_000 ||
		rule.GetBurstBytes() != 4_000_000 ||
		rule.GetPriority() != 7 ||
		rule.GetMode() != "shaping" {
		t.Fatalf("runtime QoS fields not propagated: %#v", rule)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
