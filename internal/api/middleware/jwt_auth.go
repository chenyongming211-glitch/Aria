package middleware

import (
	"context"
	"net/http"
	"strings"

	"aria/internal/api/apibase"
	"aria/internal/auth"

	"github.com/google/uuid"
)

// contextKey 用于上下文键的类型安全
type contextKey string

const (
	UserIDKey   contextKey = "user_id"
	UsernameKey contextKey = "username"
	UserRoleKey contextKey = "user_role"
	TenantIDKey contextKey = "tenant_id"
)

var mcpWhitelist = []string{"/api/v2/auth/force-change-password", "/api/v2/auth/login", "/api/v2/auth/refresh", "/api/v2/auth/logout"}

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
			apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeAuthHeaderRequired, "Authorization header required", nil)
			return
		}

		var tokenString string
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			tokenString = authHeader
		}

		if tokenString == "" {
			apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidToken, "Invalid token format", nil)
			return
		}

		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			if err == auth.ErrExpiredToken {
				apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidToken, "Token has expired", nil)
			} else {
				apibase.WriteError(w, http.StatusUnauthorized, apibase.CodeInvalidToken, "Invalid token", nil)
			}
			return
		}

		if claims.MustChangePassword && !containsPath(mcpWhitelist, r.URL.Path) {
			apibase.WriteError(w, http.StatusForbidden, apibase.CodeForbidden, "You must change your password before proceeding", nil)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UsernameKey, claims.Username)
		ctx = context.WithValue(ctx, UserRoleKey, claims.Role)

		if claims.Role == "super_admin" {
			if tenantIDHeader := r.Header.Get("X-Tenant-ID"); tenantIDHeader != "" {
				if tid, err := uuid.Parse(tenantIDHeader); err == nil {
					ctx = context.WithValue(ctx, TenantIDKey, tid)
				} else if claims.TenantID != "" {
					if tid, err := uuid.Parse(claims.TenantID); err == nil {
						ctx = context.WithValue(ctx, TenantIDKey, tid)
					}
				}
			} else if claims.TenantID != "" {
				if tid, err := uuid.Parse(claims.TenantID); err == nil {
					ctx = context.WithValue(ctx, TenantIDKey, tid)
				}
			}
		} else if claims.TenantID != "" {
			if tid, err := uuid.Parse(claims.TenantID); err == nil {
				ctx = context.WithValue(ctx, TenantIDKey, tid)
			}
		}

		next(w, r.WithContext(ctx))
	}
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
