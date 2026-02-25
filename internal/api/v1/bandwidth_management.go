package v1

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"
)

type BandwidthLimitRequest struct {
	SrcIP     string `json:"src_ip,omitempty"`
	DstIP     string `json:"dst_ip,omitempty"`
	SrcPort   int    `json:"src_port,omitempty"`
	DstPort   int    `json:"dst_port,omitempty"`
	Protocol  int    `json:"protocol,omitempty"` 
	Bandwidth int    `json:"bandwidth"`          
	Direction string `json:"direction,omitempty"` 
}

type PolicyRule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	Priority    int    `json:"priority"`
	Action      string `json:"action"` 

	SrcIP     string `json:"src_ip,omitempty"`
	SrcPort   int    `json:"src_port,omitempty"`
	SrcRegion string `json:"src_region,omitempty"`

	DstIP     string `json:"dst_ip,omitempty"`
	DstPort   int    `json:"dst_port,omitempty"`
	DstRegion string `json:"dst_region,omitempty"`

	Protocol  int    `json:"protocol,omitempty"`
	ProtocolName string `json:"protocol_name,omitempty"`

	LimitBandwidth int `json:"limit_bandwidth,omitempty"` 
	LimitType      string `json:"limit_type,omitempty"`   

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type BandwidthManagementAPI struct {
	store *controllerstorage.Storage
}

func NewBandwidthManagementAPI(store *controllerstorage.Storage) (*BandwidthManagementAPI, error) {
	return &BandwidthManagementAPI{
		store: store,
	}, nil
}

func (b *BandwidthManagementAPI) Close() error {
	return nil
}

func (b *BandwidthManagementAPI) LimitBandwidth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	var req BandwidthLimitRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if req.Bandwidth <= 0 {
		WriteError(w, http.StatusBadRequest, CodeInvalidBandwidth, "Bandwidth must be greater than 0", nil)
		return
	}

	if req.Protocol == 0 {
		req.Protocol = 6
	}

	var columns []string
	var placeholders []string
	var values []interface{}
	var paramIndex = 1

	columns = append(columns, "tenant_id", "bandwidth_mbps")
	placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex), fmt.Sprintf("$%d", paramIndex+1))
	values = append(values, userTenantID.String(), req.Bandwidth)
	paramIndex += 2

	if req.SrcIP != "" {
		columns = append(columns, "src_ip", "src_port", "protocol")
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex), fmt.Sprintf("$%d", paramIndex+1), fmt.Sprintf("$%d", paramIndex+2))
		values = append(values, req.SrcIP, req.SrcPort, req.Protocol)
		paramIndex += 3
	}

	if req.DstIP != "" {
		columns = append(columns, "dst_ip", "dst_port")
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex), fmt.Sprintf("$%d", paramIndex+1))
		values = append(values, req.DstIP, req.DstPort)
		paramIndex += 2
	}

	query := fmt.Sprintf("INSERT INTO bandwidth_limits (%s) VALUES (%s)", strings.Join(columns, ", "), strings.Join(placeholders, ", "))

	_, err := b.store.DB().Exec(query, values...)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeLimitApplyFailed, "Failed to apply bandwidth limit: "+err.Error(), nil)
		return
	}

	WriteSuccess(w, req, "Bandwidth limit applied successfully")
}

func (b *BandwidthManagementAPI) DeleteBandwidthLimit(w http.ResponseWriter, r *http.Request) {
	WriteSuccess(w, map[string]string{"message": "Delete functionality not yet implemented"}, "Bandwidth limit delete not available in this version")
}

func (b *BandwidthManagementAPI) ListBandwidthLimits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	// ✅ 修复：修正扫描错位问题，严格匹配 BandwidthLimitRequest 拥有的字段
	query := `
		SELECT src_ip, dst_ip, src_port, dst_port, protocol, bandwidth_mbps
		FROM bandwidth_limits
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`
	rows, err := b.store.DB().Query(query, userTenantID.String())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeGetLimitsFailed, "Failed to retrieve bandwidth limits: "+err.Error(), nil)
		return
	}
	defer rows.Close()

	var limits []BandwidthLimitRequest
	for rows.Next() {
		var limit BandwidthLimitRequest
		err := rows.Scan(
			&limit.SrcIP,
			&limit.DstIP,
			&limit.SrcPort,
			&limit.DstPort,
			&limit.Protocol,
			&limit.Bandwidth,
		)
		if err != nil {
			rows.Close()
			WriteError(w, http.StatusInternalServerError, CodeGetLimitsFailed, "Failed to scan bandwidth limit", nil)
			return
		}
		limits = append(limits, limit)
	}

	WriteSuccess(w, limits, fmt.Sprintf("%d bandwidth limits retrieved", len(limits)))
}

