package v2

import (
	"database/sql"
	"testing"
	"time"

	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestBuildTenantNodeResponseDoesNotExposeEnrollmentToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	nodeID := uuid.New()
	tenantID := uuid.New()
	node := &controllerstorage.Node{
		ID:                nodeID,
		TenantID:          tenantID,
		PublicKey:         "node-public-key",
		Hostname:          "node-1",
		Status:            "online",
		EnrolledWithToken: "tk_sensitive_enrollment_token",
	}
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM agent_commands.*WHERE node_public_key = \$1.*status IN`).
		WithArgs(
			node.PublicKey,
			controllerstorage.AgentCommandStatusPending,
			controllerstorage.AgentCommandStatusSent,
			controllerstorage.AgentCommandStatusAcknowledged,
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s)SELECT id, node_public_key, command, params, status, COALESCE\(message, ''\).*FROM agent_commands.*WHERE node_public_key = \$1.*ORDER BY created_at DESC`).
		WithArgs(node.PublicKey).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`(?s)SELECT tenant_id, node_id, COALESCE\(desired_state_version, ''\).*FROM node_control_states.*WHERE tenant_id = \$1 AND node_id = \$2`).
		WithArgs(tenantID, nodeID).
		WillReturnError(sql.ErrNoRows)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}

	response := router.buildTenantNodeResponse(node)

	if _, exists := response["enrolled_with_token"]; exists {
		t.Fatal("tenant node response must not expose enrollment token")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestBuildTenantNodeResponseIncludesLocationFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	nodeID := uuid.New()
	tenantID := uuid.New()
	node := &controllerstorage.Node{
		ID:        nodeID,
		TenantID:  tenantID,
		PublicKey: "node-public-key",
		Hostname:  "node-1",
		Region:    "tencent-cloud",
		VPCID:     "vpc-prod-a",
		Status:    "online",
	}
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM agent_commands.*WHERE node_public_key = \$1.*status IN`).
		WithArgs(
			node.PublicKey,
			controllerstorage.AgentCommandStatusPending,
			controllerstorage.AgentCommandStatusSent,
			controllerstorage.AgentCommandStatusAcknowledged,
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s)SELECT id, node_public_key, command, params, status, COALESCE\(message, ''\).*FROM agent_commands.*WHERE node_public_key = \$1.*ORDER BY created_at DESC`).
		WithArgs(node.PublicKey).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`(?s)SELECT tenant_id, node_id, COALESCE\(desired_state_version, ''\).*FROM node_control_states.*WHERE tenant_id = \$1 AND node_id = \$2`).
		WithArgs(tenantID, nodeID).
		WillReturnError(sql.ErrNoRows)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}

	response := router.buildTenantNodeResponse(node)

	if got := response["region"]; got != node.Region {
		t.Fatalf("expected region %q, got %#v", node.Region, got)
	}
	if got := response["vpc_id"]; got != node.VPCID {
		t.Fatalf("expected vpc_id %q, got %#v", node.VPCID, got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodeOnboardingStatusPhases(t *testing.T) {
	now := time.Now().UTC()
	node := &controllerstorage.Node{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		PublicKey:         "node-public-key",
		Hostname:          "node-1",
		Status:            "online",
		LastSeen:          now.Unix(),
		RegisteredAt:      now.Unix(),
		EnrolledWithToken: "tk_sensitive_enrollment_token",
	}

	tests := []struct {
		name    string
		node    *controllerstorage.Node
		summary map[string]interface{}
		phase   string
	}{
		{
			name:    "registered online but no sync evidence",
			node:    node,
			summary: map[string]interface{}{},
			phase:   "syncing",
		},
		{
			name: "desired applied mismatch",
			node: node,
			summary: map[string]interface{}{
				"desired_state_version": "dsv-new",
				"applied_state_version": "dsv-old",
			},
			phase: "syncing",
		},
		{
			name: "online and applied",
			node: node,
			summary: map[string]interface{}{
				"desired_state_version": "dsv-new",
				"applied_state_version": "dsv-new",
				"last_sync_at":          now,
			},
			phase: "online",
		},
		{
			name: "applied observed message is not an error",
			node: node,
			summary: map[string]interface{}{
				"desired_state_version": "dsv-new",
				"applied_state_version": "dsv-new",
				"observed_state":        "applied",
				"observed_message":      "sync applied successfully",
				"configuration_status":  "applied",
				"last_sync_at":          now,
			},
			phase: "online",
		},
		{
			name: "sync error",
			node: node,
			summary: map[string]interface{}{
				"last_sync_error": "sync apply failed",
			},
			phase: "degraded",
		},
		{
			name: "failed observed message is an error",
			node: node,
			summary: map[string]interface{}{
				"observed_state":   "failed",
				"observed_message": "sync apply failed",
			},
			phase: "degraded",
		},
		{
			name: "offline no sync",
			node: &controllerstorage.Node{
				ID:                node.ID,
				TenantID:          node.TenantID,
				PublicKey:         node.PublicKey,
				Hostname:          node.Hostname,
				Status:            "offline",
				RegisteredAt:      node.RegisteredAt,
				EnrolledWithToken: node.EnrolledWithToken,
			},
			summary: map[string]interface{}{},
			phase:   "registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			onboarding := buildNodeOnboardingStatus(tt.node, tt.summary)
			if onboarding["phase"] != tt.phase {
				t.Fatalf("expected phase %q, got %#v", tt.phase, onboarding["phase"])
			}
			if onboarding["token_preview"] != "tk_sen...oken" {
				t.Fatalf("expected redacted token preview, got %#v", onboarding["token_preview"])
			}
			if _, leaked := onboarding["enrolled_with_token"]; leaked {
				t.Fatal("onboarding status must not expose raw enrollment token")
			}
			if tt.name == "applied observed message is not an error" && onboarding["last_error"] != "" {
				t.Fatalf("expected no last_error for successful observed message, got %#v", onboarding["last_error"])
			}
			if tt.name == "failed observed message is an error" && onboarding["last_error"] != "sync apply failed" {
				t.Fatalf("expected failed observed message as last_error, got %#v", onboarding["last_error"])
			}
		})
	}
}
