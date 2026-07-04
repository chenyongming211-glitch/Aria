package controllerstorage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	IPGroupKindCustom = "custom"
	IPGroupKindInline = "inline"
	IPGroupKindSystem = "system"
	IPGroupAnyName    = "any"
)

const exactDuplicateIPGroupMemberSQL = `SELECT g.id, g.name, m.cidr::text
		   FROM ip_group_members m
		   JOIN ip_groups g ON g.id = m.group_id
		  WHERE m.tenant_id = $1
		    AND g.id <> $2
		    AND m.cidr = $3::cidr
		  LIMIT 1`

const insertIPGroupSQL = `INSERT INTO ip_groups (tenant_id, name, description, kind, created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, name, COALESCE(description, ''), kind, created_by, created_at, updated_at`

const upsertInlineIPGroupSQL = `INSERT INTO ip_groups (tenant_id, name, description, kind)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, name) DO UPDATE SET updated_at = NOW()
		 RETURNING id, tenant_id, name, COALESCE(description, ''), kind, created_by, created_at, updated_at`

const insertIPGroupMemberSQL = `INSERT INTO ip_group_members (tenant_id, group_id, cidr, note)
		 VALUES ($1, $2, $3::cidr, $4)`

const upsertIPGroupMemberSQL = `INSERT INTO ip_group_members (tenant_id, group_id, cidr, note)
		 VALUES ($1, $2, $3::cidr, $4)
		 ON CONFLICT (group_id, cidr) DO UPDATE SET note = EXCLUDED.note`

const referencedIPGroupSQL = `SELECT EXISTS (
		SELECT 1 FROM acl_rules
		 WHERE tenant_id = $1 AND (src_group_id = $2 OR dst_group_id = $2)
		UNION ALL
		SELECT 1 FROM qos_rules
		 WHERE tenant_id = $1 AND group_id = $2
	) AS referenced`

const overlappingIPGroupsSQL = `SELECT g.id, g.name, m.cidr::text
	   FROM ip_group_members m
	   JOIN ip_groups g ON g.id = m.group_id
	  WHERE m.tenant_id = $1
	    AND g.id <> $2
	    AND m.cidr && $3::cidr
	  ORDER BY masklen(m.cidr) DESC, g.name ASC`