func (b *BandwidthManagementAPI) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	var req PolicyRule
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if req.Name == "" {
		WriteError(w, http.StatusBadRequest, CodePolicyNameRequired, "Policy name is required", nil)
		return
	}

	if req.Action == "" {
		req.Action = "allow" 
	}

	if req.Action != "allow" && req.Action != "deny" && req.Action != "limit" {
		WriteError(w, http.StatusBadRequest, CodeInvalidAction, "Action must be 'allow', 'deny', or 'limit'", nil)
		return
	}

	if req.Action == "limit" && req.LimitBandwidth <= 0 {
		WriteError(w, http.StatusBadRequest, CodeLimitBandwidthRequired, "Limit bandwidth is required when action is 'limit'", nil)
		return
	}

	var columns []string
	var placeholders []string
	var values []interface{}
	var paramIndex = 1

	columns = append(columns, "tenant_id", "name", "description", "enabled", "priority", "action", "created_at", "updated_at")
	placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex), fmt.Sprintf("$%d", paramIndex+1), fmt.Sprintf("$%d", paramIndex+2),
		fmt.Sprintf("$%d", paramIndex+3), fmt.Sprintf("$%d", paramIndex+4),
		fmt.Sprintf("$%d", paramIndex+5), fmt.Sprintf("$%d", paramIndex+6), fmt.Sprintf("$%d", paramIndex+7))
	
	now := time.Now()
	values = append(values, userTenantID.String(), req.Name, req.Description, req.Enabled, req.Priority, req.Action, now, now)
	paramIndex = 8

	if req.SrcIP != "" {
		columns = append(columns, "src_ip", "src_port", "src_region")
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex), fmt.Sprintf("$%d", paramIndex+1), fmt.Sprintf("$%d", paramIndex+2))
		values = append(values, req.SrcIP, req.SrcPort, req.SrcRegion)
		paramIndex += 3
	}

	if req.DstIP != "" {
		columns = append(columns, "dst_ip", "dst_port", "dst_region")
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex), fmt.Sprintf("$%d", paramIndex+1), fmt.Sprintf("$%d", paramIndex+2))
		values = append(values, req.DstIP, req.DstPort, req.DstRegion)
		paramIndex += 3
	}

	if req.Protocol != 0 {
		columns = append(columns, "protocol", "protocol_name")
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex), fmt.Sprintf("$%d", paramIndex+1))
		values = append(values, req.Protocol, getProtocolName(req.Protocol))
		paramIndex += 2
	}

	if req.Action == "limit" {
		columns = append(columns, "limit_bandwidth", "limit_type")
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex), fmt.Sprintf("$%d", paramIndex+1))
		values = append(values, req.LimitBandwidth, req.LimitType)
		paramIndex += 2
	}

	query := fmt.Sprintf("INSERT INTO policy_rules (%s) VALUES (%s) RETURNING id", strings.Join(columns, ", "), strings.Join(placeholders, ", "))

	var policyID string
	err := b.store.DB().QueryRow(query, values...).Scan(&policyID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeCreatePolicyFailed, "Failed to create policy: "+err.Error(), nil)
		return
	}

	response := req
	response.ID = policyID
	response.CreatedAt = now.Format("2006-01-02T15:04:05Z")
	response.UpdatedAt = now.Format("2006-01-02T15:04:05Z")

	WriteSuccess(w, response, "Policy created successfully")
}

func (b *BandwidthManagementAPI) GetPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		WriteError(w, http.StatusBadRequest, CodePolicyIDRequired, "Policy ID is required", nil)
		return
	}

	policyID := pathParts[4]

	query := `
		SELECT id, name, description, enabled, priority, action,
		       src_ip, src_port, src_region, dst_ip, dst_port, dst_region,
		       protocol, protocol_name, limit_bandwidth, limit_type, created_at, updated_at
		FROM policy_rules
		WHERE id = $1 AND tenant_id = $2
	`
	row := b.store.DB().QueryRow(query, policyID, userTenantID.String())

	var policy PolicyRule
	err := row.Scan(
		&policy.ID, &policy.Name, &policy.Description, &policy.Enabled, &policy.Priority, &policy.Action,
		&policy.SrcIP, &policy.SrcPort, &policy.SrcRegion,
		&policy.DstIP, &policy.DstPort, &policy.DstRegion,
		&policy.Protocol, &policy.ProtocolName, &policy.LimitBandwidth, &policy.LimitType,
		&policy.CreatedAt, &policy.UpdatedAt,
	)

	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeGetPolicyFailed, "Failed to scan policy", nil)
		return
	}

	WriteSuccess(w, policy, "Policy retrieved successfully")
}

