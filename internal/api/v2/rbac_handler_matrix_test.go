package v2

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/api/handlers"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

type handlerMatrixCase struct {
	name         string
	mode         string
	role         string
	permissions  []string
	expectStatus int
	expectAudit  bool
}

func roleLookupName(role string) string {
	switch role {
	case "member":
		return controllerstorage.SystemRoleOperator
	case "owner":
		return controllerstorage.SystemRoleAdmin
	default:
		return role
	}
}

func expectTenantStatusActive(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM tenants WHERE id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
}

func withAuthContext(req *http.Request, role string, tenantID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.UserRoleKey, role)
	ctx = context.WithValue(ctx, middleware.TenantIDKey, tenantID)
	return req.WithContext(ctx)
}

func expectPermissionLookup(mock sqlmock.Sqlmock, tenantID uuid.UUID, role string, permissions []string) {
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`)).
		WithArgs(tenantID, roleLookupName(role)).
		WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{" + strings.Join(permissions, ",") + "}"))
}

func expectTokenListSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, token, tag, max_uses, used_count, expires_at, created_at, status 
		 FROM tokens WHERE tenant_id = $1 ORDER BY created_at DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "token", "tag", "max_uses", "used_count", "expires_at", "created_at", "status",
		}).AddRow(uuid.New(), "tk_demo", "default", 10, 1, now.Add(24*time.Hour), now, "active"))
}

func expectTenantUpdateSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE tenants 
		SET name = COALESCE(NULLIF($1, ''), name),
		    code = COALESCE(NULLIF($2, ''), code),
		    status = COALESCE(NULLIF($3, ''), status),
		    resource_quota = CASE WHEN $4 = '' THEN resource_quota ELSE $4 END,
		    updated_at = NOW()
		WHERE id = $5`)).
		WithArgs("", "", "", "", tenantID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectResolveAlertSuccess(mock sqlmock.Sqlmock, tenantID, alertID uuid.UUID) {
	now := time.Now()
	title := "CPU high usage"
	alertColumns := []string{
		"id", "tenant_id", "node_id", "alert_type", "severity", "title", "message",
		"context", "status", "created_at", "resolved_at",
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE id = $1`)).
		WithArgs(alertID).
		WillReturnRows(sqlmock.NewRows(alertColumns).AddRow(
			alertID,
			tenantID,
			nil,
			"cpu_high",
			"warning",
			title,
			"high cpu",
			[]byte(`{}`),
			"active",
			now.Add(-10*time.Minute),
			nil,
		))

	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE alerts
		SET status = 'resolved', resolved_at = NOW()
		WHERE id = $1 AND status = 'active'
		RETURNING id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		          context, status, created_at, resolved_at`)).
		WithArgs(alertID).
		WillReturnRows(sqlmock.NewRows(alertColumns).AddRow(
			alertID,
			tenantID,
			nil,
			"cpu_high",
			"warning",
			title,
			"high cpu",
			[]byte(`{}`),
			"resolved",
			now.Add(-10*time.Minute),
			now,
		))

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at`)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nil, "alert_resolved", "user", "Alert resolved: "+title, []byte(`{}`), now))
}

func expectRolesListSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, name, description, is_system, permissions, created_at, updated_at
		FROM roles WHERE tenant_id = $1 ORDER BY is_system DESC, name ASC
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "description", "is_system", "permissions", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, "viewer", "read only", true, "{nodes:read,roles:read}", now, now))
}

func expectUsersListSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, username, email, role FROM users WHERE tenant_id = $1 ORDER BY created_at DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "role"}).AddRow(uuid.New().String(), "alice", "alice@example.com", "viewer"))
}

func expectNodeLookup(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, routes string) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID,
			"pub-key-1",
			"machine-1",
			tenantID,
			"1.1.1.1:51820",
			"10.0.0.1",
			"1.1.1.1",
			"sh",
			"vpc-1",
			"node-1",
			"10.0.0.10",
			10,
			time.Now().Unix(),
			time.Now().Add(-time.Hour).Unix(),
			"member",
			"kernel",
			"6.0",
			true,
			"online",
			int64(0),
			routes,
			"",
			now,
			now,
		))
}

func expectNodeLifecycleTransitionByPublicKey(
	mock sqlmock.Sqlmock,
	publicKey string,
	tenantID, nodeID uuid.UUID,
	fromStatus, targetStatus, revokeReason, eventType, actor, summary string,
) {
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1 FOR UPDATE`)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, publicKey, "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, fromStatus, int64(0), "{}", "", now, now,
		))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE nodes
		SET status = $2, updated_at = NOW()
		WHERE public_key = $1
	`)).
		WithArgs(publicKey, targetStatus).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
			UPDATE node_certificates
			SET status = $2,
			    revoked_at = NOW(),
			    revoke_reason = $3,
			    updated_at = NOW()
			WHERE node_id = $1
	`)).
		WithArgs(nodeID, controllerstorage.CertStatusRevoked, revokeReason).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE policy_deliveries
		SET command_status = $2,
		    last_error = $3,
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE command_id IN (
			SELECT id
			FROM agent_commands
			WHERE node_public_key = $1 AND status IN ($4, $5, $6)
		)
	`)).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusFailed, "node status changed to "+targetStatus, controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE agent_commands
		SET status = $2,
		    message = $3,
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE node_public_key = $1 AND status IN ($4, $5, $6)
	`)).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusFailed, "node status changed to "+targetStatus, controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
			INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
			VALUES ($1, $2, $3, $4, $5, $6)
		`)).
		WithArgs(tenantID, nodeID, eventType, actor, summary, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func expectACLListSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
			        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
			        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
			        priority, enabled, COALESCE(description, ''),
			        created_at, updated_at
			   FROM acl_rules
			  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority DESC, created_at DESC`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow-web", "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", now, now))
}

func expectQoSListSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'policing'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND category = $3
		  ORDER BY priority ASC, created_at DESC`)).
		WithArgs(tenantID, nodeID, "runtime").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "category", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "runtime", "", "10.0.0.0/24", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 0, "policing", true, "https limit", now, now))
}

func expectRolesCreateSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO roles (tenant_id, name, description, is_system, permissions)
		VALUES ($1, $2, $3, false, $4)
		RETURNING id, tenant_id, name, description, is_system, permissions, created_at, updated_at`)).
		WithArgs(tenantID, "custom-operator", "custom", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "description", "is_system", "permissions", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, "custom-operator", "custom", false, "{roles:read,roles:write}", now, now))
}

func expectUsersCreateSuccess(mock sqlmock.Sqlmock, tenantID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users (id, username, password_hash, tenant_id, role, email, created_at) 
			  VALUES ($1, $2, $3, $4, $5, $6, NOW())`)).
		WithArgs(sqlmock.AnyArg(), "bob", sqlmock.AnyArg(), tenantID, "member", "bob@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectACLCreateSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
			 VALUES ($1, $2, $3, $4, NULLIF($5, '')::cidr, NULLIF($6, '')::cidr, $7, $8, $9, $10, $11, $12, $13, $14::cidr, $15::cidr, $16, $17)
			 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
			           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
			           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
			           priority, enabled, COALESCE(description, ''),
			           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow-web", "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", "10.0.0.0/24", "0.0.0.0/0", 443, 443).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow-web", "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", now, now))
}

func expectQoSCreateSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO qos_rules (tenant_id, node_id, category, src_cidr, dst_cidr, src_port, dst_port, protocol, bandwidth_mbps, direction, rate_bps, burst_bytes, priority, mode, enabled, description)
		 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, NULLIF($5, '')::cidr, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 RETURNING id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		           COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		           bandwidth_mbps, COALESCE(direction, 'egress'),
		           COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		           COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		           COALESCE(priority, 0), COALESCE(mode, 'policing'),
		           enabled, COALESCE(description, ''), created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "runtime", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 200, "egress", uint64(200000000), uint64(2500000), 100, "policing", true, "qos web").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "category", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "runtime", "", "10.0.0.0/24", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 100, "policing", true, "qos web", now, now))
}

func expectACLGetForUpdate(mock sqlmock.Sqlmock, tenantID, nodeID, ruleID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		        priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, "allow-web", "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", now, now))
}

func expectACLUpdatePreservingExistingFields(mock sqlmock.Sqlmock, tenantID, nodeID, ruleID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE acl_rules SET
			name = $4, action = $5, src_cidr = NULLIF($6, '')::cidr, dst_cidr = NULLIF($7, '')::cidr,
			dst_port = $8, protocol = $9, direction = $10, ports = $11, priority = $12, description = $13,
			enabled = $14, src_net = $15::cidr, dst_net = $16::cidr, min_port = $17, max_port = $18, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID, "allow-web", "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, "allow web", false, "10.0.0.0/24", "0.0.0.0/0", 443, 443).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectQoSGetForUpdate(mock sqlmock.Sqlmock, tenantID, nodeID, ruleID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'policing'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE id = $1 AND tenant_id = $2 AND node_id = $3 AND category = $4`)).
		WithArgs(ruleID, tenantID, nodeID, "runtime").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "category", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, "runtime", "", "10.0.0.0/24", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 100, "policing", true, "qos web", now, now))
}

