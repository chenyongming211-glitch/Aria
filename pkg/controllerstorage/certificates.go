package controllerstorage

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

const (
	CertStatusIssued  = "issued"
	CertStatusRevoked = "revoked"
	CertStatusExpired = "expired"
)

// NodeCertificate stores certificate lifecycle metadata for a node.
type NodeCertificate struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	NodeID       uuid.UUID
	SerialNumber string
	CertPEM      string
	CAPEM        string
	NotBefore    time.Time
	NotAfter     time.Time
	Status       string
	IssuedAt     time.Time
	RevokedAt    *time.Time
	RevokeReason string
	RenewedFrom  *uuid.UUID
	UpdatedAt    time.Time
}

func (s *Storage) UpsertNodeCertificate(cert *NodeCertificate) error {
	if cert == nil {
		return sql.ErrNoRows
	}

	if cert.Status == "" {
		cert.Status = CertStatusIssued
	}

	query := `
		INSERT INTO node_certificates (
			tenant_id, node_id, serial_number, cert_pem, ca_pem,
			not_before, not_after, status, issued_at, renewed_from, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, NOW())
		ON CONFLICT (node_id) DO UPDATE SET
			serial_number = EXCLUDED.serial_number,
			cert_pem = EXCLUDED.cert_pem,
			ca_pem = EXCLUDED.ca_pem,
			not_before = EXCLUDED.not_before,
			not_after = EXCLUDED.not_after,
			status = EXCLUDED.status,
			issued_at = NOW(),
			revoked_at = NULL,
			revoke_reason = '',
			renewed_from = EXCLUDED.renewed_from,
			updated_at = NOW()
	`

	_, err := s.db.Exec(
		query,
		cert.TenantID,
		cert.NodeID,
		cert.SerialNumber,
		cert.CertPEM,
		cert.CAPEM,
		cert.NotBefore,
		cert.NotAfter,
		cert.Status,
		cert.RenewedFrom,
	)
	return err
}

func (s *Storage) GetNodeCertificate(nodeID uuid.UUID) (*NodeCertificate, error) {
	row := s.db.QueryRow(`
		SELECT id, tenant_id, node_id, serial_number, cert_pem, ca_pem,
		       not_before, not_after, status, issued_at, revoked_at,
		       COALESCE(revoke_reason, ''), renewed_from, updated_at
		FROM node_certificates
		WHERE node_id = $1
	`, nodeID)

	cert, err := scanNodeCertificate(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cert, err
}

func (s *Storage) RevokeNodeCertificate(nodeID uuid.UUID, reason string) error {
	_, err := revokeNodeCertificatesTx(s.db, nodeID, reason)
	return err
}

func revokeNodeCertificatesTx(tx roleExec, nodeID uuid.UUID, reason string) (int64, error) {
	if reason == "" {
		reason = "node_lifecycle"
	}
	result, err := tx.Exec(`
		UPDATE node_certificates
		SET status = $2,
		    revoked_at = NOW(),
		    revoke_reason = $3,
		    updated_at = NOW()
		WHERE node_id = $1 AND status = $4
	`, nodeID, CertStatusRevoked, reason, CertStatusIssued)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func scanNodeCertificate(row interface{ Scan(dest ...interface{}) error }) (*NodeCertificate, error) {
	var cert NodeCertificate
	var revokedAt sql.NullTime
	var renewedFrom sql.NullString

	if err := row.Scan(
		&cert.ID,
		&cert.TenantID,
		&cert.NodeID,
		&cert.SerialNumber,
		&cert.CertPEM,
		&cert.CAPEM,
		&cert.NotBefore,
		&cert.NotAfter,
		&cert.Status,
		&cert.IssuedAt,
		&revokedAt,
		&cert.RevokeReason,
		&renewedFrom,
		&cert.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if revokedAt.Valid {
		cert.RevokedAt = &revokedAt.Time
	}
	if renewedFrom.Valid {
		id, err := uuid.Parse(renewedFrom.String)
		if err == nil {
			cert.RenewedFrom = &id
		}
	}
	return &cert, nil
}
