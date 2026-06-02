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

func TestBuildTenantNodeQoSPoliciesReturnsCategoryErrors(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND category = $3
		  ORDER BY created_at DESC`)).
		WithArgs(tenantID, nodeID, controllerstorage.QoSCategoryService).
		WillReturnError(errors.New("qos query failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	_, err = router.buildTenantNodeQoSPolicies(tenantID, &controllerstorage.Node{
		ID:       nodeID,
		TenantID: tenantID,
		Hostname: "node-a",
	})
	if err == nil {
		t.Fatalf("expected QoS category query error")
	}
	if !strings.Contains(err.Error(), controllerstorage.QoSCategoryService) {
		t.Fatalf("expected category in error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
