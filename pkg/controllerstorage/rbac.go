package controllerstorage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Role struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var (
	SystemRoleAdmin    = "admin"
	SystemRoleOperator = "operator"
	SystemRoleViewer   = "viewer"

	AdminPermissions = []string{
		"nodes:read", "nodes:write",
		"routes:read", "routes:write",
		"acls:read", "acls:write",
		"qos:read", "qos:write",
		"ip-groups:read", "ip-groups:write",
		"blacklist:read", "blacklist:write",
		"monitoring:read",
		"commands:write",
		"tokens:read", "tokens:write",
		"users:read", "users:write",
		"roles:read", "roles:write",
		"ai:use",
		"policies:read",
		"settings:read", "settings:write",
	}

	OperatorPermissions = []string{
		"nodes:read", "nodes:write",
		"routes:read", "routes:write",
		"acls:read", "acls:write",
		"qos:read", "qos:write",
		"ip-groups:read", "ip-groups:write",
		"blacklist:read", "blacklist:write",
		"monitoring:read",
		"commands:write",
		"ai:use",
		"policies:read",
	}

	ViewerPermissions = []string{
		"nodes:read",
		"routes:read",
		"acls:read",
		"qos:read",
		"ip-groups:read",
		"blacklist:read",
		"monitoring:read",
		"ai:use",
		"policies:read",
	}
)

func NormalizeRoleName(role string) string {
	roleName := strings.TrimSpace(role)
	lowerRoleName := strings.ToLower(roleName)
	switch lowerRoleName {
	case "member":
		return SystemRoleOperator
	case "owner":
		return SystemRoleAdmin
	case SystemRoleAdmin, SystemRoleOperator, SystemRoleViewer, "super_admin":
		return lowerRoleName
	default:
		return roleName
	}
}

type roleExec interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func (s *Storage) createSystemRoles(exec roleExec, tenantID uuid.UUID) error {
	systemRoles := []struct {
		name        string
		desc        string
		permissions []string
	}{
		{SystemRoleAdmin, "Full access to all resources", AdminPermissions},
		{SystemRoleOperator, "Network and policy management, no user management", OperatorPermissions},
		{SystemRoleViewer, "Read-only access to all resources", ViewerPermissions},
	}

	for _, sr := range systemRoles {
		_, err := exec.Exec(`
			INSERT INTO roles (tenant_id, name, description, is_system, permissions)
			VALUES ($1, $2, $3, true, $4)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				description = EXCLUDED.description,
				is_system = true,
				permissions = EXCLUDED.permissions,
				updated_at = NOW()
		`, tenantID, sr.name, sr.desc, pq.Array(sr.permissions))
		if err != nil {
			return fmt.Errorf("failed to create system role %s: %w", sr.name, err)
		}
	}
	return nil
}

func (s *Storage) CreateSystemRoles(tenantID uuid.UUID) error {
	return s.createSystemRoles(s.db, tenantID)
}

func (s *Storage) EnsureTenantRolesTx(tx *sql.Tx, tenantID uuid.UUID) error {
	return s.createSystemRoles(tx, tenantID)
}

func (s *Storage) GetRoleByName(tenantID uuid.UUID, name string) (*Role, error) {
	r := &Role{}
	err := s.db.QueryRow(`
		SELECT id, tenant_id, name, description, is_system, permissions, created_at, updated_at
		FROM roles WHERE tenant_id = $1 AND name = $2
	`, tenantID, name).Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.IsSystem,
		pq.Array(&r.Permissions), &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("role '%s' not found for tenant %s: %w", name, tenantID, err)
	}
	return r, nil
}

func (s *Storage) ListRoles(tenantID uuid.UUID) ([]*Role, error) {
	rows, err := s.db.Query(`
		SELECT id, tenant_id, name, description, is_system, permissions, created_at, updated_at
		FROM roles WHERE tenant_id = $1 ORDER BY is_system DESC, name ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		r := &Role{}
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.IsSystem,
			pq.Array(&r.Permissions), &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

func (s *Storage) GetRoleByID(tenantID, roleID uuid.UUID) (*Role, error) {
	r := &Role{}
	err := s.db.QueryRow(`
		SELECT id, tenant_id, name, description, is_system, permissions, created_at, updated_at
		FROM roles WHERE tenant_id = $1 AND id = $2
	`, tenantID, roleID).Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.IsSystem,
		pq.Array(&r.Permissions), &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}
	return r, nil
}

func (s *Storage) CreateRole(role *Role) (*Role, error) {
	r := &Role{}
	err := s.db.QueryRow(`
		INSERT INTO roles (tenant_id, name, description, is_system, permissions)
		VALUES ($1, $2, $3, false, $4)
		RETURNING id, tenant_id, name, description, is_system, permissions, created_at, updated_at
	`, role.TenantID, role.Name, role.Description, pq.Array(role.Permissions)).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.Description, &r.IsSystem,
		pq.Array(&r.Permissions), &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create role: %w", err)
	}
	return r, nil
}

func (s *Storage) UpdateRole(tenantID, roleID uuid.UUID, description string, permissions []string) (*Role, error) {
	r := &Role{}
	err := s.db.QueryRow(`
		UPDATE roles SET description = $3, permissions = $4, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2 AND is_system = false
		RETURNING id, tenant_id, name, description, is_system, permissions, created_at, updated_at
	`, tenantID, roleID, description, pq.Array(permissions)).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.Description, &r.IsSystem,
		pq.Array(&r.Permissions), &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to update role (may be a system role): %w", err)
	}
	return r, nil
}

func (s *Storage) DeleteRole(tenantID, roleID uuid.UUID) error {
	result, err := s.db.Exec(`
		DELETE FROM roles WHERE tenant_id = $1 AND id = $2 AND is_system = false
	`, tenantID, roleID)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("role not found or is a system role")
	}
	return nil
}

func (s *Storage) GetRolePermissions(tenantID uuid.UUID, roleName string) ([]string, error) {
	var permissions []string
	err := s.db.QueryRow(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`, tenantID, roleName).Scan(pq.Array(&permissions))
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions for role '%s': %w", roleName, err)
	}
	return permissions, nil
}

// EnsureTenantRoles ensures system roles exist for a given tenant.
// Called on tenant creation and on application startup.
func (s *Storage) EnsureTenantRoles(tenantID uuid.UUID) error {
	return s.CreateSystemRoles(tenantID)
}

// EnsureAllTenantRoles ensures system roles exist for all tenants.
// Called on application startup.
func (s *Storage) EnsureAllTenantRoles() error {
	rows, err := s.db.Query(`SELECT id FROM tenants`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tenantID uuid.UUID
		if err := rows.Scan(&tenantID); err != nil {
			return err
		}
		if err := s.CreateSystemRoles(tenantID); err != nil {
			return fmt.Errorf("failed to ensure roles for tenant %s: %w", tenantID, err)
		}
	}
	return rows.Err()
}
