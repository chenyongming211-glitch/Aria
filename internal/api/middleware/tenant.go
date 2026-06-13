package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"aria/pkg/controllerstorage"
)

// ✅ 修复：从 jwt_auth.go 导入共享的常量定义
// 不在此处重复声明 TenantIDKey 等常量，直接使用 middleware 包中的定义

// TenantMiddleware 验证租户上下文并将其存储在请求上下文中
type TenantMiddleware struct {
	store *controllerstorage.Storage
}

// NewTenantMiddleware 创建新的租户中间件
func NewTenantMiddleware(store *controllerstorage.Storage) *TenantMiddleware {
	return &TenantMiddleware{
		store: store,
	}
}

// FromToken 从边缘设备的注册令牌(tk_xxx)获取租户ID并注入上下文
func (tm *TenantMiddleware) FromToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		tokenStr := extractHTTPAuthorizationToken(authHeader)

		// ✅ 修复：直接使用原生 SQL 查询，解除与其他隔离模块的循环依赖
		var tenantID uuid.UUID
		err := tm.store.DB().QueryRow(
			`SELECT tenant_id FROM tokens WHERE token = $1 AND status = 'active'`,
			tokenStr,
		).Scan(&tenantID)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), TenantIDKey, tenantID)
		next(w, r.WithContext(ctx))
	}
}

// GetTenantID 从请求上下文获取租户ID
func GetTenantID(ctx context.Context) (uuid.UUID, bool) {
	if ctx == nil {
		return uuid.Nil, false
	}

	val := ctx.Value(TenantIDKey)
	if val == nil {
		return uuid.Nil, false
	}

	// 尝试作为 uuid.UUID 类型断言
	if tenantID, ok := val.(uuid.UUID); ok {
		return tenantID, true
	}

	// 尝试作为 string 类型断言并解析
	if tenantIDStr, ok := val.(string); ok && tenantIDStr != "" {
		tenantID, err := uuid.Parse(tenantIDStr)
		if err == nil {
			return tenantID, true
		}
	}

	return uuid.Nil, false
}
