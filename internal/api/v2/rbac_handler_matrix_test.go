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
			WHERE node_id = $1 AND status = $4
	`)).
		WithArgs(nodeID, controllerstorage.CertStatusRevoked, revokeReason, controllerstorage.CertStatusIssued).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
	`)).
		WithArgs(tenantID, nodeID, controllerstorage.AuditCertRevoked, actor, "Node certificate revoked due to node lifecycle change", sqlmock.AnyArg()).
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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
			        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
			        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
			        priority, enabled, COALESCE(description, ''),
			        created_at, updated_at
			   FROM acl_rules
			  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority ASC, created_at ASC`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow-web", "allow", nil, nil, "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", now, now))
	expectNoPolicyStats(mock, tenantID, nodeID)
	expectPolicyDeliveryListEmpty(mock, tenantID, nodeID, "acl")
}

func expectACLRuntimeConflictCheckEmpty(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	expectACLConflictCheckEmpty(mock, tenantID, nodeID)
}

func expectACLConflictCheckEmpty(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
			        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
			        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
			        priority, enabled, COALESCE(description, ''),
			        created_at, updated_at
			   FROM acl_rules
			  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority ASC, created_at ASC`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}))
}

func expectQoSListSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'auto'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority ASC, created_at DESC`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "group_id", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, nil, "", "10.0.0.0/24", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 0, "policing", true, "https limit", now, now))
	expectNoPolicyStats(mock, tenantID, nodeID)
	expectPolicyDeliveryListEmpty(mock, tenantID, nodeID, "qos")
}

func expectQoSConflictCheckEmpty(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'auto'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority ASC, created_at DESC`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "group_id", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}))
}

func expectNoPolicyStats(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id, node_id, stats, updated_at
			FROM node_policy_stats
			WHERE tenant_id = $1 AND node_id = $2`)).
		WithArgs(tenantID, nodeID).
		WillReturnError(sql.ErrNoRows)
}

func expectPolicyDeliveryListEmpty(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, domain string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, policy_domain, policy_ref, COALESCE(policy_name, ''), action, command_id, command_status,
		       COALESCE(last_error, ''), metadata, created_at, updated_at, completed_at
		FROM policy_deliveries
		WHERE tenant_id = $1 AND node_id = $2 AND policy_domain = $3
		ORDER BY created_at DESC
		LIMIT $4`)).
		WithArgs(tenantID, nodeID, domain, 100).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name",
			"action", "command_id", "command_status", "last_error", "metadata", "created_at", "updated_at", "completed_at",
		}))
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
	expectACLCreateSuccessWithEnabled(mock, tenantID, nodeID, true)
}

func expectACLCreateSuccessWithEnabled(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, enabled bool) {
	now := time.Now()
	srcGroupID := expectInlineGroupResolve(mock, tenantID, "10.0.0.0/24")
	expectACLConflictCheckEmpty(mock, tenantID, nodeID)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_group_id, dst_group_id, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
			 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::cidr, NULLIF($8, '')::cidr, $9, $10, $11, $12, $13, $14, $15, $16::cidr, $17::cidr, $18, $19)
			 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
			           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
			           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
			           priority, enabled, COALESCE(description, ''),
			           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow-web", "allow", srcGroupID, nil, "", "", 443, 6, "ingress", "443", 100, enabled, "allow web", "0.0.0.0/0", "0.0.0.0/0", 443, 443).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow-web", "allow", srcGroupID, nil, "0.0.0.0/0", "0.0.0.0/0", 443, 6, "ingress", "443", 100, enabled, "allow web", now, now))
}

func expectACLCreateSuccessWithAnyCIDRs(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	expectACLConflictCheckEmpty(mock, tenantID, nodeID)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_group_id, dst_group_id, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
			 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::cidr, NULLIF($8, '')::cidr, $9, $10, $11, $12, $13, $14, $15, $16::cidr, $17::cidr, $18, $19)
			 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
			           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
			           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
			           priority, enabled, COALESCE(description, ''),
			           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow-any", "allow", nil, nil, "", "", nil, 1, "egress", "", 100, true, "allow any", "0.0.0.0/0", "0.0.0.0/0", 0, 65535).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow-any", "allow", nil, nil, "0.0.0.0/0", "0.0.0.0/0", 0, 1, "egress", "", 100, true, "allow any", now, now))
}

func expectQoSCreateSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	expectQoSCreateSuccessWithEnabled(mock, tenantID, nodeID, true)
}

func expectQoSCreateSuccessWithEnabled(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, enabled bool) {
	expectQoSCreateSuccessWithModeAndEnabled(mock, tenantID, nodeID, "auto", enabled)
}

