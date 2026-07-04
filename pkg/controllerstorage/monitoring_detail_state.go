package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type NodeMonitoringDetailState struct {
	ControlState *NodeControlState
	PolicyStats  *NodePolicyStats
	Certificate  *NodeCertificate
}

func (s *Storage) GetNodeMonitoringDetailState(tenantID, nodeID uuid.UUID) (*NodeMonitoringDetailState, error) {
	row := s.db.QueryRow(`
		WITH control AS (
			SELECT tenant_id::text AS tenant_id,
			       node_id::text AS node_id,
			       COALESCE(desired_state_version, '') AS desired_state_version,
			       desired_state_metadata,
			       desired_state_updated_at,
			       COALESCE(applied_state_version, '') AS applied_state_version,
			       applied_state_updated_at,
			       COALESCE(observed_state, '') AS observed_state,
			       COALESCE(observed_message, '') AS observed_message,
			       observed_at,
			       last_sync_at,
			       COALESCE(last_sync_error, '') AS last_sync_error,
			       created_at,
			       updated_at
			FROM node_control_states
			WHERE tenant_id = $1 AND node_id = $2
		),
		policy AS (
			SELECT tenant_id::text AS tenant_id,
			       node_id::text AS node_id,
			       stats,
			       updated_at
			FROM node_policy_stats
			WHERE tenant_id = $1 AND node_id = $2
		),
		certificate AS (
			SELECT id::text AS id,
			       tenant_id::text AS tenant_id,
			       node_id::text AS node_id,
			       serial_number,
			       cert_pem,
			       ca_pem,
			       not_before,
			       not_after,
			       status,
			       issued_at,
			       revoked_at,
			       COALESCE(revoke_reason, '') AS revoke_reason,
			       renewed_from::text AS renewed_from,
			       updated_at
			FROM node_certificates
			WHERE node_id = $2
		)
		SELECT control.tenant_id,
		       control.node_id,
		       control.desired_state_version,
		       control.desired_state_metadata,
		       control.desired_state_updated_at,
		       control.applied_state_version,
		       control.applied_state_updated_at,
		       control.observed_state,
		       control.observed_message,
		       control.observed_at,
		       control.last_sync_at,
		       control.last_sync_error,
		       control.created_at,
		       control.updated_at,
		       policy.tenant_id,
		       policy.node_id,
		       policy.stats,
		       policy.updated_at,
		       certificate.id,
		       certificate.tenant_id,
		       certificate.node_id,
		       certificate.serial_number,
		       certificate.cert_pem,
		       certificate.ca_pem,
		       certificate.not_before,
		       certificate.not_after,
		       certificate.status,
		       certificate.issued_at,
		       certificate.revoked_at,
		       certificate.revoke_reason,
		       certificate.renewed_from,
		       certificate.updated_at
		FROM (SELECT 1) seed
		LEFT JOIN control ON TRUE
		LEFT JOIN policy ON TRUE
		LEFT JOIN certificate ON TRUE
	`, tenantID, nodeID)

	state, err := scanNodeMonitoringDetailState(row)
	if err == sql.ErrNoRows {
		return &NodeMonitoringDetailState{}, nil
	}
	return state, err
}

