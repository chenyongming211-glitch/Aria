package controllerstorage

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestMarkExpiredNodeCertificatesUpdatesIssuedRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.New()
	certID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE node_certificates
		SET status = $2,
		    updated_at = NOW()
		WHERE status = $3 AND not_after < $1
		RETURNING id, tenant_id, node_id, serial_number, cert_pem, ca_pem,
		          not_before, not_after, status, issued_at, revoked_at,
		          COALESCE(revoke_reason, ''), renewed_from, updated_at
	`)).
		WithArgs(now, CertStatusExpired, CertStatusIssued).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "serial_number", "cert_pem", "ca_pem",
			"not_before", "not_after", "status", "issued_at", "revoked_at",
			"revoke_reason", "renewed_from", "updated_at",
		}).AddRow(
			certID, tenantID, nodeID, "expired-serial", "cert", "ca",
			now.Add(-48*time.Hour), now.Add(-time.Hour), CertStatusExpired, now.Add(-48*time.Hour), nil, "", nil, now,
		))

	certs, err := store.MarkExpiredNodeCertificates(now)
	if err != nil {
		t.Fatalf("MarkExpiredNodeCertificates failed: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 expired cert, got %d", len(certs))
	}
	if certs[0].Status != CertStatusExpired {
		t.Fatalf("expected status %q, got %q", CertStatusExpired, certs[0].Status)
	}
	if certs[0].NodeID != nodeID {
		t.Fatalf("expected node id %s, got %s", nodeID, certs[0].NodeID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListCertificatesExpiringBeforeReturnsIssuedCandidates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	nodeID := uuid.New()
	deadline := time.Now().UTC().Add(72 * time.Hour)
	notAfter := time.Now().UTC().Add(2 * time.Hour)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT c.tenant_id::text, c.node_id::text, COALESCE(n.hostname, ''), c.serial_number, c.not_after
		FROM node_certificates c
		JOIN nodes n ON n.id = c.node_id
		WHERE c.status = $1
		  AND c.not_after <= $2
		  AND COALESCE(n.status, 'online') NOT IN ('deleted', 'suspended', 'banned')
		ORDER BY c.not_after ASC
	`)).
		WithArgs(CertStatusIssued, deadline).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "node_id", "hostname", "serial_number", "not_after"}).
			AddRow(tenantID.String(), nodeID.String(), "edge-1", "serial-1", notAfter))

	candidates, err := store.ListCertificatesExpiringBefore(deadline)
	if err != nil {
		t.Fatalf("ListCertificatesExpiringBefore failed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Hostname != "edge-1" {
		t.Fatalf("expected hostname edge-1, got %q", candidates[0].Hostname)
	}
	if candidates[0].NodeID != nodeID.String() {
		t.Fatalf("expected node id %s, got %q", nodeID, candidates[0].NodeID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListCertificatesExpiringBeforeIncludesExpiredIssuedCandidates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	nodeID := uuid.New()
	deadline := time.Now().UTC().Add(72 * time.Hour)
	notAfter := time.Now().UTC().Add(-2 * time.Hour)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT c.tenant_id::text, c.node_id::text, COALESCE(n.hostname, ''), c.serial_number, c.not_after
		FROM node_certificates c
		JOIN nodes n ON n.id = c.node_id
		WHERE c.status = $1
		  AND c.not_after <= $2
		  AND COALESCE(n.status, 'online') NOT IN ('deleted', 'suspended', 'banned')
		ORDER BY c.not_after ASC
	`)).
		WithArgs(CertStatusIssued, deadline).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "node_id", "hostname", "serial_number", "not_after"}).
			AddRow(tenantID.String(), nodeID.String(), "edge-expired", "expired-issued", notAfter))

	candidates, err := store.ListCertificatesExpiringBefore(deadline)
	if err != nil {
		t.Fatalf("ListCertificatesExpiringBefore failed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].SerialNumber != "expired-issued" {
		t.Fatalf("expected expired issued certificate candidate, got %#v", candidates[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
