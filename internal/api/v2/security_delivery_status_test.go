package v2

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/api/apibase"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestListTenantNodeACLsIncludesCompletedDeliveryStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	deliveryID := uuid.New()
	commandID := uuid.New().String()

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
		}).AddRow(ruleID, tenantID, nodeID, "allow-icmp", "allow", nil, nil, "100.64.0.27/32", "100.64.0.2/32", 0, 1, "egress", "", 10, true, "allow icmp", now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id, node_id, stats, updated_at
		FROM node_policy_stats
		WHERE tenant_id = $1 AND node_id = $2`)).
		WithArgs(tenantID, nodeID).
		WillReturnError(sql.ErrNoRows)
	expectCompletedPolicyDeliveryList(mock, tenantID, nodeID, "acl", ruleID.String(), deliveryID, commandID, now)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.listTenantNodeACLs(rr, tenantID, nodeID)

	assertPolicyStatusResponse(t, rr, ruleID.String(), commandID)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListTenantNodeQoSIncludesCompletedDeliveryStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	deliveryID := uuid.New()
	commandID := uuid.New().String()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'policing'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority ASC, created_at DESC`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "group_id", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, nil, "100.64.0.27/32", "100.64.0.2/32", 0, 5201, 6, 1, "egress", uint64(1000000), uint64(125000), 10, "policing", true, "tcp limit", now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id, node_id, stats, updated_at
		FROM node_policy_stats
		WHERE tenant_id = $1 AND node_id = $2`)).
		WithArgs(tenantID, nodeID).
		WillReturnError(sql.ErrNoRows)
	expectCompletedPolicyDeliveryList(mock, tenantID, nodeID, "qos", ruleID.String(), deliveryID, commandID, now)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.listTenantNodeQoS(rr, tenantID, nodeID)

	assertPolicyStatusResponse(t, rr, ruleID.String(), commandID)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestPolicyDeliveryErrorResolverUsesPolicyNameForRuntimeGroupCIDRConflict(t *testing.T) {
	rawError := "sync failed: sync apply failed: acl_qos: Identity error: Invalid CIDR: CIDR 100.64.0.2/32 already belongs to runtime group 2"
	delivery := &controllerstorage.PolicyDelivery{
		PolicyDomain:  "qos",
		PolicyRef:     "new-qos-rule",
		PolicyName:    "new QoS rule",
		CommandStatus: "failed",
		LastError:     rawError,
	}

	resolver := policyDeliveryCIDRErrorResolver([]policyCIDRUsage{
		{
			policyRef:  "existing-rule",
			policyName: "two-node-icmp-allow",
			domain:     "acl",
			cidr:       "100.64.0.2/32",
		},
		{
			policyRef:  "new-qos-rule",
			policyName: "new QoS rule",
			domain:     "qos",
			cidr:       "100.64.0.2/32",
		},
	})

	payload := policyDeliveryToMapWithErrorResolver(delivery, resolver)
	lastError, _ := payload["last_error"].(string)
	rawLastError, _ := payload["raw_last_error"].(string)

	if !strings.Contains(lastError, "two-node-icmp-allow") {
		t.Fatalf("expected friendly error to include existing policy name, got %q", lastError)
	}
	if strings.Contains(lastError, "runtime group 2") {
		t.Fatalf("friendly error should not expose runtime group id, got %q", lastError)
	}
	if rawLastError != rawError {
		t.Fatalf("expected raw_last_error to preserve original error, got %q", rawLastError)
	}
}

func TestListTenantNodeACLsReturnsStatsLoadError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	statsErr := errors.New("stats json decode failed")

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
		}).AddRow(ruleID, tenantID, nodeID, "allow-icmp", "allow", nil, nil, "100.64.0.27/32", "100.64.0.2/32", 0, 1, "egress", "", 10, true, "allow icmp", now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id, node_id, stats, updated_at
			FROM node_policy_stats
			WHERE tenant_id = $1 AND node_id = $2`)).
		WithArgs(tenantID, nodeID).
		WillReturnError(statsErr)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.listTenantNodeACLs(rr, tenantID, nodeID)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListTenantNodeQoSReturnsStatsLoadError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	statsErr := errors.New("stats query failed")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
			        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
			        bandwidth_mbps, COALESCE(direction, 'egress'),
			        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
			        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
			        COALESCE(priority, 0), COALESCE(mode, 'policing'),
			        enabled, COALESCE(description, ''), created_at, updated_at
			   FROM qos_rules
			  WHERE tenant_id = $1 AND node_id = $2
			  ORDER BY priority ASC, created_at DESC`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "group_id", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, nil, "100.64.0.27/32", "100.64.0.2/32", 0, 0, 0, 1, "egress", uint64(1000000), uint64(125000), 10, "policing", true, "tcp limit", now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id, node_id, stats, updated_at
			FROM node_policy_stats
			WHERE tenant_id = $1 AND node_id = $2`)).
		WithArgs(tenantID, nodeID).
		WillReturnError(statsErr)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.listTenantNodeQoS(rr, tenantID, nodeID)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestDeleteTenantNodeACLReturnsNotFoundWhenRuleMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	node := &controllerstorage.Node{ID: nodeID, TenantID: tenantID, PublicKey: "pub-key-1"}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM acl_rules WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.deleteTenantNodeACL(rr, tenantID, node, ruleID.String())

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestDeleteTenantNodeQoSReturnsNotFoundWhenRuleMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	node := &controllerstorage.Node{ID: nodeID, TenantID: tenantID, PublicKey: "pub-key-1"}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM qos_rules WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.deleteTenantNodeQoS(rr, tenantID, node, ruleID.String())

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestDeleteTenantNodeBlacklistReturnsNotFoundWhenRuleMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	node := &controllerstorage.Node{ID: nodeID, TenantID: tenantID, PublicKey: "pub-key-1"}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM blacklist_rules WHERE id = $1 AND tenant_id = $2 AND node_id = $3 AND scope = $4`)).
		WithArgs(ruleID, tenantID, nodeID, controllerstorage.BlacklistScopeSrc).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.deleteTenantNodeBlacklistRule(rr, tenantID, node, controllerstorage.BlacklistScopeSrc, ruleID.String())

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestPolicyMutationsRejectInactiveNodes(t *testing.T) {
	tests := []struct {
		name   string
		status string
		run    func(*Router, http.ResponseWriter, *controllerstorage.Node, uuid.UUID)
	}{
		{
			name:   "acl create suspended",
			status: "suspended",
			run: func(router *Router, rr http.ResponseWriter, node *controllerstorage.Node, tenantID uuid.UUID) {
				req := httptest.NewRequest(http.MethodPost, "/",
					strings.NewReader(`{"name":"deny-icmp","action":"deny","src_cidr":"any","dst_cidr":"any","protocol":1,"direction":"ingress","enabled":true}`))
				router.createTenantNodeACL(rr, req, tenantID, node)
			},
		},
		{
			name:   "qos create banned",
			status: "banned",
			run: func(router *Router, rr http.ResponseWriter, node *controllerstorage.Node, tenantID uuid.UUID) {
				req := httptest.NewRequest(http.MethodPost, "/",
					strings.NewReader(`{"dst_cidr":"100.64.0.2/32","bandwidth_mbps":10,"direction":"egress","mode":"policing","enabled":true}`))
				router.createTenantNodeQoS(rr, req, tenantID, node)
			},
		},
		{
			name:   "blacklist create suspended",
			status: "suspended",
			run: func(router *Router, rr http.ResponseWriter, node *controllerstorage.Node, tenantID uuid.UUID) {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"cidr":"203.0.113.250/32"}`))
				router.createTenantNodeBlacklistRule(rr, req, tenantID, node, controllerstorage.BlacklistScopeSrc)
			},
		},
		{
			name:   "route create suspended",
			status: "suspended",
			run: func(router *Router, rr http.ResponseWriter, node *controllerstorage.Node, tenantID uuid.UUID) {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"cidr":"10.255.0.0/24"}`))
				router.addTenantNodeRoute(rr, req, tenantID, node)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			tenantID := uuid.New()
			node := &controllerstorage.Node{
				ID:        uuid.New(),
				TenantID:  tenantID,
				PublicKey: "inactive-node-key",
				Hostname:  "inactive-node",
				Status:    tc.status,
			}
			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			rr := httptest.NewRecorder()

			tc.run(router, rr, node, tenantID)

			if rr.Code != http.StatusConflict {
				t.Fatalf("expected 409, got %d body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tc.status) {
				t.Fatalf("expected response to mention status %q, body=%s", tc.status, rr.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func expectCompletedPolicyDeliveryList(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, domain, policyRef string, deliveryID uuid.UUID, commandID string, now time.Time) {
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
		}).AddRow(
			deliveryID, tenantID, nodeID, domain, policyRef, "policy",
			"update", commandID, controllerstorage.AgentCommandStatusCompleted, "", []byte(`{}`), now, now, now,
		))
}

func assertPolicyStatusResponse(t *testing.T, rr *httptest.ResponseRecorder, ruleID, commandID string) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal response data: %v", err)
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d: %#v", len(rows), rows)
	}
	row := rows[0]
	if row["id"] != ruleID {
		t.Fatalf("expected rule id %s, got %#v", ruleID, row["id"])
	}
	if row["policy_status"] != "applied" {
		t.Fatalf("expected policy_status=applied, got %#v in row %#v", row["policy_status"], row)
	}
	if row["pending_cmds"] != float64(0) {
		t.Fatalf("expected pending_cmds=0, got %#v in row %#v", row["pending_cmds"], row)
	}
	lastDelivery, ok := row["last_delivery"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected last_delivery object, got %#v in row %#v", row["last_delivery"], row)
	}
	if lastDelivery["command_id"] != commandID {
		t.Fatalf("expected command_id %s, got %#v", commandID, lastDelivery["command_id"])
	}
	if lastDelivery["command_status"] != controllerstorage.AgentCommandStatusCompleted {
		t.Fatalf("expected command_status completed, got %#v", lastDelivery["command_status"])
	}
	history, ok := row["delivery_history"].([]interface{})
	if !ok || len(history) != 1 {
		t.Fatalf("expected one delivery history entry, got %#v", row["delivery_history"])
	}
}
