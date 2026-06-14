package controllerstorage

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCreateIPGroupStoresTenantScopedMembers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	groupID := uuid.New()
	now := time.Now()

	expectNoExactDuplicateIPGroupMember(mock, tenantID, uuid.Nil, "10.10.0.0/16")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(insertIPGroupSQL)).
		WithArgs(tenantID, "office", "office networks", IPGroupKindCustom, sql.NullString{}).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "description", "kind", "created_by", "created_at", "updated_at",
		}).AddRow(groupID, tenantID, "office", "office networks", IPGroupKindCustom, sql.NullString{}, now, now))
	mock.ExpectExec(regexp.QuoteMeta(insertIPGroupMemberSQL)).
		WithArgs(tenantID, groupID, "10.10.0.0/16", "").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	created, err := store.CreateIPGroup(&IPGroupRecord{
		TenantID:    tenantID,
		Name:        " office ",
		Description: " office networks ",
		Kind:        IPGroupKindCustom,
		Members:     []IPGroupMemberRecord{{CIDR: "10.10.0.12/16"}},
	})
	if err != nil {
		t.Fatalf("CreateIPGroup failed: %v", err)
	}
	if created.ID != groupID || created.Name != "office" || len(created.Members) != 1 || created.Members[0].CIDR != "10.10.0.0/16" {
		t.Fatalf("unexpected IP group: %#v", created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateIPGroupRejectsExactDuplicateCIDRAcrossCustomGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	existingGroupID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(exactDuplicateIPGroupMemberSQL)).
		WithArgs(tenantID, uuid.Nil, "10.10.0.0/16").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "cidr"}).
			AddRow(existingGroupID, "office", "10.10.0.0/16"))

	_, err = store.CreateIPGroup(&IPGroupRecord{
		TenantID: tenantID,
		Name:     "branch",
		Kind:     IPGroupKindCustom,
		Members:  []IPGroupMemberRecord{{CIDR: "10.10.0.0/16"}},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate CIDR") {
		t.Fatalf("expected duplicate CIDR error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureInlineIPGroupUsesDeterministicName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	groupID := uuid.New()
	now := time.Now()
	name := inlineIPGroupName([]string{"10.10.0.0/16", "192.0.2.4/32"})

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(upsertInlineIPGroupSQL)).
		WithArgs(tenantID, name, "inline policy group", IPGroupKindInline).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "description", "kind", "created_by", "created_at", "updated_at",
		}).AddRow(groupID, tenantID, name, "inline policy group", IPGroupKindInline, sql.NullString{}, now, now))
	expectNoExactDuplicateIPGroupMember(mock, tenantID, groupID, "10.10.0.0/16")
	expectNoExactDuplicateIPGroupMember(mock, tenantID, groupID, "192.0.2.4/32")
	mock.ExpectExec(regexp.QuoteMeta(upsertIPGroupMemberSQL)).
		WithArgs(tenantID, groupID, "10.10.0.0/16", "inline").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(upsertIPGroupMemberSQL)).
		WithArgs(tenantID, groupID, "192.0.2.4/32", "inline").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	group, err := store.EnsureInlineIPGroup(tenantID, []string{"192.0.2.4", "10.10.0.8/16"})
	if err != nil {
		t.Fatalf("EnsureInlineIPGroup failed: %v", err)
	}
	if group.ID != groupID || group.Name != name || group.Kind != IPGroupKindInline || len(group.Members) != 2 {
		t.Fatalf("unexpected inline group: %#v", group)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureInlineIPGroupRejectsExactDuplicateCIDRAgainstExistingCustomGroup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	inlineGroupID := uuid.New()
	existingCustomGroupID := uuid.New()
	now := time.Now()
	name := inlineIPGroupName([]string{"10.10.0.0/16"})

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(upsertInlineIPGroupSQL)).
		WithArgs(tenantID, name, "inline policy group", IPGroupKindInline).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "description", "kind", "created_by", "created_at", "updated_at",
		}).AddRow(inlineGroupID, tenantID, name, "inline policy group", IPGroupKindInline, sql.NullString{}, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(exactDuplicateIPGroupMemberSQL)).
		WithArgs(tenantID, inlineGroupID, "10.10.0.0/16").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "cidr"}).
			AddRow(existingCustomGroupID, "office", "10.10.0.0/16"))
	mock.ExpectRollback()

	_, err = store.EnsureInlineIPGroup(tenantID, []string{"10.10.0.8/16"})
	if err == nil || !strings.Contains(err.Error(), "duplicate CIDR") {
		t.Fatalf("expected duplicate CIDR error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestDeleteIPGroupRejectsReferencedGroup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	groupID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(referencedIPGroupSQL)).
		WithArgs(tenantID, groupID).
		WillReturnRows(sqlmock.NewRows([]string{"referenced"}).AddRow(true))

	err = store.DeleteIPGroup(tenantID, groupID)
	if err == nil || !strings.Contains(err.Error(), "ip group is referenced by policy rules") {
		t.Fatalf("expected referenced error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestFindIPGroupOverlapWarningsReportsLPMResolution(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	groupID := uuid.New()
	overlapID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(overlappingIPGroupsSQL)).
		WithArgs(tenantID, groupID, "10.10.1.0/24").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "cidr"}).
			AddRow(overlapID, "office", "10.10.0.0/16"))

	warnings, err := store.FindIPGroupOverlapWarnings(tenantID, groupID, []IPGroupMemberRecord{{CIDR: "10.10.1.0/24"}})
	if err != nil {
		t.Fatalf("FindIPGroupOverlapWarnings failed: %v", err)
	}
	if len(warnings) != 1 || warnings[0].Type != "overlap" || warnings[0].Resolution != "longest_prefix_wins" {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func expectNoExactDuplicateIPGroupMember(mock sqlmock.Sqlmock, tenantID, excludeGroupID uuid.UUID, cidr string) {
	mock.ExpectQuery(regexp.QuoteMeta(exactDuplicateIPGroupMemberSQL)).
		WithArgs(tenantID, excludeGroupID, cidr).
		WillReturnError(sql.ErrNoRows)
}
