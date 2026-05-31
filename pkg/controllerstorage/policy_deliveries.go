package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PolicyDelivery struct {
	ID            uuid.UUID              `json:"id"`
	TenantID      uuid.UUID              `json:"tenant_id"`
	NodeID        uuid.UUID              `json:"node_id"`
	PolicyDomain  string                 `json:"policy_domain"`
	PolicyRef     string                 `json:"policy_ref"`
	PolicyName    string                 `json:"policy_name,omitempty"`
	Action        string                 `json:"action"`
	CommandID     string                 `json:"command_id"`
	CommandStatus string                 `json:"command_status"`
	LastError     string                 `json:"last_error,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
}

func (s *Storage) CreatePolicyDelivery(delivery *PolicyDelivery) (*PolicyDelivery, error) {
	if delivery.Metadata == nil {
		delivery.Metadata = map[string]interface{}{}
	}

	metadataJSON, err := json.Marshal(delivery.Metadata)
	if err != nil {
		return nil, err
	}

	created := &PolicyDelivery{}
	var (
		rawMetadata []byte
		completedAt sql.NullTime
	)

	err = s.db.QueryRow(`
		INSERT INTO policy_deliveries (
			tenant_id,
			node_id,
			policy_domain,
			policy_ref,
			policy_name,
			action,
			command_id,
			command_status,
			last_error,
			metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, tenant_id, node_id, policy_domain, policy_ref, COALESCE(policy_name, ''), action, command_id, command_status,
		          COALESCE(last_error, ''), metadata, created_at, updated_at, completed_at
	`,
		delivery.TenantID,
		delivery.NodeID,
		delivery.PolicyDomain,
		delivery.PolicyRef,
		delivery.PolicyName,
		delivery.Action,
		delivery.CommandID,
		delivery.CommandStatus,
		delivery.LastError,
		metadataJSON,
	).Scan(
		&created.ID,
		&created.TenantID,
		&created.NodeID,
		&created.PolicyDomain,
		&created.PolicyRef,
		&created.PolicyName,
		&created.Action,
		&created.CommandID,
		&created.CommandStatus,
		&created.LastError,
		&rawMetadata,
		&created.CreatedAt,
		&created.UpdatedAt,
		&completedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(rawMetadata) > 0 {
		if err := json.Unmarshal(rawMetadata, &created.Metadata); err != nil {
			return nil, err
		}
	}
	if created.Metadata == nil {
		created.Metadata = map[string]interface{}{}
	}

	if completedAt.Valid {
		created.CompletedAt = &completedAt.Time
	}

	return created, nil
}

func (s *Storage) UpdatePolicyDeliveryStatusByCommand(commandID, status, message string) error {
	_, err := s.db.Exec(`
		UPDATE policy_deliveries
		SET command_status = $2::varchar,
		    last_error = CASE
		        WHEN $2::varchar = 'failed' THEN $3
		        WHEN $2::varchar = 'completed' THEN ''
		        ELSE last_error
		    END,
		    updated_at = NOW(),
		    completed_at = CASE
		        WHEN $2::varchar IN ('completed', 'failed') THEN NOW()
		        ELSE completed_at
		    END
		WHERE command_id = $1
	`, commandID, status, message)
	return err
}

func (s *Storage) ListPolicyDeliveriesByNodeAndDomain(tenantID, nodeID uuid.UUID, domain string, limit int) ([]*PolicyDelivery, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, tenant_id, node_id, policy_domain, policy_ref, COALESCE(policy_name, ''), action, command_id, command_status,
		       COALESCE(last_error, ''), metadata, created_at, updated_at, completed_at
		FROM policy_deliveries
		WHERE tenant_id = $1 AND node_id = $2 AND policy_domain = $3
		ORDER BY created_at DESC
		LIMIT $4
	`, tenantID, nodeID, domain, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deliveries := make([]*PolicyDelivery, 0, limit)
	for rows.Next() {
		delivery, err := scanPolicyDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return deliveries, nil
}

func scanPolicyDelivery(row interface {
	Scan(dest ...interface{}) error
}) (*PolicyDelivery, error) {
	var (
		delivery    PolicyDelivery
		rawMetadata []byte
		completedAt sql.NullTime
	)

	if err := row.Scan(
		&delivery.ID,
		&delivery.TenantID,
		&delivery.NodeID,
		&delivery.PolicyDomain,
		&delivery.PolicyRef,
		&delivery.PolicyName,
		&delivery.Action,
		&delivery.CommandID,
		&delivery.CommandStatus,
		&delivery.LastError,
		&rawMetadata,
		&delivery.CreatedAt,
		&delivery.UpdatedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}

	if len(rawMetadata) > 0 {
		if err := json.Unmarshal(rawMetadata, &delivery.Metadata); err != nil {
			return nil, err
		}
	}
	if delivery.Metadata == nil {
		delivery.Metadata = map[string]interface{}{}
	}

	if completedAt.Valid {
		delivery.CompletedAt = &completedAt.Time
	}

	return &delivery, nil
}
