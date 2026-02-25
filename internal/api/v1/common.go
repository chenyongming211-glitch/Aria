package v1

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"time"
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

// 统一响应辅助函数

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

// WriteValidationError 写入参数验证错误响应
func WriteValidationError(w http.ResponseWriter, fieldErrors map[string]string) {
	WriteError(w, http.StatusBadRequest, "VALIDATION_FAILED", "请求参数验证失败", fieldErrors)
}

// ParseRequestJSON 解析请求JSON体
func ParseRequestJSON(r *http.Request, dest interface{}) error {
	return json.NewDecoder(r.Body).Decode(dest)
}

// 统一错误码常量
const (
	// 通用错误码
	CodeOK                   = "SUCCESS"
	CodeBadRequest           = "BAD_REQUEST"
	CodeUnauthorized         = "UNAUTHORIZED"
	CodeForbidden            = "FORBIDDEN"
	CodeNotFound             = "NOT_FOUND"
	CodeInternalServerError   = "INTERNAL_ERROR"
	CodeValidationFailed      = "VALIDATION_FAILED"
	CodeRateLimitExceeded    = "RATE_LIMIT_EXCEEDED"
	CodeMethodNotAllowed      = "METHOD_NOT_ALLOWED"
	CodeNotImplemented      = "NOT_IMPLEMENTED"
	CodeEndpointNotFound     = "ENDPOINT_NOT_FOUND"
	CodeInvalidPath         = "INVALID_PATH"

	// 认证相关错误码
	CodeAuthHeaderRequired   = "AUTH_HEADER_REQUIRED"
	CodeInvalidToken         = "INVALID_TOKEN"
	CodeInvalidCredentials  = "INVALID_CREDENTIALS"
	CodeCreateTokenFailed   = "CREATE_TOKEN_FAILED"

	// 租户相关错误码
	CodeTenantIDRequired    = "TENANT_ID_REQUIRED"
	CodeInvalidTenantID     = "INVALID_TENANT_ID"
	CodeTenantNotFound       = "TENANT_NOT_FOUND"
	CodeTenantContextNotFound = "TENANT_CONTEXT_NOT_FOUND"
	CodeAccessDenied         = "ACCESS_DENIED"

	// 请求相关错误码
	CodeInvalidRequest       = "INVALID_REQUEST"

	// 创建/操作相关错误码
	CodeCreateTenantFailed   = "CREATE_TENANT_FAILED"
	CodeListTenantsFailed   = "LIST_TENANTS_FAILED"
	CodeScanTenantFailed    = "SCAN_TENANT_FAILED"
	CodeGetNodesFailed      = "GET_NODES_FAILED"

	// 带宽相关错误码
	CodeInvalidBandwidth    = "INVALID_BANDWIDTH"
	CodeLimitApplyFailed    = "LIMIT_APPLY_FAILED"
	CodeLimitIDRequired    = "LIMIT_ID_REQUIRED"
	CodeGetLimitsFailed    = "GET_LIMITS_FAILED"

	// 策略相关错误码
	CodePolicyNameRequired     = "POLICY_NAME_REQUIRED"
	CodeInvalidAction         = "INVALID_ACTION"
	CodeLimitBandwidthRequired = "LIMIT_BANDWIDTH_REQUIRED"
	CodePolicyIDRequired      = "POLICY_ID_REQUIRED"
	CodePolicyNotFound        = "POLICY_NOT_FOUND"
	CodeCreatePolicyFailed    = "CREATE_POLICY_FAILED"
	CodeUpdatePolicyFailed    = "UPDATE_POLICY_FAILED"
	CodeDeletePolicyFailed    = "DELETE_POLICY_FAILED"
	CodeGetPolicyFailed       = "GET_POLICY_FAILED"
	CodeGetPoliciesFailed    = "GET_POLICIES_FAILED"
	CodeQoSRuleApplyFailed   = "QOS_RULE_APPLY_FAILED"
	CodeQoSRuleUpdateFailed  = "QOS_RULE_UPDATE_FAILED"

	// Token 相关错误码
	CodeInvalidResourceQuota = "INVALID_RESOURCE_QUOTA"
	CodeInvalidTTLFormat    = "INVALID_TTL_FORMAT"
	CodeListTokensFailed     = "LIST_TOKENS_FAILED"
	CodeScanTokenFailed      = "SCAN_TOKEN_FAILED"
	CodeTokenIDRequired     = "TOKEN_ID_REQUIRED"
	CodeInvalidTokenID       = "INVALID_TOKEN_ID"
	CodeDeleteTokenFailed    = "DELETE_TOKEN_FAILED"
	CodeTokenNotFound        = "TOKEN_NOT_FOUND"
	CodeTokenParamRequired   = "TOKEN_PARAM_REQUIRED"
	CodeGetTokenNodesFailed  = "GET_TOKEN_NODES_FAILED"

	// 节点相关错误码
	CodeScanNodeFailed = "SCAN_NODE_FAILED"

	// ACL 相关错误码
	CodeGetACLRulesFailed = "GET_ACL_RULES_FAILED"
	CodeScanACLRuleFailed = "SCAN_ACL_RULE_FAILED"

	// 其他错误码
	CodeInvalidProtocol = "INVALID_PROTOCOL"

	// AI 相关错误码
	CodeMessageEmpty   = "MESSAGE_EMPTY"
	CodeAIServiceError = "AI_SERVICE_ERROR"
)

// randomString 生成随机字符串
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}