type IPGroupRecord struct {
	ID          uuid.UUID             `json:"id"`
	TenantID    uuid.UUID             `json:"tenant_id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Kind        string                `json:"kind"`
	CreatedBy   sql.NullString        `json:"created_by"`
	Members     []IPGroupMemberRecord `json:"members"`
	Warnings    []IPGroupWarning      `json:"warnings,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

type IPGroupMemberRecord struct {
	ID      uuid.UUID `json:"id"`
	GroupID uuid.UUID `json:"group_id"`
	CIDR    string    `json:"cidr"`
	Note    string    `json:"note"`
}

type IPGroupWarning struct {
	Type              string `json:"type"`
	CIDR              string `json:"cidr"`
	OverlapsGroupID   string `json:"overlaps_group_id"`
	OverlapsGroupName string `json:"overlaps_group_name"`
	OverlapsCIDR      string `json:"overlaps_cidr"`
	Resolution        string `json:"resolution"`
}

type ipGroupExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

func (s *Storage) CreateIPGroup(group *IPGroupRecord) (*IPGroupRecord, error) {
	if group == nil {
		return nil, fmt.Errorf("ip group cannot be nil")
	}
	normalized, err := normalizeIPGroupForWrite(group)
	if err != nil {
		return nil, err
	}
	if err := s.rejectExactDuplicateIPGroupMembers(normalized.TenantID, uuid.Nil, normalized.Members); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackIfOpen(tx)

	created := &IPGroupRecord{}
	err = tx.QueryRow(
		insertIPGroupSQL,
		normalized.TenantID,
		normalized.Name,
		normalized.Description,
		normalized.Kind,
		normalized.CreatedBy,
	).Scan(
		&created.ID,
		&created.TenantID,
		&created.Name,
		&created.Description,
		&created.Kind,
		&created.CreatedBy,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := insertIPGroupMembers(tx, created.TenantID, created.ID, normalized.Members, false); err != nil {
		return nil, err
	}
	created.Members = withIPGroupID(normalized.Members, created.ID)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Storage) ListIPGroups(tenantID uuid.UUID) ([]*IPGroupRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, name, COALESCE(description, ''), kind, created_by, created_at, updated_at
		   FROM ip_groups
		  WHERE tenant_id = $1
		  ORDER BY kind ASC, name ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]*IPGroupRecord, 0)
	for rows.Next() {
		group, err := scanIPGroup(rows)
		if err != nil {
			return nil, err
		}
		group.Members, err = s.listIPGroupMembers(tenantID, group.ID)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Storage) GetIPGroup(tenantID, groupID uuid.UUID) (*IPGroupRecord, error) {
	return getIPGroupWith(s.db, tenantID, groupID)
}

func getIPGroupWith(q ipGroupExecutor, tenantID, groupID uuid.UUID) (*IPGroupRecord, error) {
	group, err := scanIPGroup(q.QueryRow(
		`SELECT id, tenant_id, name, COALESCE(description, ''), kind, created_by, created_at, updated_at
		   FROM ip_groups
		  WHERE tenant_id = $1 AND id = $2`,
		tenantID,
		groupID,
	))
	if err != nil {
		return nil, err
	}
	group.Members, err = listIPGroupMembersWith(q, tenantID, group.ID)
	if err != nil {
		return nil, err
	}
	return group, nil
}

func (s *Storage) UpdateIPGroup(group *IPGroupRecord) (*IPGroupRecord, error) {
	if group == nil {
		return nil, fmt.Errorf("ip group cannot be nil")
	}
	return s.UpdateIPGroupByID(group.TenantID, group.ID, group)
}

func (s *Storage) UpdateIPGroupByID(tenantID, groupID uuid.UUID, group *IPGroupRecord) (*IPGroupRecord, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackIfOpen(tx)

	updated, err := updateIPGroupWith(tx, tenantID, groupID, group)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}

func updateIPGroupWith(q ipGroupExecutor, tenantID, groupID uuid.UUID, group *IPGroupRecord) (*IPGroupRecord, error) {
	if group == nil {
		return nil, fmt.Errorf("ip group cannot be nil")
	}
	group.TenantID = tenantID
	group.ID = groupID
	normalized, err := normalizeIPGroupForWrite(group)
	if err != nil {
		return nil, err
	}
	if err := rejectExactDuplicateIPGroupMembersWith(q, tenantID, groupID, normalized.Members); err != nil {
		return nil, err
	}

	updated := &IPGroupRecord{}
	err = q.QueryRow(
		`UPDATE ip_groups
		    SET name = $3, description = $4, kind = $5, updated_at = NOW()
		  WHERE tenant_id = $1 AND id = $2
		  RETURNING id, tenant_id, name, COALESCE(description, ''), kind, created_by, created_at, updated_at`,
		tenantID,
		groupID,
		normalized.Name,
		normalized.Description,
		normalized.Kind,
	).Scan(
		&updated.ID,
		&updated.TenantID,
		&updated.Name,
		&updated.Description,
		&updated.Kind,
		&updated.CreatedBy,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if _, err := q.Exec(`DELETE FROM ip_group_members WHERE tenant_id = $1 AND group_id = $2`, tenantID, groupID); err != nil {
		return nil, err
	}
	if err := insertIPGroupMembers(q, tenantID, groupID, normalized.Members, false); err != nil {
		return nil, err
	}
	updated.Members = withIPGroupID(normalized.Members, groupID)
	return updated, nil
}

func (s *Storage) DeleteIPGroup(tenantID, groupID uuid.UUID) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer rollbackIfOpen(tx)

	var referenced bool
	if err := tx.QueryRow(referencedIPGroupSQL, tenantID, groupID).Scan(&referenced); err != nil {
		return err
	}
	if referenced {
		return fmt.Errorf("ip group is referenced by policy rules")
	}

	result, err := tx.Exec(`DELETE FROM ip_groups WHERE tenant_id = $1 AND id = $2`, tenantID, groupID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Storage) EnsureInlineIPGroup(tenantID uuid.UUID, cidrs []string) (*IPGroupRecord, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackIfOpen(tx)

	group, err := ensureInlineIPGroupWith(tx, tenantID, cidrs)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *Storage) ResolvePolicyGroupRef(tenantID uuid.UUID, explicit uuid.NullUUID, directCIDR string) (uuid.NullUUID, error) {
	return resolvePolicyGroupRefWith(s.db, tenantID, explicit, directCIDR)
}

func (s *Storage) ListIPGroupsForNodePolicySnapshot(tenantID, nodeID uuid.UUID) ([]*IPGroupRecord, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT g.id, g.tenant_id, g.name, COALESCE(g.description, ''), g.kind, g.created_by, g.created_at, g.updated_at
		   FROM ip_groups g
		  WHERE g.tenant_id = $1
		    AND g.id IN (
			SELECT src_group_id FROM acl_rules
			 WHERE tenant_id = $1 AND node_id = $2 AND enabled = true AND src_group_id IS NOT NULL
			UNION
			SELECT dst_group_id FROM acl_rules
			 WHERE tenant_id = $1 AND node_id = $2 AND enabled = true AND dst_group_id IS NOT NULL
			UNION
			SELECT group_id FROM qos_rules
			 WHERE tenant_id = $1 AND node_id = $2 AND enabled = true AND group_id IS NOT NULL
		    )
		  ORDER BY g.kind ASC, g.name ASC, g.id ASC`,
		tenantID,
		nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]*IPGroupRecord, 0)
	for rows.Next() {
		group, err := scanIPGroup(rows)
		if err != nil {
			return nil, err
		}
		group.Members, err = s.listIPGroupMembers(tenantID, group.ID)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func resolvePolicyGroupRefWith(q ipGroupExecutor, tenantID uuid.UUID, explicit uuid.NullUUID, directCIDR string) (uuid.NullUUID, error) {
	if explicit.Valid {
		var found uuid.UUID
		if err := q.QueryRow(`SELECT id FROM ip_groups WHERE tenant_id = $1 AND id = $2`, tenantID, explicit.UUID).Scan(&found); err != nil {
			return uuid.NullUUID{}, err
		}
		return uuid.NullUUID{UUID: found, Valid: true}, nil
	}
	if policyCIDRIsAny(directCIDR) {
		return uuid.NullUUID{}, nil
	}
	group, err := ensureInlineIPGroupWith(q, tenantID, []string{directCIDR})
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: group.ID, Valid: true}, nil
}

func ensureInlineIPGroupWith(q ipGroupExecutor, tenantID uuid.UUID, cidrs []string) (*IPGroupRecord, error) {
	members := make([]IPGroupMemberRecord, 0, len(cidrs))
	for _, cidr := range cidrs {
		members = append(members, IPGroupMemberRecord{CIDR: cidr, Note: "inline"})
	}
	normalizedMembers, err := normalizeIPGroupMembers(members)
	if err != nil {
		return nil, err
	}
	if len(normalizedMembers) == 0 {
		return nil, fmt.Errorf("inline ip group requires at least one CIDR")
	}
	normalizedCIDRs := make([]string, 0, len(normalizedMembers))
	for _, member := range normalizedMembers {
		normalizedCIDRs = append(normalizedCIDRs, member.CIDR)
	}

	group := &IPGroupRecord{}
	err = q.QueryRow(
		upsertInlineIPGroupSQL,
		tenantID,
		inlineIPGroupName(normalizedCIDRs),
		"inline policy group",
		IPGroupKindInline,
	).Scan(
		&group.ID,
		&group.TenantID,
		&group.Name,
		&group.Description,
		&group.Kind,
		&group.CreatedBy,
		&group.CreatedAt,
		&group.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := rejectExactDuplicateIPGroupMembersWith(q, tenantID, group.ID, normalizedMembers); err != nil {
		return nil, err
	}

	if err := insertIPGroupMembers(q, tenantID, group.ID, normalizedMembers, true); err != nil {
		return nil, err
	}
	group.Members = withIPGroupID(normalizedMembers, group.ID)
	return group, nil
}

func (s *Storage) FindIPGroupOverlapWarnings(tenantID, excludeGroupID uuid.UUID, members []IPGroupMemberRecord) ([]IPGroupWarning, error) {
	normalizedMembers, err := normalizeIPGroupMembers(members)
	if err != nil {
		return nil, err
	}
	warnings := make([]IPGroupWarning, 0)
	for _, member := range normalizedMembers {
		rows, err := s.db.Query(overlappingIPGroupsSQL, tenantID, excludeGroupID, member.CIDR)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var overlapsGroupID uuid.UUID
			var overlapsName string
			var overlapsCIDR string
			if err := rows.Scan(&overlapsGroupID, &overlapsName, &overlapsCIDR); err != nil {
				_ = rows.Close()
				return nil, err
			}
			warnings = append(warnings, IPGroupWarning{
				Type:              "overlap",
				CIDR:              member.CIDR,
				OverlapsGroupID:   overlapsGroupID.String(),
				OverlapsGroupName: overlapsName,
				OverlapsCIDR:      overlapsCIDR,
				Resolution:        "longest_prefix_wins",
			})
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

func (s *Storage) rejectExactDuplicateIPGroupMembers(tenantID, excludeGroupID uuid.UUID, members []IPGroupMemberRecord) error {
	return rejectExactDuplicateIPGroupMembersWith(s.db, tenantID, excludeGroupID, members)
}

func rejectExactDuplicateIPGroupMembersWith(q ipGroupExecutor, tenantID, excludeGroupID uuid.UUID, members []IPGroupMemberRecord) error {
	for _, member := range members {
		var existingID uuid.UUID
		var existingName string
		var existingCIDR string
		err := q.QueryRow(exactDuplicateIPGroupMemberSQL, tenantID, excludeGroupID, member.CIDR).Scan(&existingID, &existingName, &existingCIDR)
		if err == nil {
			return fmt.Errorf("duplicate CIDR %s already belongs to IP group %s (%s)", existingCIDR, existingName, existingID)
		}
		if err != sql.ErrNoRows {
			return err
		}
	}
	return nil
}

func (s *Storage) listIPGroupMembers(tenantID, groupID uuid.UUID) ([]IPGroupMemberRecord, error) {
	return listIPGroupMembersWith(s.db, tenantID, groupID)
}

func (s *Storage) ListNodesReferencingIPGroup(tenantID, groupID uuid.UUID) ([]*Node, error) {
	return s.listNodesReferencingIPGroupWith(s.db, tenantID, groupID)
}

func (s *Storage) listNodesReferencingIPGroupWith(q ipGroupExecutor, tenantID, groupID uuid.UUID) ([]*Node, error) {
	rows, err := q.Query(
		`SELECT `+nodeSelectColumns+`
		   FROM nodes
		  WHERE tenant_id = $1
		    AND status NOT IN ('deleted', 'suspended', 'banned')
		    AND (
		      EXISTS (
		        SELECT 1 FROM acl_rules
		         WHERE tenant_id = $1
		           AND node_id = nodes.id
		           AND (src_group_id = $2 OR dst_group_id = $2)
		      )
		      OR EXISTS (
		        SELECT 1 FROM qos_rules
		         WHERE tenant_id = $1
		           AND node_id = nodes.id
		           AND group_id = $2
		      )
		    )
		  ORDER BY hostname ASC, public_key ASC`,
		tenantID,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make([]*Node, 0)
	for rows.Next() {
		node, err := s.scanNodeRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func listIPGroupMembersWith(q ipGroupExecutor, tenantID, groupID uuid.UUID) ([]IPGroupMemberRecord, error) {
	rows, err := q.Query(
		`SELECT id, group_id, cidr::text, COALESCE(note, '')
		   FROM ip_group_members
		  WHERE tenant_id = $1 AND group_id = $2
		  ORDER BY cidr::text ASC`,
		tenantID,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]IPGroupMemberRecord, 0)
	for rows.Next() {
		member := IPGroupMemberRecord{}
		if err := rows.Scan(&member.ID, &member.GroupID, &member.CIDR, &member.Note); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func insertIPGroupMembers(q ipGroupExecutor, tenantID, groupID uuid.UUID, members []IPGroupMemberRecord, upsert bool) error {
	query := insertIPGroupMemberSQL
	if upsert {
		query = upsertIPGroupMemberSQL
	}
	for _, member := range members {
		if _, err := q.Exec(query, tenantID, groupID, member.CIDR, strings.TrimSpace(member.Note)); err != nil {
			return err
		}
	}
	return nil
}

func normalizeIPGroupForWrite(group *IPGroupRecord) (*IPGroupRecord, error) {
	normalized := *group
	normalized.Name = normalizeIPGroupName(group.Name)
	normalized.Description = strings.TrimSpace(group.Description)
	normalized.Kind = normalizeIPGroupKind(group.Kind)
	if normalized.TenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if normalized.Name == "" {
		return nil, fmt.Errorf("ip group name is required")
	}
	if err := validateIPGroupKind(normalized.Kind); err != nil {
		return nil, err
	}
	members, err := normalizeIPGroupMembers(group.Members)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 && !(normalized.Kind == IPGroupKindSystem && normalized.Name == IPGroupAnyName) {
		return nil, fmt.Errorf("ip group requires at least one CIDR member")
	}
	normalized.Members = members
	return &normalized, nil
}

func normalizeIPGroupKind(kind string) string {
	trimmed := strings.ToLower(strings.TrimSpace(kind))
	if trimmed == "" {
		return IPGroupKindCustom
	}
	return trimmed
}

func validateIPGroupKind(kind string) error {
	switch kind {
	case IPGroupKindCustom, IPGroupKindInline, IPGroupKindSystem:
		return nil
	default:
		return fmt.Errorf("unsupported ip group kind: %s", kind)
	}
}

func normalizeIPGroupName(name string) string {
	return strings.TrimSpace(name)
}

func normalizeIPGroupMembers(members []IPGroupMemberRecord) ([]IPGroupMemberRecord, error) {
	seen := map[string]struct{}{}
	normalized := make([]IPGroupMemberRecord, 0, len(members))
	for _, member := range members {
		cidr, err := normalizeCIDR(member.CIDR)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[cidr]; ok {
			continue
		}
		seen[cidr] = struct{}{}
		member.CIDR = cidr
		member.Note = strings.TrimSpace(member.Note)
		normalized = append(normalized, member)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].CIDR < normalized[j].CIDR
	})
	return normalized, nil
}

func normalizeCIDR(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "any") || trimmed == "0" {
		return "0.0.0.0/0", nil
	}
	prefix, err := netip.ParsePrefix(trimmed)
	if err == nil {
		return prefix.Masked().String(), nil
	}
	addr, err := netip.ParseAddr(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR %q", value)
	}
	if addr.Is4() {
		return netip.PrefixFrom(addr, 32).String(), nil
	}
	return netip.PrefixFrom(addr, 128).String(), nil
}

func inlineIPGroupName(cidrs []string) string {
	copyCIDRs := append([]string(nil), cidrs...)
	sort.Strings(copyCIDRs)
	sum := sha256.Sum256([]byte(strings.Join(copyCIDRs, ",")))
	return "inline:" + hex.EncodeToString(sum[:8])
}

func withIPGroupID(members []IPGroupMemberRecord, groupID uuid.UUID) []IPGroupMemberRecord {
	copied := make([]IPGroupMemberRecord, 0, len(members))
	for _, member := range members {
		member.GroupID = groupID
		copied = append(copied, member)
	}
	return copied
}

func scanIPGroup(scanner interface {
	Scan(dest ...interface{}) error
}) (*IPGroupRecord, error) {
	group := &IPGroupRecord{}
	err := scanner.Scan(
		&group.ID,
		&group.TenantID,
		&group.Name,
		&group.Description,
		&group.Kind,
		&group.CreatedBy,
		&group.CreatedAt,
		&group.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return group, nil
}

func rollbackIfOpen(tx *sql.Tx) {
	rollbackTx(tx, "IP group transaction")
}
