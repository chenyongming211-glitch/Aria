package apibase

import (
	"encoding/json"
	"net/http"
)

// APIResponse 标准响应格式
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *APIMeta    `json:"meta,omitempty"`
	Code    string      `json:"code,omitempty"` // 业务状态码
}

// APIError 错误信息
type APIError struct {
	Code    string            `json:"code"`              // 错误代码
	Message string            `json:"message"`           // 错误消息
	Details map[string]string `json:"details,omitempty"` // 详细错误信息
}

// APIMeta 分页和元数据信息
type APIMeta struct {
	Total    int    `json:"total,omitempty"`     // 总数
	Page     int    `json:"page,omitempty"`      // 页码
	PageSize int    `json:"page_size,omitempty"` // 页大小
	Next     string `json:"next,omitempty"`      // 下一页链接
	Prev     string `json:"prev,omitempty"`      // 上一页链接
}

// WriteSuccess 写入成功响应
func WriteSuccess(w http.ResponseWriter, data interface{}, message string) {
	resp := APIResponse{
		Success: true,
		Data:    data,
		Message: message,
		Code:    "SUCCESS",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// WriteError 写入错误响应
func WriteError(w http.ResponseWriter, statusCode int, errorCode, message string, details map[string]string) {
	resp := APIResponse{
		Success: false,
		Message: message,
		Error: &APIError{
			Code:    errorCode,
			Message: message,
			Details: details,
		},
		Code: errorCode,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// ParseRequestJSON 解析请求JSON体
func ParseRequestJSON(r *http.Request, dest interface{}) error {
	return json.NewDecoder(r.Body).Decode(dest)
}

// 统一错误码常量
const (
	// 通用错误码
	CodeOK                  = "SUCCESS"
	CodeBadRequest          = "BAD_REQUEST"
	CodeUnauthorized        = "UNAUTHORIZED"
	CodeForbidden           = "FORBIDDEN"
	CodeNotFound            = "NOT_FOUND"
	CodeInternalServerError = "INTERNAL_ERROR"
	CodeValidationFailed    = "VALIDATION_FAILED"
	CodeRateLimitExceeded   = "RATE_LIMIT_EXCEEDED"
	CodeMethodNotAllowed    = "METHOD_NOT_ALLOWED"
	CodeNotImplemented      = "NOT_IMPLEMENTED"
	CodeServiceUnavailable  = "SERVICE_UNAVAILABLE"
	CodeEndpointNotFound    = "ENDPOINT_NOT_FOUND"
	CodeInvalidPath         = "INVALID_PATH"

	// 认证相关错误码
	CodeAuthHeaderRequired = "AUTH_HEADER_REQUIRED"
	CodeInvalidToken       = "INVALID_TOKEN"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeCreateTokenFailed  = "CREATE_TOKEN_FAILED"

	// 租户相关错误码
	CodeTenantIDRequired      = "TENANT_ID_REQUIRED"
	CodeInvalidTenantID       = "INVALID_TENANT_ID"
	CodeTenantNotFound        = "TENANT_NOT_FOUND"
	CodeTenantContextNotFound = "TENANT_CONTEXT_NOT_FOUND"
	CodeAccessDenied          = "ACCESS_DENIED"

	// 请求相关错误码
	CodeInvalidRequest = "INVALID_REQUEST"

	// 创建/操作相关错误码
	CodeCreateTenantFailed = "CREATE_TENANT_FAILED"
	CodeListTenantsFailed  = "LIST_TENANTS_FAILED"
	CodeScanTenantFailed   = "SCAN_TENANT_FAILED"
	CodeGetNodesFailed     = "GET_NODES_FAILED"
	CodeNodeNotFound       = "NODE_NOT_FOUND"
	CodeUpdateNodeFailed   = "UPDATE_NODE_FAILED"
	CodeScanNodeFailed     = "SCAN_NODE_FAILED"
	CodeGetACLRulesFailed  = "GET_ACL_RULES_FAILED"
	CodeScanACLRuleFailed  = "SCAN_ACL_RULE_FAILED"
	CodeGetLimitsFailed    = "GET_LIMITS_FAILED"
	CodeLimitApplyFailed   = "LIMIT_APPLY_FAILED"

	// 用户相关错误码
	CodeListUsersFailed  = "LIST_USERS_FAILED"
	CodeCreateUserFailed = "CREATE_USER_FAILED"
	CodeUpdateUserFailed = "UPDATE_USER_FAILED"
	CodeDeleteUserFailed = "DELETE_USER_FAILED"
	CodeInvalidUserID    = "INVALID_USER_ID"
	CodeUserNotFound     = "USER_NOT_FOUND"

	// 令牌相关错误码
	CodeInvalidTokenID    = "INVALID_TOKEN_ID"
	CodeTokenNotFound     = "TOKEN_NOT_FOUND"
	CodeDeleteTokenFailed = "DELETE_TOKEN_FAILED"
)