func (b *BandwidthManagementAPI) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		WriteError(w, http.StatusBadRequest, CodePolicyIDRequired, "Policy ID is required", nil)
		return
	}

	policyID := pathParts[4]

	var req PolicyRule
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	var setClauses []string
	var setValues []interface{}
	var paramIndex = 1

	if req.Name != "" {
		setClauses = append(setClauses, "name = $"+strconv.Itoa(paramIndex))
		setValues = append(setValues, req.Name)
		paramIndex++
	}

	// ✅ 修复：移除了永远为假的对比逻辑，使用 PUT 语义直接覆盖更新
	setClauses = append(setClauses, "description = $"+strconv.Itoa(paramIndex))
	setValues = append(setValues, req.Description)
	paramIndex++

	setClauses = append(setClauses, "enabled = $"+strconv.Itoa(paramIndex))
	setValues = append(setValues, req.Enabled)
	paramIndex++

	setClauses = append(setClauses, "priority = $"+strconv.Itoa(paramIndex))
	setValues = append(setValues, req.Priority)
	paramIndex++

	if req.Action != "" {
		setClauses = append(setClauses, "action = $"+strconv.Itoa(paramIndex))
		setValues = append(setValues, req.Action)
		paramIndex++
	}

	setClauses = append(setClauses, "src_ip = $"+strconv.Itoa(paramIndex))
	setValues = append(setValues, req.SrcIP)
	paramIndex++

	setClauses = append(setClauses, "src_port = $"+strconv.Itoa(paramIndex))
	setValues = append(setValues, req.SrcPort)
	paramIndex++

	setClauses = append(setClauses, "dst_ip = $"+strconv.Itoa(paramIndex))
	setValues = append(setValues, req.DstIP)
	paramIndex++

	setClauses = append(setClauses, "dst_port = $"+strconv.Itoa(paramIndex))
	setValues = append(setValues, req.DstPort)
	paramIndex++

	if req.Protocol != 0 {
		setClauses = append(setClauses, "protocol = $"+strconv.Itoa(paramIndex))
		setValues = append(setValues, req.Protocol)
		paramIndex++
		
		// ✅ 修复：修正了 protocol_name 占位符错位的严重 Bug
		setClauses = append(setClauses, "protocol_name = $"+strconv.Itoa(paramIndex))
		setValues = append(setValues, getProtocolName(req.Protocol))
		paramIndex++
	}

	if req.Action == "limit" {
		setClauses = append(setClauses, "limit_bandwidth = $"+strconv.Itoa(paramIndex))
		setValues = append(setValues, req.LimitBandwidth)
		paramIndex++
		
		setClauses = append(setClauses, "limit_type = $"+strconv.Itoa(paramIndex))
		setValues = append(setValues, req.LimitType)
		paramIndex++
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	query := "UPDATE policy_rules SET " + strings.Join(setClauses, ", ") + 
		fmt.Sprintf(" WHERE id = $%d AND tenant_id = $%d", paramIndex, paramIndex+1)

	var values []interface{}
	values = append(values, setValues...)
	values = append(values, policyID, userTenantID.String())

	_, err := b.store.DB().Exec(query, values...)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeUpdatePolicyFailed, "Failed to update policy: "+err.Error(), nil)
		return
	}

	WriteSuccess(w, req, "Policy updated successfully")
}

func (b *BandwidthManagementAPI) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		WriteError(w, http.StatusBadRequest, CodePolicyIDRequired, "Policy ID is required", nil)
		return
	}

	policyID := pathParts[4]

	query := `DELETE FROM policy_rules WHERE id = $1 AND tenant_id = $2`
	_, err := b.store.DB().Exec(query, policyID, userTenantID.String())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeDeletePolicyFailed, "Failed to delete policy: "+err.Error(), nil)
		return
	}

	WriteSuccess(w, map[string]string{"id": policyID}, "Policy deleted successfully")
}

