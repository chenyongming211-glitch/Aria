package v2

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestBuildTenantNodeQoSPoliciesReturnsRuntimeErrors(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'auto'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority ASC, created_at DESC`)).
		WithArgs(tenantID, nodeID).
		WillReturnError(errors.New("qos query failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	_, err = router.buildTenantNodeQoSPolicies(tenantID, &controllerstorage.Node{
		ID:       nodeID,
		TenantID: tenantID,
		Hostname: "node-a",
	})
	if err == nil {
		t.Fatalf("expected QoS runtime query error")
	}
	if !strings.Contains(err.Error(), "QoS rules") {
		t.Fatalf("expected QoS error context, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