func expectQoSCreateSuccessWithModeAndEnabled(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, mode string, enabled bool) {
	now := time.Now()
	groupID := expectInlineGroupResolve(mock, tenantID, "10.0.0.0/24")
	expectQoSConflictCheckEmpty(mock, tenantID, nodeID)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO qos_rules (tenant_id, node_id, group_id, src_cidr, dst_cidr, src_port, dst_port, protocol, bandwidth_mbps, direction, rate_bps, burst_bytes, priority, mode, enabled, description)
		 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, NULLIF($5, '')::cidr, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 RETURNING id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		           COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		           bandwidth_mbps, COALESCE(direction, 'egress'),
		           COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		           COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		           COALESCE(priority, 0), COALESCE(mode, 'auto'),
		           enabled, COALESCE(description, ''), created_at, updated_at`)).
		WithArgs(tenantID, nodeID, groupID, "", "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 200, "egress", uint64(200000000), uint64(2500000), 100, mode, enabled, "qos web").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "group_id", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, groupID, "", "", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 100, mode, enabled, "qos web", now, now))
}

func expectQoSCreateSuccessWithAnyCIDR(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	expectQoSConflictCheckEmpty(mock, tenantID, nodeID)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO qos_rules (tenant_id, node_id, group_id, src_cidr, dst_cidr, src_port, dst_port, protocol, bandwidth_mbps, direction, rate_bps, burst_bytes, priority, mode, enabled, description)
		 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, NULLIF($5, '')::cidr, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 RETURNING id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		           COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		           bandwidth_mbps, COALESCE(direction, 'egress'),
		           COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		           COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		           COALESCE(priority, 0), COALESCE(mode, 'auto'),
		           enabled, COALESCE(description, ''), created_at, updated_at`)).
		WithArgs(tenantID, nodeID, nil, "", "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 200, "egress", uint64(200000000), uint64(2500000), 100, "auto", true, "qos any").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "group_id", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, nil, "", "", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 100, "auto", true, "qos any", now, now))
}

func expectInlineGroupResolve(mock sqlmock.Sqlmock, tenantID uuid.UUID, cidr string) uuid.UUID {
	groupID := uuid.New()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO ip_groups (tenant_id, name, description, kind)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, name) DO UPDATE SET updated_at = NOW()
		 RETURNING id, tenant_id, name, COALESCE(description, ''), kind, created_by, created_at, updated_at`)).
		WithArgs(tenantID, sqlmock.AnyArg(), "inline policy group", controllerstorage.IPGroupKindInline).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "kind", "created_by", "created_at", "updated_at"}).
			AddRow(groupID, tenantID, "inline-test", "inline policy group", controllerstorage.IPGroupKindInline, nil, now, now))
	expectNoInlineDuplicateIPGroupMember(mock, tenantID, groupID, cidr)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO ip_group_members (tenant_id, group_id, cidr, note)
		 VALUES ($1, $2, $3::cidr, $4)
		 ON CONFLICT (group_id, cidr) DO UPDATE SET note = EXCLUDED.note`)).
		WithArgs(tenantID, groupID, cidr, "inline").
		WillReturnResult(sqlmock.NewResult(0, 1))
	return groupID
}