func (b *BandwidthManagementAPI) ListPolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	userTenantID, exists := middleware.GetTenantID(r.Context())
	if !exists {
		WriteError(w, http.StatusUnauthorized, CodeTenantContextNotFound, "请先选择租户：当前未设置租户上下文", nil)
		return
	}

	queryParams := r.URL.Query()
	var whereClauses []string
	
	// ✅ 修复：将提取到的参数准确追加到 args 列表中供 DB.Query 使用
	var args []interface{}
	args = append(args, userTenantID.String())

	if name := queryParams.Get("name"); name != "" {
		args = append(args, name)
		whereClauses = append(whereClauses, fmt.Sprintf("name = $%d", len(args)))
	}

	if action := queryParams.Get("action"); action != "" {
		args = append(args, action)
		whereClauses = append(whereClauses, fmt.Sprintf("action = $%d", len(args)))
	}

	if enabledStr := queryParams.Get("enabled"); enabledStr != "" {
		if enabled, err := strconv.ParseBool(enabledStr); err == nil {
			args = append(args, enabled)
			whereClauses = append(whereClauses, fmt.Sprintf("enabled = $%d", len(args)))
		}
	}

	baseQuery := `
		SELECT id, name, description, enabled, priority, action,
		       src_ip, src_port, src_region, dst_ip, dst_port, dst_region,
		       protocol, protocol_name, limit_bandwidth, limit_type, created_at, updated_at
		FROM policy_rules
		WHERE tenant_id = $1
	`

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = " AND " + strings.Join(whereClauses, " AND ")
	}

	sqlQuery := baseQuery + whereClause + " ORDER BY priority DESC, created_at DESC"

	// ✅ 正确传入过滤参数 args
	rows, err := b.store.DB().Query(sqlQuery, args...)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeGetPoliciesFailed, "Failed to retrieve policies: "+err.Error(), nil)
		return
	}
	defer rows.Close()

	var policies []PolicyRule
	for rows.Next() {
		var policy PolicyRule
		err := rows.Scan(
			&policy.ID, &policy.Name, &policy.Description, &policy.Enabled, &policy.Priority, &policy.Action,
			&policy.SrcIP, &policy.SrcPort, &policy.SrcRegion,
			&policy.DstIP, &policy.DstPort, &policy.DstRegion,
			&policy.Protocol, &policy.ProtocolName, &policy.LimitBandwidth, &policy.LimitType,
			&policy.CreatedAt, &policy.UpdatedAt,
		)
		if err != nil {
			rows.Close()
			WriteError(w, http.StatusInternalServerError, CodeGetPoliciesFailed, "Failed to scan policy", nil)
			return
		}
		policies = append(policies, policy)
	}

	WriteSuccess(w, policies, fmt.Sprintf("%d policies retrieved", len(policies)))
}

func (b *BandwidthManagementAPI) HandleBandwidthManagement(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		WriteError(w, http.StatusBadRequest, CodeInvalidPath, "Invalid path", nil)
		return
	}

	subResource := pathParts[3]

	switch subResource {
	case "limits":
		b.handleLimits(w, r, pathParts)
	case "policies":
		b.handlePolicies(w, r, pathParts)
	default:
		WriteError(w, http.StatusNotFound, CodeEndpointNotFound, "Unknown endpoint", nil)
	}
}

func (b *BandwidthManagementAPI) handleLimits(w http.ResponseWriter, r *http.Request, pathParts []string) {
	if len(pathParts) >= 5 {
		switch r.Method {
		case http.MethodGet:
			b.ListBandwidthLimits(w, r)
		case http.MethodDelete:
			b.DeleteBandwidthLimit(w, r)
		default:
			WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		b.ListBandwidthLimits(w, r)
	case http.MethodPost:
		b.LimitBandwidth(w, r)
	default:
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func (b *BandwidthManagementAPI) handlePolicies(w http.ResponseWriter, r *http.Request, pathParts []string) {
	if len(pathParts) >= 5 {
		switch r.Method {
		case http.MethodGet:
			b.GetPolicy(w, r)
		case http.MethodPut:
			b.UpdatePolicy(w, r)
		case http.MethodDelete:
			b.DeletePolicy(w, r)
		default:
			WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		b.ListPolicies(w, r)
	case http.MethodPost:
		b.CreatePolicy(w, r)
	default:
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
	}
}

func SetupBandwidthManagementRoutes(mux *http.ServeMux, store *controllerstorage.Storage) error {
	api, err := NewBandwidthManagementAPI(store)
	if err != nil {
		return fmt.Errorf("creating bandwidth management API: %w", err)
	}

	withJWT := middleware.JWTAuthMiddleware

	mux.HandleFunc("/api/v1/bandwidth/limits", withJWT(func(w http.ResponseWriter, r *http.Request) {
		api.HandleBandwidthManagement(w, r)
	}))

	mux.HandleFunc("/api/v1/bandwidth/limits/", withJWT(func(w http.ResponseWriter, r *http.Request) {
		api.HandleBandwidthManagement(w, r)
	}))

	mux.HandleFunc("/api/v1/bandwidth/policies", withJWT(func(w http.ResponseWriter, r *http.Request) {
		api.HandleBandwidthManagement(w, r)
	}))

	mux.HandleFunc("/api/v1/bandwidth/policies/", withJWT(func(w http.ResponseWriter, r *http.Request) {
		api.HandleBandwidthManagement(w, r)
	}))

	mux.HandleFunc("/api/v1/bandwidth/", withJWT(func(w http.ResponseWriter, r *http.Request) {
		api.HandleBandwidthManagement(w, r)
	}))

	return nil
}

func getProtocolName(protocol int) string {
	switch protocol {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 1:
		return "ICMP"
	case 58:
		return "ICMPv6"
	default:
		return "Unknown"
	}
}