func expectQoSUpdatePreservingExistingFields(mock sqlmock.Sqlmock, tenantID, nodeID, ruleID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE qos_rules SET
			src_cidr = NULLIF($5, ''), dst_cidr = NULLIF($6, ''),
			src_port = $7, dst_port = $8, protocol = $9,
			bandwidth_mbps = $10, direction = $11, rate_bps = $12, burst_bytes = $13,
			priority = $14, mode = $15, description = $16,
			enabled = $17, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND node_id = $3 AND category = $4`)).
		WithArgs(ruleID, tenantID, nodeID, "runtime", "", "10.0.0.0/24", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 100, "policing", "qos web", false).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectBumpDesiredState(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO node_control_states (tenant_id, node_id, desired_state_version, desired_state_updated_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (node_id) DO UPDATE SET
		    desired_state_version = EXCLUDED.desired_state_version,
		    desired_state_updated_at = NOW(),
		    updated_at = NOW()`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectPolicyDispatchSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	desiredMetadata := []byte(`{}`)
	commandParams := []byte(`{}`)
	controlStateColumns := []string{
		"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
		"applied_state_version", "applied_state_updated_at", "observed_state", "observed_message", "observed_at",
		"last_sync_at", "last_sync_error", "created_at", "updated_at",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO node_control_states (`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(controlStateColumns).AddRow(
			tenantID, nodeID, "desired-v1", desiredMetadata, now, "", nil, "", "", nil, nil, "", now, now,
		))

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO agent_commands (node_public_key, command, params, status, priority, timeout_seconds)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`)).
		WithArgs("pub-key-1", "sync", sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, 1, 60).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("cmd-1", now, now))

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO policy_deliveries (`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "cmd-1", controllerstorage.AgentCommandStatusPending, "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name", "action", "command_id", "command_status",
			"last_error", "metadata", "created_at", "updated_at", "completed_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "acl", "rule-ref", "rule-name", "create", "cmd-1", controllerstorage.AgentCommandStatusPending, "", []byte(`{}`), now, now, nil))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)
		FROM agent_commands
		WHERE node_public_key = $1
		  AND status IN ($2, $3, $4)`)).
		WithArgs("pub-key-1", controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1
		ORDER BY created_at DESC
		LIMIT 1`)).
		WithArgs("pub-key-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "node_public_key", "command", "params", "status", "message", "priority", "timeout_seconds",
			"created_at", "updated_at", "sent_at", "acknowledged_at", "completed_at", "result",
		}).AddRow("cmd-1", "pub-key-1", "sync", commandParams, controllerstorage.AgentCommandStatusPending, "", 1, 60, now, now, nil, nil, nil, []byte(`{}`)))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id, node_id, COALESCE(desired_state_version, ''), desired_state_metadata, desired_state_updated_at,
		       COALESCE(applied_state_version, ''), applied_state_updated_at, COALESCE(observed_state, ''),
		       COALESCE(observed_message, ''), observed_at, last_sync_at, COALESCE(last_sync_error, ''),
		       created_at, updated_at
		FROM node_control_states
		WHERE tenant_id = $1 AND node_id = $2`)).
		WithArgs(tenantID, nodeID).
		WillReturnError(sql.ErrNoRows)
}