func expectNoInlineDuplicateIPGroupMember(mock sqlmock.Sqlmock, tenantID, excludeGroupID uuid.UUID, cidr string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT g.id, g.name, m.cidr::text
		   FROM ip_group_members m
		   JOIN ip_groups g ON g.id = m.group_id
		  WHERE m.tenant_id = $1
		    AND g.id <> $2
		    AND m.cidr = $3::cidr
		  LIMIT 1`)).
		WithArgs(tenantID, excludeGroupID, cidr).
		WillReturnError(sql.ErrNoRows)
}

func expectACLGetForUpdate(mock sqlmock.Sqlmock, tenantID, nodeID, ruleID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		        priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, "allow-web", "allow", nil, nil, "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", now, now))
}

func expectACLUpdatePreservingExistingFields(mock sqlmock.Sqlmock, tenantID, nodeID, ruleID uuid.UUID) {
	srcGroupID := expectInlineGroupResolve(mock, tenantID, "10.0.0.0/24")
	expectACLConflictCheckEmpty(mock, tenantID, nodeID)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE acl_rules SET
			name = $4, action = $5, src_group_id = $6, dst_group_id = $7, src_cidr = NULLIF($8, '')::cidr, dst_cidr = NULLIF($9, '')::cidr,
			dst_port = $10, protocol = $11, direction = $12, ports = $13, priority = $14, description = $15,
			enabled = $16, src_net = $17::cidr, dst_net = $18::cidr, min_port = $19, max_port = $20, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID, "allow-web", "allow", srcGroupID, nil, "", "0.0.0.0/0", 443, 6, "ingress", "443", 100, "allow web", false, "0.0.0.0/0", "0.0.0.0/0", 443, 443).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectQoSGetForUpdate(mock sqlmock.Sqlmock, tenantID, nodeID, ruleID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'auto'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "group_id", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, nil, "", "10.0.0.0/24", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 100, "policing", true, "qos web", now, now))
}

func expectQoSUpdatePreservingExistingFields(mock sqlmock.Sqlmock, tenantID, nodeID, ruleID uuid.UUID) {
	groupID := expectInlineGroupResolve(mock, tenantID, "10.0.0.0/24")
	expectQoSConflictCheckEmpty(mock, tenantID, nodeID)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE qos_rules SET
			group_id = $4, src_cidr = NULLIF($5, '')::cidr, dst_cidr = NULLIF($6, '')::cidr,
			src_port = $7, dst_port = $8, protocol = $9,
			bandwidth_mbps = $10, direction = $11, rate_bps = $12, burst_bytes = $13,
			priority = $14, mode = $15, description = $16,
			enabled = $17, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID, groupID, "", "", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 100, "policing", "qos web", false).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectPolicyDispatchSuccess(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectBegin()
	expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
	mock.ExpectCommit()
	expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)
}

func expectPolicyDispatchSuccessInOpenTx(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	desiredMetadata := []byte(`{}`)
	controlStateColumns := []string{
		"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
		"applied_state_version", "applied_state_updated_at", "observed_state", "observed_message", "observed_at",
		"last_sync_at", "last_sync_error", "created_at", "updated_at",
	}

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO node_control_states (`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(controlStateColumns).AddRow(
			tenantID, nodeID, "desired-v1", desiredMetadata, now, "", nil, "", "", nil, nil, "", now, now,
		))

	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands ac")).
		WithArgs("pub-key-1", controllerstorage.AgentCommandStatusStale, sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged, tenantID, nodeID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(tenantID, nodeID, controllerstorage.AgentCommandStatusStale, sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO agent_commands (node_public_key, command, params, status, priority, timeout_seconds, deadline_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW() + make_interval(secs => $6))
		RETURNING id, created_at, updated_at
	`)).
		WithArgs("pub-key-1", "sync", sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, 1, 60).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("cmd-1", now, now))

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO policy_deliveries (`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "cmd-1", controllerstorage.AgentCommandStatusPending, "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name", "action", "command_id", "command_status",
			"last_error", "metadata", "created_at", "updated_at", "completed_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "acl", "rule-ref", "rule-name", "create", "cmd-1", controllerstorage.AgentCommandStatusPending, "", []byte(`{}`), now, now, nil))
}

func expectPolicyDispatchPostCommitSummary(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	now := time.Now()
	commandParams := []byte(`{}`)

	expectPolicyDispatchAuditEvents(mock, tenantID, nodeID, now)

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

func expectPolicyDispatchAuditEvents(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, now time.Time) {
	auditColumns := []string{
		"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
	}

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)`)).
		WithArgs(tenantID, sqlmock.AnyArg(), controllerstorage.AuditCommandQueued, "controller", "Command queued: sync", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(auditColumns).AddRow(uuid.New(), tenantID, nodeID.String(), controllerstorage.AuditCommandQueued, "controller", "Command queued: sync", []byte(`{}`), now))

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)`)).
		WithArgs(tenantID, sqlmock.AnyArg(), controllerstorage.AuditPolicyChanged, "user", "Policy changed", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(auditColumns).AddRow(uuid.New(), tenantID, nodeID.String(), controllerstorage.AuditPolicyChanged, "user", "Policy changed", []byte(`{}`), now))
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
				mock.ExpectBegin()
				expectACLCreateSuccess(mock, tenantID, nodeID)
				expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
				mock.ExpectCommit()
				expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)
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

