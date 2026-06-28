package controllerstorage

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

var ipGroupReferenceColumns = []string{
	"domain",
	"rule_id",
	"rule_name",
	"node_id",
	"node_name",
	"direction",
	"enabled",
	"total",
	"delivery_id",
	"command_id",
	"command_status",
	"last_error",
	"delivery_created_at",
}

func TestListIPGroupReferencesUsesLatestDelivery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	groupID := uuid.New()
	nodeID := uuid.New()
	aclID := uuid.New()
	qosID := uuid.New()
	deliveryID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(listIPGroupReferencesSQL)).
		WithArgs(tenantID, groupID, 20, 0).
		WillReturnRows(sqlmock.NewRows(ipGroupReferenceColumns).
			AddRow("acl", aclID, "office-acl", nodeID, "node-a", "egress", true, 2, deliveryID, "cmd-2", AgentCommandStatusCompleted, "", now).
			AddRow("qos", qosID, "office-qos", nodeID, "node-a", "egress", true, 2, nil, nil, nil, nil, nil))

	page, err := store.ListIPGroupReferences(context.Background(), tenantID, groupID, 20, 0)
	if err != nil {
		t.Fatalf("ListIPGroupReferences failed: %v", err)
	}
	if page.Total != 2 || page.Limit != 20 || page.Offset != 0 || page.HasMore {
		t.Fatalf("unexpected page metadata: %#v", page)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 references, got %d", len(page.Items))
	}

	aclRef := findIPGroupReference(t, page.Items, "acl", aclID)
	if aclRef.LatestDelivery == nil {
		t.Fatalf("expected latest delivery for ACL ref")
	}
	if aclRef.LatestDelivery.Status != AgentCommandStatusCompleted || aclRef.LatestDelivery.CommandID != "cmd-2" || aclRef.LatestDelivery.LastError != "" {
		t.Fatalf("unexpected latest delivery: %#v", aclRef.LatestDelivery)
	}

	qosRef := findIPGroupReference(t, page.Items, "qos", qosID)
	if qosRef.LatestDelivery != nil {
		t.Fatalf("expected nil delivery for QoS ref, got %#v", qosRef.LatestDelivery)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListIPGroupReferencesPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	groupID := uuid.New()
	nodeID := uuid.New()
	aclID := uuid.New()
	deliveryID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(listIPGroupReferencesSQL)).
		WithArgs(tenantID, groupID, 1, 0).
		WillReturnRows(sqlmock.NewRows(ipGroupReferenceColumns).
			AddRow("acl", aclID, "office-acl", nodeID, "node-a", "egress", true, 2, deliveryID, "cmd-2", AgentCommandStatusCompleted, "", now))

	page, err := store.ListIPGroupReferences(context.Background(), tenantID, groupID, 1, 0)
	if err != nil {
		t.Fatalf("ListIPGroupReferences failed: %v", err)
	}
	if page.Total != 2 || page.Limit != 1 || page.Offset != 0 || !page.HasMore {
		t.Fatalf("unexpected page metadata: %#v", page)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(page.Items))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func findIPGroupReference(t *testing.T, refs []*IPGroupReferenceRecord, domain string, ruleID uuid.UUID) *IPGroupReferenceRecord {
	t.Helper()
	for _, ref := range refs {
		if ref.Domain == domain && ref.RuleID == ruleID {
			return ref
		}
	}
	t.Fatalf("reference %s/%s not found in %#v", domain, ruleID, refs)
	return nil
}
