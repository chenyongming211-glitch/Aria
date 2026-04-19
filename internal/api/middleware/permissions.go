package middleware

import (
	"context"
	"net/http"

	"aria/internal/api/apibase"
	controllerstorage "aria/pkg/controllerstorage"
)

const RoleSuperAdmin = "super_admin"

const (
	PermNodesRead      = "nodes:read"
	PermNodesWrite     = "nodes:write"
	PermRoutesRead     = "routes:read"
	PermRoutesWrite    = "routes:write"
	PermAclsRead       = "acls:read"
	PermAclsWrite      = "acls:write"
	PermQosRead        = "qos:read"
	PermQosWrite       = "qos:write"
	PermBlacklistRead  = "blacklist:read"
	PermBlacklistWrite = "blacklist:write"
	PermMonitoringRead = "monitoring:read"
	PermCommandsWrite  = "commands:write"
	PermTokensRead     = "tokens:read"
	PermTokensWrite    = "tokens:write"
	PermUsersRead      = "users:read"
	PermUsersWrite     = "users:write"
	PermRolesRead      = "roles:read"
	PermRolesWrite     = "roles:write"
	PermAiUse          = "ai:use"
	PermPoliciesRead   = "policies:read"
	PermSettingsRead   = "settings:read"
	PermSettingsWrite  = "settings:write"
)

const permKey contextKey = "user_permissions"

func GetPermissions(ctx context.Context) ([]string, bool) {
	val := ctx.Value(permKey)
	if val == nil {
		return nil, false
	}
	perms, ok := val.([]string)
	return perms, ok
}

// RequirePermission creates middleware that checks if the authenticated user has a specific permission.
// Must be used after JWTAuthMiddleware.
func RequirePermission(store *controllerstorage.Storage, permission string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			role, _ := GetUserRole(r.Context())
			tenantID, hasTenant := GetTenantID(r.Context())

			if role == RoleSuperAdmin {
				next(w, r)
				return
			}

			if !hasTenant {
				apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Tenant context required", nil)
				return
			}

			roleName := role
			if roleName == "member" || roleName == "owner" {
				roleName = controllerstorage.SystemRoleOperator
			}

			permissions, err := store.GetRolePermissions(tenantID, roleName)
			if err != nil {
				apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Role not found", nil)
				return
			}

			if !containsPermission(permissions, permission) {
				apibase.WriteError(w, http.StatusForbidden, apibase.CodeAccessDenied, "Insufficient permissions", nil)
				return
			}

			ctx := context.WithValue(r.Context(), permKey, permissions)
			next(w, r.WithContext(ctx))
		}
	}
}

func containsPermission(permissions []string, target string) bool {
	for _, p := range permissions {
		if p == target {
			return true
		}
	}
	return false
}