func scanNodeMonitoringDetailState(row interface {
	Scan(dest ...interface{}) error
}) (*NodeMonitoringDetailState, error) {
	var (
		controlTenantID      sql.NullString
		controlNodeID        sql.NullString
		desiredStateVersion  sql.NullString
		desiredMetadata      []byte
		desiredUpdatedAt     sql.NullTime
		appliedStateVersion  sql.NullString
		appliedUpdatedAt     sql.NullTime
		observedState        sql.NullString
		observedMessage      sql.NullString
		observedAt           sql.NullTime
		lastSyncAt           sql.NullTime
		lastSyncError        sql.NullString
		controlCreatedAt     sql.NullTime
		controlUpdatedAt     sql.NullTime
		statsTenantID        sql.NullString
		statsNodeID          sql.NullString
		rawPolicyStats       []byte
		policyStatsUpdatedAt sql.NullTime
		certID               sql.NullString
		certTenantID         sql.NullString
		certNodeID           sql.NullString
		certSerialNumber     sql.NullString
		certPEM              sql.NullString
		caPEM                sql.NullString
		certNotBefore        sql.NullTime
		certNotAfter         sql.NullTime
		certStatus           sql.NullString
		certIssuedAt         sql.NullTime
		certRevokedAt        sql.NullTime
		certRevokeReason     sql.NullString
		certRenewedFrom      sql.NullString
		certUpdatedAt        sql.NullTime
	)

	if err := row.Scan(
		&controlTenantID,
		&controlNodeID,
		&desiredStateVersion,
		&desiredMetadata,
		&desiredUpdatedAt,
		&appliedStateVersion,
		&appliedUpdatedAt,
		&observedState,
		&observedMessage,
		&observedAt,
		&lastSyncAt,
		&lastSyncError,
		&controlCreatedAt,
		&controlUpdatedAt,
		&statsTenantID,
		&statsNodeID,
		&rawPolicyStats,
		&policyStatsUpdatedAt,
		&certID,
		&certTenantID,
		&certNodeID,
		&certSerialNumber,
		&certPEM,
		&caPEM,
		&certNotBefore,
		&certNotAfter,
		&certStatus,
		&certIssuedAt,
		&certRevokedAt,
		&certRevokeReason,
		&certRenewedFrom,
		&certUpdatedAt,
	); err != nil {
		return nil, err
	}

	result := &NodeMonitoringDetailState{}

	if controlNodeID.Valid {
		controlState, err := buildBundledControlState(
			controlTenantID,
			controlNodeID,
			desiredStateVersion,
			desiredMetadata,
			desiredUpdatedAt,
			appliedStateVersion,
			appliedUpdatedAt,
			observedState,
			observedMessage,
			observedAt,
			lastSyncAt,
			lastSyncError,
			controlCreatedAt,
			controlUpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		result.ControlState = controlState
	}

	if statsNodeID.Valid {
		policyStats, err := buildBundledPolicyStats(statsTenantID, statsNodeID, rawPolicyStats, policyStatsUpdatedAt)
		if err != nil {
			return nil, err
		}
		result.PolicyStats = policyStats
	}

	if certID.Valid {
		cert, err := buildBundledNodeCertificate(
			certID,
			certTenantID,
			certNodeID,
			certSerialNumber,
			certPEM,
			caPEM,
			certNotBefore,
			certNotAfter,
			certStatus,
			certIssuedAt,
			certRevokedAt,
			certRevokeReason,
			certRenewedFrom,
			certUpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		result.Certificate = cert
	}

	return result, nil
}

func buildBundledControlState(
	tenantIDValue sql.NullString,
	nodeIDValue sql.NullString,
	desiredStateVersion sql.NullString,
	rawMetadata []byte,
	desiredUpdatedAt sql.NullTime,
	appliedStateVersion sql.NullString,
	appliedUpdatedAt sql.NullTime,
	observedState sql.NullString,
	observedMessage sql.NullString,
	observedAt sql.NullTime,
	lastSyncAt sql.NullTime,
	lastSyncError sql.NullString,
	createdAt sql.NullTime,
	updatedAt sql.NullTime,
) (*NodeControlState, error) {
	tenantID, err := uuidFromRequiredNullString(tenantIDValue, "control tenant_id")
	if err != nil {
		return nil, err
	}
	nodeID, err := uuidFromRequiredNullString(nodeIDValue, "control node_id")
	if err != nil {
		return nil, err
	}

	metadata := map[string]interface{}{}
	if len(rawMetadata) > 0 {
		if err := json.Unmarshal(rawMetadata, &metadata); err != nil {
			return nil, fmt.Errorf("unmarshal control state metadata: %w", err)
		}
	}

	state := &NodeControlState{
		TenantID:             tenantID,
		NodeID:               nodeID,
		DesiredStateVersion:  nullStringValue(desiredStateVersion),
		DesiredStateMetadata: metadata,
		AppliedStateVersion:  nullStringValue(appliedStateVersion),
		ObservedState:        nullStringValue(observedState),
		ObservedMessage:      nullStringValue(observedMessage),
		LastSyncError:        nullStringValue(lastSyncError),
		CreatedAt:            createdAt.Time,
		UpdatedAt:            updatedAt.Time,
	}
	if desiredUpdatedAt.Valid {
		state.DesiredStateUpdatedAt = &desiredUpdatedAt.Time
	}
	if appliedUpdatedAt.Valid {
		state.AppliedStateUpdatedAt = &appliedUpdatedAt.Time
	}
	if observedAt.Valid {
		state.ObservedAt = &observedAt.Time
	}
	if lastSyncAt.Valid {
		state.LastSyncAt = &lastSyncAt.Time
	}
	return state, nil
}

func buildBundledPolicyStats(
	tenantIDValue sql.NullString,
	nodeIDValue sql.NullString,
	rawStats []byte,
	updatedAt sql.NullTime,
) (*NodePolicyStats, error) {
	tenantID, err := uuidFromRequiredNullString(tenantIDValue, "policy stats tenant_id")
	if err != nil {
		return nil, err
	}
	nodeID, err := uuidFromRequiredNullString(nodeIDValue, "policy stats node_id")
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{}
	if len(rawStats) > 0 {
		if err := json.Unmarshal(rawStats, &stats); err != nil {
			return nil, fmt.Errorf("unmarshal policy stats: %w", err)
		}
	}
	return &NodePolicyStats{
		TenantID:  tenantID,
		NodeID:    nodeID,
		Stats:     stats,
		UpdatedAt: updatedAt.Time,
	}, nil
}

func buildBundledNodeCertificate(
	idValue sql.NullString,
	tenantIDValue sql.NullString,
	nodeIDValue sql.NullString,
	serialNumber sql.NullString,
	certPEM sql.NullString,
	caPEM sql.NullString,
	notBefore sql.NullTime,
	notAfter sql.NullTime,
	status sql.NullString,
	issuedAt sql.NullTime,
	revokedAt sql.NullTime,
	revokeReason sql.NullString,
	renewedFromValue sql.NullString,
	updatedAt sql.NullTime,
) (*NodeCertificate, error) {
	id, err := uuidFromRequiredNullString(idValue, "certificate id")
	if err != nil {
		return nil, err
	}
	tenantID, err := uuidFromRequiredNullString(tenantIDValue, "certificate tenant_id")
	if err != nil {
		return nil, err
	}
	nodeID, err := uuidFromRequiredNullString(nodeIDValue, "certificate node_id")
	if err != nil {
		return nil, err
	}

	cert := &NodeCertificate{
		ID:           id,
		TenantID:     tenantID,
		NodeID:       nodeID,
		SerialNumber: nullStringValue(serialNumber),
		CertPEM:      nullStringValue(certPEM),
		CAPEM:        nullStringValue(caPEM),
		NotBefore:    notBefore.Time,
		NotAfter:     notAfter.Time,
		Status:       nullStringValue(status),
		IssuedAt:     issuedAt.Time,
		RevokeReason: nullStringValue(revokeReason),
		UpdatedAt:    updatedAt.Time,
	}
	if revokedAt.Valid {
		cert.RevokedAt = &revokedAt.Time
	}
	if renewedFromValue.Valid && strings.TrimSpace(renewedFromValue.String) != "" {
		renewedFrom, err := uuid.Parse(strings.TrimSpace(renewedFromValue.String))
		if err != nil {
			return nil, fmt.Errorf("parse certificate renewed_from: %w", err)
		}
		cert.RenewedFrom = &renewedFrom
	}
	return cert, nil
}

func uuidFromRequiredNullString(value sql.NullString, field string) (uuid.UUID, error) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return uuid.Nil, fmt.Errorf("missing %s", field)
	}
	id, err := uuid.Parse(strings.TrimSpace(value.String))
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse %s: %w", field, err)
	}
	return id, nil
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