func TestRBACHandlerMatrix_TokensRead(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses permission checks", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows granted read permission", mode: "enforce", role: "viewer", permissions: []string{"tokens:read"}, expectStatus: http.StatusOK},
		{name: "super admin bypasses role permissions", mode: "enforce", role: "super_admin", expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}

			if tc.expectStatus == http.StatusOK {
				expectTokenListSuccess(mock, tenantID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/tokens", nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if !tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "" {
				t.Fatalf("unexpected audit denied header")
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_SettingsWrite(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied write with marker", mode: "audit", role: "viewer", permissions: []string{"settings:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing write permission", mode: "enforce", role: "viewer", permissions: []string{"settings:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows granted write permission", mode: "enforce", role: "admin", permissions: []string{"settings:write"}, expectStatus: http.StatusOK},
		{name: "super admin bypasses role permissions", mode: "enforce", role: "super_admin", expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}

			if tc.expectStatus == http.StatusOK {
				expectTenantUpdateSuccess(mock, tenantID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(
				httptest.NewRequest(http.MethodPut, "/api/v2/tenants/"+tenantID.String(), strings.NewReader(`{}`)),
				tc.role,
				tenantID,
			)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if !tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "" {
				t.Fatalf("unexpected audit denied header")
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_CommandsWrite(t *testing.T) {
	tenantID := uuid.New()
	alertID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/monitoring/alerts/" + alertID.String() + "/resolve"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses commands permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied commands with marker", mode: "audit", role: "viewer", permissions: []string{"monitoring:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing commands permission", mode: "enforce", role: "viewer", permissions: []string{"monitoring:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows granted commands permission", mode: "enforce", role: "admin", permissions: []string{"commands:write"}, expectStatus: http.StatusOK},
		{name: "super admin bypasses role permissions", mode: "enforce", role: "super_admin", expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}

			if tc.expectStatus == http.StatusOK {
				expectResolveAlertSuccess(mock, tenantID, alertID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodPost, path, nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if !tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "" {
				t.Fatalf("unexpected audit denied header")
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_RolesRead(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses roles read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied roles read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing roles read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows roles read permission", mode: "enforce", role: "viewer", permissions: []string{"roles:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectRolesListSuccess(mock, tenantID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			// Call handler directly with a path shape that reaches role listing branch.
			req := withAuthContext(httptest.NewRequest(http.MethodGet, "/x/x/x/x/x/x/roles", nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.handleTenantRoles(rr, req, tenantID)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_UsersRead(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses users read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied users read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing users read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows users read permission", mode: "enforce", role: "admin", permissions: []string{"users:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectUsersListSuccess(mock, tenantID)
			}

			store := controllerstorage.NewStorageWithDB(db)
			router := &Router{
				store:     store,
				tenantAPI: handlers.NewTenantAPI(store),
			}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/users", nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.handleTenantUsers(rr, req, tenantID, tc.role)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_RoutesRead(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/routes/route-a"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses routes read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied routes read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing routes read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows routes read permission", mode: "enforce", role: "viewer", permissions: []string{"routes:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			// First lookup happens in handleTenantNodes before route permission check.
			expectNodeLookup(mock, tenantID, nodeID, "{route-a}")
			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				// Additional lookups happen in handleTenantNodeRoutes and getTenantNodeRoute.
				expectNodeLookup(mock, tenantID, nodeID, "{route-a}")
				expectNodeLookup(mock, tenantID, nodeID, "{route-a}")
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, path, nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_ACLsRead(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/security/acls"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses ACL read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied ACL read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing ACL read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows ACL read permission", mode: "enforce", role: "viewer", permissions: []string{"acls:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectACLListSuccess(mock, tenantID, nodeID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, path, nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_QoSRead(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/qos"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses QoS read permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied QoS read with marker", mode: "audit", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing QoS read permission", mode: "enforce", role: "viewer", permissions: []string{"nodes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows QoS read permission", mode: "enforce", role: "viewer", permissions: []string{"qos:read"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectQoSListSuccess(mock, tenantID, nodeID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodGet, path, nil), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestTenantNodeQoSRejectsLegacyCategoryListPath(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/qos/service"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "admin", []string{"qos:read"})

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(http.MethodGet, path, nil), "admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusMethodNotAllowed, rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestRBACHandlerMatrix_RolesWrite(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses roles write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied roles write with marker", mode: "audit", role: "viewer", permissions: []string{"roles:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing roles write permission", mode: "enforce", role: "viewer", permissions: []string{"roles:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows roles write permission", mode: "enforce", role: "admin", permissions: []string{"roles:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectRolesCreateSuccess(mock, tenantID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				"/x/x/x/x/x/x/roles",
				strings.NewReader(`{"name":"custom-operator","description":"custom","permissions":["roles:read","roles:write"]}`),
			), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.handleTenantRoles(rr, req, tenantID)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_UsersWrite(t *testing.T) {
	tenantID := uuid.New()
	cases := []handlerMatrixCase{
		{name: "off mode bypasses users write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied users write with marker", mode: "audit", role: "viewer", permissions: []string{"users:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing users write permission", mode: "enforce", role: "viewer", permissions: []string{"users:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows users write permission", mode: "enforce", role: "admin", permissions: []string{"users:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectUsersCreateSuccess(mock, tenantID)
			}

			store := controllerstorage.NewStorageWithDB(db)
			router := &Router{
				store:     store,
				tenantAPI: handlers.NewTenantAPI(store),
			}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				"/api/v2/tenants/"+tenantID.String()+"/users",
				strings.NewReader(`{"username":"bob","password":"P@ssw0rd","email":"bob@example.com","role":"member"}`),
			), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.handleTenantUsers(rr, req, tenantID, tc.role)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_RoutesWrite(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/routes"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses routes write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied routes write with marker", mode: "audit", role: "viewer", permissions: []string{"routes:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing routes write permission", mode: "enforce", role: "viewer", permissions: []string{"routes:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows routes write permission", mode: "enforce", role: "admin", permissions: []string{"routes:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{10.0.0.0/24}")
			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				// Route POST hits addTenantNodeRoute with secondary lookup.
				expectNodeLookup(mock, tenantID, nodeID, "{10.0.0.0/24}")
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"cidr":"10.0.0.0/24"}`)), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestRBACHandlerMatrix_ACLsWrite(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/security/acls"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses ACL write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied ACL write with marker", mode: "audit", role: "viewer", permissions: []string{"acls:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing ACL write permission", mode: "enforce", role: "viewer", permissions: []string{"acls:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows ACL write permission", mode: "enforce", role: "admin", permissions: []string{"acls:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectACLCreateSuccess(mock, tenantID, nodeID)
				expectBumpDesiredState(mock, tenantID, nodeID)
				expectPolicyDispatchSuccess(mock, tenantID, nodeID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				path,
				strings.NewReader(`{"name":"allow-web","action":"allow","src_cidr":"10.0.0.0/24","dst_cidr":"0.0.0.0/0","dst_port":443,"protocol":6,"priority":100,"description":"allow web"}`),
			), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestACLUpdatePreservesOmittedFields(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/security/acls/" + ruleID.String()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "admin", []string{"acls:write"})
	expectACLGetForUpdate(mock, tenantID, nodeID, ruleID)
	expectACLUpdatePreservingExistingFields(mock, tenantID, nodeID, ruleID)
	expectBumpDesiredState(mock, tenantID, nodeID)
	expectPolicyDispatchSuccess(mock, tenantID, nodeID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPut,
		path,
		strings.NewReader(`{"enabled":false}`),
	), "admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestRBACHandlerMatrix_QoSWrite(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/qos"
	cases := []handlerMatrixCase{
		{name: "off mode bypasses QoS write permission", mode: "off", role: "member", expectStatus: http.StatusOK},
		{name: "audit mode allows denied QoS write with marker", mode: "audit", role: "viewer", permissions: []string{"qos:read"}, expectStatus: http.StatusOK, expectAudit: true},
		{name: "enforce mode denies missing QoS write permission", mode: "enforce", role: "viewer", permissions: []string{"qos:read"}, expectStatus: http.StatusForbidden},
		{name: "enforce mode allows QoS write permission", mode: "enforce", role: "admin", permissions: []string{"qos:write"}, expectStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RBAC_ENFORCEMENT", tc.mode)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			if tc.role != "super_admin" {
				expectTenantStatusActive(mock, tenantID)
			}

			if tc.mode != "off" && tc.role != "super_admin" {
				expectPermissionLookup(mock, tenantID, tc.role, tc.permissions)
			}
			if tc.expectStatus == http.StatusOK {
				expectQoSCreateSuccess(mock, tenantID, nodeID)
				expectBumpDesiredState(mock, tenantID, nodeID)
				expectPolicyDispatchSuccess(mock, tenantID, nodeID)
			}

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				path,
				strings.NewReader(`{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":200,"priority":100,"description":"qos web"}`),
			), tc.role, tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != tc.expectStatus {
				t.Fatalf("expected status %d, got %d", tc.expectStatus, rr.Code)
			}
			if tc.expectAudit && rr.Header().Get("X-RBAC-Audit-Denied") != "true" {
				t.Fatalf("expected audit denied header")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestQoSUpdatePreservesOmittedFields(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/qos/" + ruleID.String()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "admin", []string{"qos:write"})
	expectQoSGetForUpdate(mock, tenantID, nodeID, ruleID)
	expectQoSUpdatePreservingExistingFields(mock, tenantID, nodeID, ruleID)
	expectBumpDesiredState(mock, tenantID, nodeID)
	expectPolicyDispatchSuccess(mock, tenantID, nodeID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPut,
		path,
		strings.NewReader(`{"enabled":false}`),
	), "admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestQoSWriteRejectsZeroBandwidth(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/qos"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "admin", []string{"qos:write"})

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPost,
		path,
		strings.NewReader(`{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":0,"description":"invalid qos"}`),
	), "admin", tenantID)
	rr := httptest.NewRecorder()
	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
