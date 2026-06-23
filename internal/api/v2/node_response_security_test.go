package v2

import (
	"database/sql"
	"testing"

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
