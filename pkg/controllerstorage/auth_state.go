package controllerstorage

import (
	"database/sql"
	"strings"

	"github.com/google/uuid"
)

type NodeRuntimeAuthState struct {
	NodeID              uuid.UUID
	TenantID            uuid.UUID
	NodeStatus          string
	RuntimeTokenVersion int
	TenantStatus        string
}

func (s *Storage) GetUserTokenVersion(userID string) (int, error) {
	parsedUserID, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return 0, err
	}

	var tokenVersion int
	err = s.db.QueryRow(`SELECT COALESCE(token_version, 0) FROM users WHERE id = $1`, parsedUserID).Scan(&tokenVersion)
	if err != nil {
		return 0, err
	}
	return tokenVersion, nil
}

func (s *Storage) GetNodeRuntimeTokenVersion(nodeID uuid.UUID) (int, error) {
	var tokenVersion int
	err := s.db.QueryRow(`SELECT COALESCE(runtime_token_version, 0) FROM nodes WHERE id = $1`, nodeID).Scan(&tokenVersion)
	if err != nil {
		return 0, err
	}
	return tokenVersion, nil
}

func (s *Storage) GetNodeRuntimeAuthState(nodeID uuid.UUID) (*NodeRuntimeAuthState, error) {
	state := &NodeRuntimeAuthState{}
	err := s.db.QueryRow(`
		SELECT n.id, n.tenant_id, COALESCE(n.status, 'online'), COALESCE(n.runtime_token_version, 0), COALESCE(t.status, '')
		FROM nodes n
		JOIN tenants t ON t.id = n.tenant_id
		WHERE n.id = $1
	`, nodeID).Scan(
		&state.NodeID,
		&state.TenantID,
		&state.NodeStatus,
		&state.RuntimeTokenVersion,
		&state.TenantStatus,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state.NodeStatus = strings.TrimSpace(state.NodeStatus)
	state.TenantStatus = strings.TrimSpace(state.TenantStatus)
	return state, nil
}
