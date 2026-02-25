package v1

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"
)

type TenantAPI struct {
	store *controllerstorage.Storage
}

func NewTenantAPI(store *controllerstorage.Storage) *TenantAPI {
	return &TenantAPI{
		store: store,
	}
}

type TenantResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Code  string `json:"code"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
	I18n  struct {
		Name string `json:"name,omitempty"`
	} `json:"i18n,omitempty"`
}

type TenantCreateRequest struct {
	Name  string `json:"name"`
	Code  string `json:"code"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

func (t *TenantAPI) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req TenantCreateRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	tenantID := uuid.New()

	query := `INSERT INTO tenants (id, name, code, created_at, updated_at) VALUES ($1, $2, $3, NOW(), NOW())`
	_, err := t.store.DB().Exec(query, tenantID, req.Name, req.Code)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreateTenantFailed, "Failed to create tenant: "+err.Error(), nil)
		return
	}

	resp := TenantResponse{
		ID:    tenantID.String(),
		Name:  req.Name,
		Code:  req.Code,
		Email: req.Email,
		Phone: req.Phone,
	}

	WriteSuccess(w, resp, "Tenant created successfully")
}

func (t *TenantAPI) GetTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		WriteError(w, http.StatusBadRequest, CodeTenantIDRequired, "Tenant ID is required", nil)
		return
	}

	tenantIDStr := pathParts[3]
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidTenantID, "Invalid tenant ID", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	if userTenantID != tenantID {
		WriteError(w, http.StatusForbidden, CodeAccessDenied, "Access denied", nil)
		return
	}

	var tenant controllerstorage.TenantInfo
	query := `SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`
	err = t.store.DB().QueryRow(query, tenantID).Scan(
		&tenant.ID, &tenant.Name, &tenant.Code, &tenant.Status,
		&tenant.ResourceQuota, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if err != nil {
		WriteError(w, http.StatusNotFound, CodeTenantNotFound, "Tenant not found", nil)
		return
	}

	resp := TenantResponse{
		ID:   tenant.ID.String(),
		Name: tenant.Name,
		Code: tenant.Code,
	}

	WriteSuccess(w, resp, "Tenant retrieved successfully")
}

func (t *TenantAPI) ListTenants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	role, exists := middleware.GetUserRole(r.Context())
	if !exists || role != "admin" {
		WriteError(w, http.StatusForbidden, CodeAccessDenied, "Access denied: super admin only", nil)
		return
	}

	rows, err := t.store.DB().Query(`SELECT id, name, code FROM tenants`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeListTenantsFailed, "Failed to list tenants", nil)
		return
	}
	defer rows.Close()

	var tenants []TenantResponse
	for rows.Next() {
		var id uuid.UUID
		var name, code string
		if err := rows.Scan(&id, &name, &code); err != nil {
			WriteError(w, http.StatusInternalServerError, CodeScanTenantFailed, "Failed to scan tenant", nil)
			return
		}

		tenants = append(tenants, TenantResponse{
			ID:   id.String(),
			Name: name,
			Code: code,
		})
	}

	WriteSuccess(w, tenants, fmt.Sprintf("%d tenants retrieved", len(tenants)))
}

// ✅ 核心修复：干掉不存在的 GetAllNodes 调用，直接下推 SQL 进行租户级高性能查询
func (t *TenantAPI) GetTenantNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	query := `
		SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role,
		       COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0),
		       advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at
		FROM nodes 
		WHERE tenant_id = $1 
		ORDER BY last_seen DESC`

	rows, err := t.store.DB().Query(query, userTenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeGetNodesFailed, "Failed to get nodes", nil)
		return
	}
	defer rows.Close()

	var tenantNodes []map[string]interface{}
	for rows.Next() {
		var node controllerstorage.Node
		var advertisedRoutes interface{}
		var enrolledWithToken interface{}

		err := rows.Scan(
			&node.ID, &node.PublicKey, &node.MachineID, &node.TenantID,
			&node.Endpoint, &node.PrivateIP, &node.PublicIP, &node.Region,
			&node.VPCID, &node.Hostname, &node.AssignedIP, &node.IPOffset,
			&node.LastSeen, &node.RegisteredAt, &node.Role,
			&node.RuntimeMode, &node.KernelVersion, &node.HasAESNI,
			&node.Status, &node.OfflineSince,
			&advertisedRoutes, &enrolledWithToken,
			&node.CreatedAt, &node.UpdatedAt,
		)
		if err != nil {
			continue // 生产环境建议加上 log.Printf
		}

		tenantNodes = append(tenantNodes, map[string]interface{}{
			"id":          node.ID.String(),
			"public_key":  node.PublicKey,
			"hostname":    node.Hostname,
			"assigned_ip": node.AssignedIP,
			"status":      node.Status,
			"last_seen":   node.LastSeen,
		})
	}

	WriteSuccess(w, tenantNodes, fmt.Sprintf("%d nodes retrieved", len(tenantNodes)))
}

func (t *TenantAPI) HandleTenants(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) >= 4 {
		switch pathParts[3] {
		case "list":
			if r.Method == http.MethodGet {
				t.ListTenants(w, r)
				return
			}
		default:
			switch r.Method {
			case http.MethodGet:
				t.GetTenant(w, r)
				return
			}
		}
	}

	switch r.Method {
	case http.MethodPost:
		t.CreateTenant(w, r)
	case http.MethodGet:
		t.ListTenants(w, r)
	default:
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func SetupTenantAPIRoutes(mux *http.ServeMux, store *controllerstorage.Storage) {
	api := NewTenantAPI(store)
	withJWT := middleware.JWTAuthMiddleware

	mux.HandleFunc("/api/v1/tenants", withJWT(api.HandleTenants))
	mux.HandleFunc("/api/v1/tenants/", withJWT(api.HandleTenants))
	mux.HandleFunc("/api/v1/nodes", withJWT(api.GetTenantNodes))
}