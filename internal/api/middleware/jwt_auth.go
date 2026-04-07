package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aria/internal/auth"

	"github.com/google/uuid"
)

// contextKey 用于上下文键的类型安全
type contextKey string

const (
	// ✅ 修复：将所有鉴权相关的上下文键统一在此处定义，避免编译冲突
	UserIDKey   contextKey = "user_id"
	UsernameKey contextKey = "username"
	UserRoleKey contextKey = "user_role"
	TenantIDKey contextKey = "tenant_id"
)

var mcpWhitelist = []string{"/v1/auth/force-change-password", "/v1/auth/login", "/v1/auth/refresh", "/v1/auth/logout"}

func containsPath(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

// JWTAuthMiddleware 创建JWT认证中间件
func JWTAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			WriteUnauthorizedError(w, "Authorization header required")
			return
		}

		var tokenString string
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			tokenString = authHeader
		}

		if tokenString == "" {
			WriteUnauthorizedError(w, "Invalid token format")
			return
		}

		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			if err == auth.ErrExpiredToken {
				WriteUnauthorizedError(w, "Token has expired")
			} else {
				WriteUnauthorizedError(w, "Invalid token")
			}
			return
		}

		if claims.MustChangePassword && !containsPath(mcpWhitelist, r.URL.Path) {
			WriteForbiddenError(w, "You must change your password before proceeding")
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UsernameKey, claims.Username)
		ctx = context.WithValue(ctx, UserRoleKey, claims.Role)

		// 租户上下文设置：
		// - super_admin：允许通过 X-Tenant-ID 头切换租户（管理多租户需要）
		// - 其他角色：强制使用 JWT claims 中的 TenantID，防止越权
		if claims.Role == "super_admin" {
			if tenantIDHeader := r.Header.Get("X-Tenant-ID"); tenantIDHeader != "" {
				if _, err := uuid.Parse(tenantIDHeader); err == nil {
					ctx = context.WithValue(ctx, TenantIDKey, tenantIDHeader)
				} else if claims.TenantID != "" {
					ctx = context.WithValue(ctx, TenantIDKey, claims.TenantID)
				}
			} else if claims.TenantID != "" {
				ctx = context.WithValue(ctx, TenantIDKey, claims.TenantID)
			}
		} else if claims.TenantID != "" {
			ctx = context.WithValue(ctx, TenantIDKey, claims.TenantID)
		}

		next(w, r.WithContext(ctx))
	}
}

func WriteUnauthorizedError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	response := map[string]interface{}{
		"success": false,
		"message": message,
		"code":    "UNAUTHORIZED",
		"error": map[string]interface{}{
			"code":    "UNAUTHORIZED",
			"message": message,
		},
	}
	json.NewEncoder(w).Encode(response)
}

func GetUserID(ctx context.Context) (string, bool) {
	value := ctx.Value(UserIDKey)
	if value == nil {
		return "", false
	}
	return value.(string), true
}

func GetUsername(ctx context.Context) (string, bool) {
	value := ctx.Value(UsernameKey)
	if value == nil {
		return "", false
	}
	return value.(string), true
}

func GetUserRole(ctx context.Context) (string, bool) {
	value := ctx.Value(UserRoleKey)
	if value == nil {
		return "", false
	}
	return value.(string), true
}

func RequireRole(requiredRole string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			role, exists := GetUserRole(ctx)
			if !exists {
				WriteUnauthorizedError(w, "User not authenticated")
				return
			}

			if role != "admin" && role != requiredRole {
				WriteForbiddenError(w, fmt.Sprintf("Insufficient permissions. Required role: %s", requiredRole))
				return
			}

			next(w, r)
		}
	}
}

func WriteForbiddenError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	response := map[string]interface{}{
		"success": false,
		"message": message,
		"code":    "FORBIDDEN",
		"error": map[string]interface{}{
			"code":    "FORBIDDEN",
			"message": message,
		},
	}
	json.NewEncoder(w).Encode(response)
}

func RequireTenantAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		role, exists := GetUserRole(ctx)
		if !exists {
			WriteUnauthorizedError(w, "User not authenticated")
			return
		}

		tenantIDVal := ctx.Value(TenantIDKey)
		var tenantID string
		if tenantIDVal != nil {
			switch v := tenantIDVal.(type) {
			case string:
				tenantID = v
			case uuid.UUID:
				tenantID = v.String()
			}
		}

		targetTenantID := r.PathValue("tenant_id")

		if role == "super_admin" {
			next(w, r)
			return
		}

		if role == "member" || role == "viewer" {
			WriteForbiddenError(w, "user management requires admin role")
			return
		}

		if targetTenantID != "" && tenantID != "" && tenantID != targetTenantID {
			WriteForbiddenError(w, "cross-tenant access prohibited")
			return
		}

		next(w, r)
	}
}
