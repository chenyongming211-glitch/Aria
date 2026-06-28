package controllerstorage

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type IPGroupReferenceDelivery struct {
	ID        uuid.UUID `json:"id"`
	CommandID string    `json:"command_id"`
	Status    string    `json:"status"`
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type IPGroupReferenceRecord struct {
	Domain         string                    `json:"domain"`
	RuleID         uuid.UUID                 `json:"rule_id"`
	RuleName       string                    `json:"rule_name"`
	NodeID         uuid.UUID                 `json:"node_id"`
	NodeName       string                    `json:"node_name"`
	Direction      string                    `json:"direction"`
	Enabled        bool                      `json:"enabled"`
	LatestDelivery *IPGroupReferenceDelivery `json:"latest_delivery,omitempty"`
}

type IPGroupReferencesPage struct {
	Items   []*IPGroupReferenceRecord `json:"items"`
	Total   int                       `json:"total"`
	Limit   int                       `json:"limit"`
	Offset  int                       `json:"offset"`
	HasMore bool                      `json:"has_more"`
}

const listIPGroupReferencesSQL = `
WITH refs AS (
	SELECT 'acl' AS domain,
	       ar.id AS rule_id,
	       COALESCE(ar.name, '') AS rule_name,
	       ar.node_id,
	       n.hostname AS node_name,
	       ar.direction,
	       ar.enabled
	  FROM acl_rules ar
	  JOIN nodes n ON n.id = ar.node_id AND n.tenant_id = ar.tenant_id
	 WHERE ar.tenant_id = $1
	   AND (ar.src_group_id = $2 OR ar.dst_group_id = $2)
	UNION ALL
	SELECT 'qos' AS domain,
	       qr.id AS rule_id,
	       COALESCE(NULLIF(qr.description, ''), qr.id::text) AS rule_name,
	       qr.node_id,
	       n.hostname AS node_name,
	       qr.direction,
	       qr.enabled
	  FROM qos_rules qr
	  JOIN nodes n ON n.id = qr.node_id AND n.tenant_id = qr.tenant_id
	 WHERE qr.tenant_id = $1
	   AND qr.group_id = $2
)
SELECT refs.domain,
       refs.rule_id,
       refs.rule_name,
       refs.node_id,
       refs.node_name,
       refs.direction,
       refs.enabled,
       COUNT(*) OVER() AS total,
       pd.id AS delivery_id,
       pd.command_id,
       pd.command_status,
       COALESCE(pd.last_error, '') AS last_error,
       pd.created_at AS delivery_created_at
  FROM refs
  LEFT JOIN LATERAL (
	SELECT id, command_id, command_status, last_error, created_at
	  FROM policy_deliveries
	 WHERE tenant_id = $1
	   AND node_id = refs.node_id
	   AND policy_domain = refs.domain
	   AND policy_ref = refs.rule_id::text
	 ORDER BY created_at DESC
	 LIMIT 1
  ) pd ON true
 ORDER BY refs.domain, refs.rule_name, refs.rule_id
 LIMIT $3 OFFSET $4`

func (s *Storage) ListIPGroupReferences(ctx context.Context, tenantID, groupID uuid.UUID, limit, offset int) (*IPGroupReferencesPage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, listIPGroupReferencesSQL, tenantID, groupID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	page := &IPGroupReferencesPage{Limit: limit, Offset: offset}
	for rows.Next() {
		var rec IPGroupReferenceRecord
		var total int
		var deliveryID uuid.NullUUID
		var commandID, status, lastError sql.NullString
		var createdAt sql.NullTime
		if err := rows.Scan(
			&rec.Domain,
			&rec.RuleID,
			&rec.RuleName,
			&rec.NodeID,
			&rec.NodeName,
			&rec.Direction,
			&rec.Enabled,
			&total,
			&deliveryID,
			&commandID,
			&status,
			&lastError,
			&createdAt,
		); err != nil {
			return nil, err
		}
		page.Total = total
		if deliveryID.Valid {
			rec.LatestDelivery = &IPGroupReferenceDelivery{
				ID:        deliveryID.UUID,
				CommandID: commandID.String,
				Status:    status.String,
				LastError: lastError.String,
				CreatedAt: createdAt.Time,
			}
		}
		page.Items = append(page.Items, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	page.HasMore = page.Offset+len(page.Items) < page.Total
	return page, nil
}
