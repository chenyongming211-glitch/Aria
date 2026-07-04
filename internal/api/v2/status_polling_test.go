package v2

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestPolicyDeliveryStatusEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	aclRef := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").String()
	routeRef := "10.20.0.0/16"
	aclDeliveryID := uuid.New()
	routeDeliveryID := uuid.New()
	aclCommandID := uuid.New().String()
	routeCommandID := uuid.New().String()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT ON (node_id, policy_domain, policy_ref)
			id, tenant_id, node_id, policy_domain, policy_ref, COALESCE(policy_name, ''), action, command_id, command_status,
			COALESCE(last_error, ''), metadata, created_at, updated_at, completed_at
		FROM policy_deliveries
		WHERE tenant_id = $1 AND ((node_id = $2 AND policy_domain = $3 AND policy_ref = $4) OR (node_id = $5 AND policy_domain = $6 AND policy_ref = $7))
		ORDER BY node_id, policy_domain, policy_ref, created_at DESC`)).
		WithArgs(tenantID, nodeID, "acl", aclRef, nodeID, "route", routeRef).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name", "action", "command_id", "command_status", "last_error", "metadata", "created_at", "updated_at", "completed_at",
		}).
			AddRow(aclDeliveryID, tenantID, nodeID, "acl", aclRef, "allow-vpn-icmp", "create", aclCommandID, "completed", "", []byte(`{"kind":"acl"}`), now, now, now).
			AddRow(routeDeliveryID, tenantID, nodeID, "route", routeRef, "route-10-20", "update", routeCommandID, "sent", "", []byte(`{"kind":"route"}`), now, now, nil))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	body := []byte(`{"items":[{"node_id":"11111111-1111-1111-1111-111111111111","policy_domain":"acl","policy_ref":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},{"node_id":"11111111-1111-1111-1111-111111111111","policy_domain":"route","policy_ref":"10.20.0.0/16"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/policy-deliveries/status", bytes.NewReader(body))
	req = withAuthContext(req, "super_admin", tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 status items, got %d", len(items))
	}
	aclItem := items[0].(map[string]interface{})
	if aclItem["policy_status"] != "applied" {
		t.Fatalf("expected acl policy_status applied, got %#v", aclItem["policy_status"])
	}
	if aclItem["pending_cmds"] != float64(0) {
		t.Fatalf("expected acl pending_cmds 0, got %#v", aclItem["pending_cmds"])
	}
	if _, ok := aclItem["last_delivery"].(map[string]interface{}); !ok {
		t.Fatalf("expected acl last_delivery map, got %#v", aclItem["last_delivery"])
	}
	if history := aclItem["delivery_history"].([]interface{}); len(history) != 1 {
		t.Fatalf("expected acl delivery_history length 1, got %d", len(history))
	}
	routeItem := items[1].(map[string]interface{})
	if routeItem["policy_status"] != "in_progress" {
		t.Fatalf("expected route policy_status in_progress, got %#v", routeItem["policy_status"])
	}
	if routeItem["pending_cmds"] != float64(1) {
		t.Fatalf("expected route pending_cmds 1, got %#v", routeItem["pending_cmds"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodeStatusEndpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	publicKey := "node-public-key"
	commandID := uuid.New().String()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset", "last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token", "created_at", "updated_at",
		}).AddRow(nodeID, publicKey, "machine-1", tenantID, "203.0.113.2:51820", "", "203.0.113.2", "beijing", "", "node-1", "100.64.0.2", 2, now.Unix(), now.Unix(), "edge", "ebpf", "6.8.0", true, "online", 0, "{10.20.0.0/16}", "", now, now))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE policy_deliveries
		SET command_status = $2,
		    last_error = $3,
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE command_id IN (
			SELECT id
			FROM agent_commands
			WHERE node_public_key = $1
			  AND status IN ($4, $5, $6)
			  AND deadline_at IS NOT NULL
			  AND deadline_at < NOW()
		)
	`)).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusFailed, "command timed out waiting for agent result", controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE agent_commands
		SET status = $2,
		    message = $3,
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE node_public_key = $1
		  AND status IN ($4, $5, $6)
		  AND deadline_at IS NOT NULL
		  AND deadline_at < NOW()
	`)).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusFailed, "command timed out waiting for agent result", controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(*)
		FROM agent_commands
		WHERE node_public_key = $1
		  AND status IN ($2, $3, $4)
	`)).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1
		ORDER BY created_at DESC
		LIMIT 1
	`)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "node_public_key", "command", "params", "status", "message", "priority", "timeout_seconds", "created_at", "updated_at", "sent_at", "acknowledged_at", "completed_at", "result",
		}).AddRow(commandID, publicKey, "sync", []byte(`{"source":"test"}`), "sent", "", 10, 30, now, now, now, nil, nil, []byte(`{}`)))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT tenant_id, node_id, COALESCE(desired_state_version, ''), desired_state_metadata, desired_state_updated_at,
		       COALESCE(applied_state_version, ''), applied_state_updated_at, COALESCE(observed_state, ''),
		       COALESCE(observed_message, ''), observed_at, last_sync_at, COALESCE(last_sync_error, ''),
		       created_at, updated_at
		FROM node_control_states
		WHERE tenant_id = $1 AND node_id = $2
	`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at", "applied_state_version", "applied_state_updated_at", "observed_state", "observed_message", "observed_at", "last_sync_at", "last_sync_error", "created_at", "updated_at",
		}).AddRow(tenantID, nodeID, "dsv-2", []byte(`{}`), now, "dsv-1", now.Add(-time.Minute), "syncing", "applying policy", now, now, "", now, now))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	body := []byte(`{"node_ids":["11111111-1111-1111-1111-111111111111"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/nodes/status", bytes.NewReader(body))
	req = withAuthContext(req, "super_admin", tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 node item, got %d", len(items))
	}
	item := items[0].(map[string]interface{})
	if item["node_id"] != nodeID.String() {
		t.Fatalf("expected node_id %s, got %#v", nodeID, item["node_id"])
	}
	if item["pending_cmds"] != float64(1) {
		t.Fatalf("expected pending_cmds 1, got %#v", item["pending_cmds"])
	}
	if item["configuration_status"] != "in_progress" {
		t.Fatalf("expected configuration_status in_progress, got %#v", item["configuration_status"])
	}
	if item["desired_state_version"] != "dsv-2" || item["applied_state_version"] != "dsv-1" {
		t.Fatalf("unexpected desired/applied versions: %#v / %#v", item["desired_state_version"], item["applied_state_version"])
	}
	if item["last_command_status"] != "sent" {
		t.Fatalf("expected last_command_status sent, got %#v", item["last_command_status"])
	}
	if _, exists := item["convergence_status"]; !exists {
		t.Fatalf("expected convergence_status in response: %#v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
