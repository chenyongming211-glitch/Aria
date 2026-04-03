package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type NodeControlState struct {
	TenantID              uuid.UUID              `json:"tenant_id"`
	NodeID                uuid.UUID              `json:"node_id"`
	DesiredStateVersion   string                 `json:"desired_state_version"`
	DesiredStateMetadata  map[string]interface{} `json:"desired_state_metadata,omitempty"`
	DesiredStateUpdatedAt *time.Time             `json:"desired_state_updated_at,omitempty"`
	AppliedStateVersion   string                 `json:"applied_state_version,omitempty"`
	AppliedStateUpdatedAt *time.Time             `json:"applied_state_updated_at,omitempty"`
	ObservedState         string                 `json:"observed_state,omitempty"`
	ObservedMessage       string                 `json:"observed_message,omitempty"`
	ObservedAt            *time.Time             `json:"observed_at,omitempty"`
	LastSyncAt            *time.Time             `json:"last_sync_at,omitempty"`
	LastSyncError         string                 `json:"last_sync_error,omitempty"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

type NodeControlStateReport struct {
	AppliedStateVersion string
	ObservedState       string
	ObservedMessage     string
	LastSyncAt          *time.Time
	LastSyncError       string
}

func NewDesiredStateVersion() string {
	return fmt.Sprintf("dsv-%d-%s", time.Now().Unix(), uuid.NewString()[:8])
}

func (s *Storage) GetNodeControlState(tenantID, nodeID uuid.UUID) (*NodeControlState, error) {
	row := s.db.QueryRow(`
		SELECT tenant_id, node_id, COALESCE(desired_state_version, ''), desired_state_metadata, desired_state_updated_at,
		       COALESCE(applied_state_version, ''), applied_state_updated_at, COALESCE(observed_state, ''),
		       COALESCE(observed_message, ''), observed_at, last_sync_at, COALESCE(last_sync_error, ''),
		       created_at, updated_at
		FROM node_control_states
		WHERE tenant_id = $1 AND node_id = $2
	`, tenantID, nodeID)

	state, err := scanNodeControlState(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return state, err
}

func (s *Storage) UpsertNodeDesiredState(tenantID, nodeID uuid.UUID, version string, metadata map[string]interface{}) (*NodeControlState, error) {
	return s.upsertNodeDesiredState(txQueryRowAdapter{s.db}, tenantID, nodeID, version, metadata)
}

func (s *Storage) ReportNodeControlState(tenantID, nodeID uuid.UUID, report NodeControlStateReport) (*NodeControlState, error) {
	return s.reportNodeControlState(txQueryRowAdapter{s.db}, tenantID, nodeID, report)
}

func (s *Storage) upsertNodeDesiredState(
	q queryRower,
	tenantID, nodeID uuid.UUID,
	version string,
	metadata map[string]interface{},
) (*NodeControlState, error) {
	if strings.TrimSpace(version) == "" {
		version = NewDesiredStateVersion()
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	row := q.QueryRow(`
		INSERT INTO node_control_states (
			tenant_id,
			node_id,
			desired_state_version,
			desired_state_metadata,
			desired_state_updated_at,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, NOW(), NOW(), NOW())
		ON CONFLICT (node_id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			desired_state_version = EXCLUDED.desired_state_version,
			desired_state_metadata = EXCLUDED.desired_state_metadata,
			desired_state_updated_at = NOW(),
			updated_at = NOW()
		RETURNING tenant_id, node_id, COALESCE(desired_state_version, ''), desired_state_metadata, desired_state_updated_at,
		          COALESCE(applied_state_version, ''), applied_state_updated_at, COALESCE(observed_state, ''),
		          COALESCE(observed_message, ''), observed_at, last_sync_at, COALESCE(last_sync_error, ''),
		          created_at, updated_at
	`, tenantID, nodeID, version, metadataJSON)

	return scanNodeControlState(row)
}

func (s *Storage) reportNodeControlState(
	q queryRower,
	tenantID, nodeID uuid.UUID,
	report NodeControlStateReport,
) (*NodeControlState, error) {
	row := q.QueryRow(`
		INSERT INTO node_control_states (
			tenant_id,
			node_id,
			applied_state_version,
			applied_state_updated_at,
			observed_state,
			observed_message,
			observed_at,
			last_sync_at,
			last_sync_error,
			created_at,
			updated_at
		) VALUES (
			$1,
			$2,
			NULLIF($3, ''),
			CASE WHEN NULLIF($3, '') IS NOT NULL THEN NOW() ELSE NULL END,
			NULLIF($4, ''),
			NULLIF($5, ''),
			CASE WHEN NULLIF($4, '') IS NOT NULL OR NULLIF($5, '') IS NOT NULL THEN NOW() ELSE NULL END,
			$6,
			NULLIF($7, ''),
			NOW(),
			NOW()
		)
		ON CONFLICT (node_id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			applied_state_version = COALESCE(NULLIF(EXCLUDED.applied_state_version, ''), node_control_states.applied_state_version),
			applied_state_updated_at = CASE
				WHEN NULLIF(EXCLUDED.applied_state_version, '') IS NOT NULL THEN NOW()
				ELSE node_control_states.applied_state_updated_at
			END,
			observed_state = COALESCE(NULLIF(EXCLUDED.observed_state, ''), node_control_states.observed_state),
			observed_message = CASE
				WHEN NULLIF(EXCLUDED.observed_message, '') IS NOT NULL THEN EXCLUDED.observed_message
				WHEN EXCLUDED.last_sync_at IS NOT NULL AND NULLIF(EXCLUDED.last_sync_error, '') IS NULL THEN ''
				ELSE node_control_states.observed_message
			END,
			observed_at = CASE
				WHEN NULLIF(EXCLUDED.observed_state, '') IS NOT NULL OR NULLIF(EXCLUDED.observed_message, '') IS NOT NULL OR EXCLUDED.last_sync_at IS NOT NULL THEN NOW()
				ELSE node_control_states.observed_at
			END,
			last_sync_at = COALESCE(EXCLUDED.last_sync_at, node_control_states.last_sync_at),
			last_sync_error = CASE
				WHEN EXCLUDED.last_sync_at IS NOT NULL OR NULLIF(EXCLUDED.last_sync_error, '') IS NOT NULL THEN COALESCE(NULLIF(EXCLUDED.last_sync_error, ''), '')
				ELSE node_control_states.last_sync_error
			END,
			updated_at = NOW()
		RETURNING tenant_id, node_id, COALESCE(desired_state_version, ''), desired_state_metadata, desired_state_updated_at,
		          COALESCE(applied_state_version, ''), applied_state_updated_at, COALESCE(observed_state, ''),
		          COALESCE(observed_message, ''), observed_at, last_sync_at, COALESCE(last_sync_error, ''),
		          created_at, updated_at
	`, tenantID, nodeID, report.AppliedStateVersion, report.ObservedState, report.ObservedMessage, report.LastSyncAt, report.LastSyncError)

	return scanNodeControlState(row)
}

func (s *Storage) syncNodeControlStateForCommandTx(
	tx *sql.Tx,
	commandID, status, message string,
	result map[string]string,
) error {
	row := tx.QueryRow(`
		SELECT n.tenant_id, n.id, ac.command, ac.params
		FROM agent_commands ac
		JOIN nodes n ON n.public_key = ac.node_public_key
		WHERE ac.id = $1
	`, commandID)

	var (
		tenantID   uuid.UUID
		nodeID     uuid.UUID
		command    string
		paramsJSON []byte
	)
	if err := row.Scan(&tenantID, &nodeID, &command, &paramsJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	params := map[string]interface{}{}
	if len(paramsJSON) > 0 {
		if err := json.Unmarshal(paramsJSON, &params); err != nil {
			return err
		}
	}

	desiredVersion := strings.TrimSpace(firstString(result["desired_state_version"], stringValue(params["desired_state_version"])))
	appliedVersion := strings.TrimSpace(result["applied_state_version"])
	observedState := strings.TrimSpace(result["observed_state"])
	observedMessage := strings.TrimSpace(result["observed_message"])

	switch status {
	case AgentCommandStatusCompleted:
		if command == "sync" && appliedVersion == "" {
			appliedVersion = desiredVersion
		}
		if observedState == "" {
			if command == "sync" && appliedVersion != "" {
				observedState = "applied"
			} else {
				observedState = "healthy"
			}
		}
	case AgentCommandStatusFailed:
		if observedState == "" {
			observedState = "error"
		}
	case AgentCommandStatusAcknowledged, AgentCommandStatusSent:
		if observedState == "" {
			observedState = "in_progress"
		}
	}

	if observedMessage == "" {
		observedMessage = strings.TrimSpace(message)
	}

	now := time.Now()
	lastSyncError := ""
	if observedState == "error" || status == AgentCommandStatusFailed {
		lastSyncError = firstString(observedMessage, strings.TrimSpace(message))
	}

	_, err := s.reportNodeControlState(tx, tenantID, nodeID, NodeControlStateReport{
		AppliedStateVersion: appliedVersion,
		ObservedState:       observedState,
		ObservedMessage:     observedMessage,
		LastSyncAt:          &now,
		LastSyncError:       lastSyncError,
	})
	return err
}

func scanNodeControlState(row interface {
	Scan(dest ...interface{}) error
}) (*NodeControlState, error) {
	var (
		state            NodeControlState
		rawMetadata      []byte
		desiredUpdatedAt sql.NullTime
		appliedUpdatedAt sql.NullTime
		observedAt       sql.NullTime
		lastSyncAt       sql.NullTime
	)

	if err := row.Scan(
		&state.TenantID,
		&state.NodeID,
		&state.DesiredStateVersion,
		&rawMetadata,
		&desiredUpdatedAt,
		&state.AppliedStateVersion,
		&appliedUpdatedAt,
		&state.ObservedState,
		&state.ObservedMessage,
		&observedAt,
		&lastSyncAt,
		&state.LastSyncError,
		&state.CreatedAt,
		&state.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if len(rawMetadata) > 0 {
		if err := json.Unmarshal(rawMetadata, &state.DesiredStateMetadata); err != nil {
			return nil, err
		}
	}
	if state.DesiredStateMetadata == nil {
		state.DesiredStateMetadata = map[string]interface{}{}
	}

	if desiredUpdatedAt.Valid {
		state.DesiredStateUpdatedAt = &desiredUpdatedAt.Time
	}
	if appliedUpdatedAt.Valid {
		state.AppliedStateUpdatedAt = &appliedUpdatedAt.Time
	}
	if observedAt.Valid {
		state.ObservedAt = &observedAt.Time
	}
	if lastSyncAt.Valid {
		state.LastSyncAt = &lastSyncAt.Time
	}

	return &state, nil
}

type queryRower interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

type txQueryRowAdapter struct {
	db *sql.DB
}

func (a txQueryRowAdapter) QueryRow(query string, args ...interface{}) *sql.Row {
	return a.db.QueryRow(query, args...)
}

func firstString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