func TestACLCreateHonorsDisabledPayload(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/security/acls"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "admin", []string{"acls:write"})
	mock.ExpectBegin()
	expectACLCreateSuccessWithEnabled(mock, tenantID, nodeID, false)
	expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
	mock.ExpectCommit()
	expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPost,
		path,
		strings.NewReader(`{"name":"allow-web","action":"allow","src_cidr":"10.0.0.0/24","dst_cidr":"0.0.0.0/0","dst_port":443,"protocol":6,"priority":100,"enabled":false,"description":"allow web"}`),
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

func TestACLCreateTreatsAnyCIDRAsEmptyMatch(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/security/acls"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectTenantStatusActive(mock, tenantID)
	expectPermissionLookup(mock, tenantID, "admin", []string{"acls:write"})
	mock.ExpectBegin()
	expectACLCreateSuccessWithAnyCIDRs(mock, tenantID, nodeID)
	expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
	mock.ExpectCommit()
	expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPost,
		path,
		strings.NewReader(`{"name":"allow-any","action":"allow","src_cidr":"any","dst_cidr":"any","protocol":1,"direction":"egress","priority":100,"description":"allow any"}`),
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
	mock.ExpectBegin()
	expectACLUpdatePreservingExistingFields(mock, tenantID, nodeID, ruleID)
	expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
	mock.ExpectCommit()
	expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)

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

func TestACLWriteRejectsRuntimeCompilerInvalidFields(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/security/acls"

	cases := []struct {
		name    string
		payload string
	}{
		{
			name:    "protocol exceeds uint8",
			payload: `{"name":"bad-acl","action":"deny","src_cidr":"10.0.0.0/24","protocol":300}`,
		},
		{
			name:    "invalid direction",
			payload: `{"name":"bad-acl","action":"deny","src_cidr":"10.0.0.0/24","protocol":6,"direction":"sideways"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectNodeLookup(mock, tenantID, nodeID, "{}")
			expectTenantStatusActive(mock, tenantID)
			expectPermissionLookup(mock, tenantID, "admin", []string{"acls:write"})

			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := withAuthContext(httptest.NewRequest(
				http.MethodPost,
				path,
				strings.NewReader(tc.payload),
			), "admin", tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d, body=%s", http.StatusBadRequest, rr.Code, rr.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
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
				mock.ExpectBegin()
				expectQoSCreateSuccess(mock, tenantID, nodeID)
				expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
				mock.ExpectCommit()
				expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)
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

func TestQoSCreateHonorsDisabledPayload(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

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
	mock.ExpectBegin()
	expectQoSCreateSuccessWithEnabled(mock, tenantID, nodeID, false)
	expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
	mock.ExpectCommit()
	expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPost,
		path,
		strings.NewReader(`{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":200,"priority":100,"enabled":false,"description":"qos web"}`),
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

func TestQoSCreateTreatsAnyCIDRAsEmptyMatch(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

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
	mock.ExpectBegin()
	expectQoSCreateSuccessWithAnyCIDR(mock, tenantID, nodeID)
	expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
	mock.ExpectCommit()
	expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPost,
		path,
		strings.NewReader(`{"dst_cidr":"any","bandwidth_mbps":200,"priority":100,"description":"qos any"}`),
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

func TestQoSCreateAllowsShapingMode(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

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
	mock.ExpectBegin()
	expectQoSCreateSuccessWithModeAndEnabled(mock, tenantID, nodeID, "shaping", true)
	expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
	mock.ExpectCommit()
	expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withAuthContext(httptest.NewRequest(
		http.MethodPost,
		path,
		strings.NewReader(`{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":200,"priority":100,"mode":"shaping","description":"qos web"}`),
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
	mock.ExpectBegin()
	expectQoSUpdatePreservingExistingFields(mock, tenantID, nodeID, ruleID)
	expectPolicyDispatchSuccessInOpenTx(mock, tenantID, nodeID)
	mock.ExpectCommit()
	expectPolicyDispatchPostCommitSummary(mock, tenantID, nodeID)

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

func TestQoSWriteRejectsRuntimeCompilerInvalidFields(t *testing.T) {
	t.Setenv("RBAC_ENFORCEMENT", "enforce")

	tenantID := uuid.New()
	nodeID := uuid.New()
	path := "/api/v2/tenants/" + tenantID.String() + "/nodes/" + nodeID.String() + "/qos"

	cases := []struct {
		name    string
		payload string
	}{
		{
			name:    "priority exceeds uint8",
			payload: `{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":1,"priority":65000}`,
		},
		{
			name:    "protocol exceeds uint8",
			payload: `{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":1,"protocol":300}`,
		},
		{
			name:    "protocol matching unsupported",
			payload: `{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":1,"protocol":6}`,
		},
		{
			name:    "legacy zero protocol field unsupported",
			payload: `{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":1,"protocol":0}`,
		},
		{
			name:    "port matching unsupported",
			payload: `{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":1,"dst_port":443}`,
		},
		{
			name:    "legacy zero port field unsupported",
			payload: `{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":1,"dst_port":0}`,
		},
		{
			name:    "invalid direction",
			payload: `{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":1,"direction":"sideways"}`,
		},
		{
			name:    "invalid mode",
			payload: `{"dst_cidr":"10.0.0.0/24","bandwidth_mbps":1,"mode":"burst-only"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
				strings.NewReader(tc.payload),
			), "admin", tenantID)
			rr := httptest.NewRecorder()
			router.HandleTenantScoped(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d, body=%s", http.StatusBadRequest, rr.Code, rr.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}
