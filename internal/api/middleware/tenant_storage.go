package middleware

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"aria/pkg/controllerstorage"
)

type TenantScopedStorage struct {
	baseStorage *controllerstorage.Storage
}

func NewTenantScopedStorage(base *controllerstorage.Storage) *TenantScopedStorage {
	return &TenantScopedStorage{
		baseStorage: base,
	}
}

func (ts *TenantScopedStorage) GetTenantNodes(ctx context.Context) ([]*controllerstorage.Node, error) {
	tenantID, exists := GetTenantID(ctx)
	if !exists {
		return nil, fmt.Errorf("tenant ID not found in context")
	}

	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 ORDER BY last_seen DESC`

	rows, err := ts.baseStorage.DB().Query(query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*controllerstorage.Node
	for rows.Next() {
		node, err := ts.baseStorage.ScanNodeRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (ts *TenantScopedStorage) ScanNode(row *sql.Row) (*controllerstorage.Node, error) {
	return ts.baseStorage.ScanNode(row)
}

func (ts *TenantScopedStorage) ScanNodeRows(rows *sql.Rows) (*controllerstorage.Node, error) {
	return ts.baseStorage.ScanNodeRows(rows)
}

func (ts *TenantScopedStorage) GetTenantNode(ctx context.Context, publicKey string) (*controllerstorage.Node, error) {
	tenantID, exists := GetTenantID(ctx)
	if !exists {
		return nil, fmt.Errorf("tenant ID not found in context")
	}

	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1 AND tenant_id = $2`

	row := ts.baseStorage.DB().QueryRow(query, publicKey, tenantID)
	return ts.baseStorage.ScanNode(row)
}

func (ts *TenantScopedStorage) SaveNodeWithTenant(ctx context.Context, node *controllerstorage.Node) error {
	tenantID, exists := GetTenantID(ctx)
	if !exists {
		return fmt.Errorf("tenant ID not found in context")
	}

	node.TenantID = tenantID
	return ts.baseStorage.SaveNode(node)
}

func (ts *TenantScopedStorage) GetTenantIDByToken(tokenStr string) (uuid.UUID, error) {
	var tenantID uuid.UUID
	err := ts.baseStorage.DB().QueryRow(
		`SELECT tenant_id FROM tokens WHERE token = $1`,
		tokenStr,
	).Scan(&tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	return tenantID, nil
}

func (ts *TenantScopedStorage) GetNode(publicKey string) (*controllerstorage.Node, error) {
	return ts.baseStorage.GetNode(publicKey)
}

func (ts *TenantScopedStorage) GetNextAvailableOffset() (int, error) {
	return ts.baseStorage.GetNextAvailableOffset()
}

func (ts *TenantScopedStorage) CalculateIP(offset int) (string, error) {
	return ts.baseStorage.CalculateIP(offset)
}

func (ts *TenantScopedStorage) DeleteNode(publicKey string) error {
	return ts.baseStorage.DeleteNode(publicKey)
}

func (ts *TenantScopedStorage) SetHeartbeatStore(hb *controllerstorage.HeartbeatStore) {
	ts.baseStorage.SetHeartbeatStore(hb)
}

func (ts *TenantScopedStorage) HasHeartbeat() bool {
	return ts.baseStorage.HasHeartbeat()
}

func (ts *TenantScopedStorage) GetAllNodes(ctx context.Context) ([]*controllerstorage.Node, error) {
	tenantID, exists := GetTenantID(ctx)
	// ✅ 修复：坚决阻断在无上下文时泄露全网节点数据的可能
	if !exists {
		return nil, fmt.Errorf("tenant ID not found in context, rejecting full dump request")
	}

	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 ORDER BY last_seen DESC`
	rows, err := ts.baseStorage.DB().Query(query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*controllerstorage.Node
	for rows.Next() {
		node, err := ts.baseStorage.ScanNodeRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (ts *TenantScopedStorage) GetNodeCount(ctx context.Context) (int, error) {
	tenantID, exists := GetTenantID(ctx)
	if !exists {
		return 0, fmt.Errorf("tenant ID not found in context")
	}

	var count int
	query := `SELECT COUNT(*) FROM nodes WHERE tenant_id = $1`
	err := ts.baseStorage.DB().QueryRow(query, tenantID).Scan(&count)
	return count, err
}

func (ts *TenantScopedStorage) GetTenantIDByPublicKey(publicKey string) (uuid.UUID, error) {
	var tenantID uuid.UUID
	query := `SELECT tenant_id FROM nodes WHERE public_key = $1`
	err := ts.baseStorage.DB().QueryRow(query, publicKey).Scan(&tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	return tenantID, nil
}

func (ts *TenantScopedStorage) UpdateTenantResourceQuota(tenantID uuid.UUID, quota string) error {
	query := `UPDATE tenants SET resource_quota = $1, updated_at = NOW() WHERE id = $2`
	_, err := ts.baseStorage.DB().Exec(query, quota, tenantID)
	return err
}

func (ts *TenantScopedStorage) GetTenantResourceQuota(tenantID uuid.UUID) (string, error) {
	var quota string
	query := `SELECT resource_quota FROM tenants WHERE id = $1`
	err := ts.baseStorage.DB().QueryRow(query, tenantID).Scan(&quota)
	return quota, err
}

func (ts *TenantScopedStorage) GetTenantInfo(tenantID uuid.UUID) (*controllerstorage.TenantInfo, error) {
	query := `SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`
	row := ts.baseStorage.DB().QueryRow(query, tenantID)

	var info controllerstorage.TenantInfo
	var code sql.NullString
	err := row.Scan(&info.ID, &info.Name, &code, &info.Status, &info.ResourceQuota, &info.CreatedAt, &info.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if code.Valid {
		info.Code = code.String
	}
	return &info, nil
}
