package controllerstorage

import "time"

type CertificateRenewalCandidate struct {
	TenantID     string
	NodeID       string
	Hostname     string
	SerialNumber string
	NotAfter     time.Time
}

func (s *Storage) MarkExpiredNodeCertificates(now time.Time) ([]*NodeCertificate, error) {
	rows, err := s.db.Query(`
		UPDATE node_certificates
		SET status = $2,
		    updated_at = NOW()
		WHERE status = $3 AND not_after < $1
		RETURNING id, tenant_id, node_id, serial_number, cert_pem, ca_pem,
		          not_before, not_after, status, issued_at, revoked_at,
		          COALESCE(revoke_reason, ''), renewed_from, updated_at
	`, now.UTC(), CertStatusExpired, CertStatusIssued)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []*NodeCertificate
	for rows.Next() {
		cert, err := scanNodeCertificate(rows)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return certs, nil
}

func (s *Storage) ListCertificatesExpiringBefore(deadline time.Time) ([]*CertificateRenewalCandidate, error) {
	rows, err := s.db.Query(`
		SELECT c.tenant_id::text, c.node_id::text, COALESCE(n.hostname, ''), c.serial_number, c.not_after
		FROM node_certificates c
		JOIN nodes n ON n.id = c.node_id
		WHERE c.status = $1
		  AND c.not_after <= $2
		  AND c.not_after >= NOW()
		  AND COALESCE(n.status, 'online') NOT IN ('deleted', 'suspended', 'banned')
		ORDER BY c.not_after ASC
	`, CertStatusIssued, deadline.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []*CertificateRenewalCandidate
	for rows.Next() {
		candidate := &CertificateRenewalCandidate{}
		if err := rows.Scan(
			&candidate.TenantID,
			&candidate.NodeID,
			&candidate.Hostname,
			&candidate.SerialNumber,
			&candidate.NotAfter,
		); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}
