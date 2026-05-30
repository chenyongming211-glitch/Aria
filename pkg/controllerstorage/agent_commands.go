package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

const (
	AgentCommandStatusPending      = "pending"
	AgentCommandStatusSent         = "sent"
	AgentCommandStatusAcknowledged = "acknowledged"
	AgentCommandStatusCompleted    = "completed"
	AgentCommandStatusFailed       = "failed"
)

type AgentCommand struct {
	ID             string                 `json:"id"`
	NodePublicKey  string                 `json:"node_public_key"`
	Command        string                 `json:"command"`
	RawParams      map[string]interface{} `json:"params,omitempty"`
	Params         map[string]string      `json:"-"`
	Status         string                 `json:"status"`
	Message        string                 `json:"message,omitempty"`
	Priority       int                    `json:"priority"`
	TimeoutSeconds int                    `json:"timeout_seconds"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	SentAt         *time.Time             `json:"sent_at,omitempty"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at,omitempty"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	Result         map[string]string      `json:"result,omitempty"`
}

func (s *Storage) QueueAgentCommand(nodePublicKey, command string, params map[string]interface{}, priority, timeoutSeconds int) (*AgentCommand, error) {
	if nodePublicKey == "" {
		return nil, errors.New("node public key is required")
	}
	if command == "" {
		return nil, errors.New("command is required")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	if params == nil {
		params = map[string]interface{}{}
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	cmd := &AgentCommand{
		NodePublicKey:  nodePublicKey,
		Command:        command,
		RawParams:      params,
		Params:         stringifyCommandParams(params),
		Status:         AgentCommandStatusPending,
		Priority:       priority,
		TimeoutSeconds: timeoutSeconds,
	}

	query := `
		INSERT INTO agent_commands (node_public_key, command, params, status, priority, timeout_seconds)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	if err := s.db.QueryRow(query,
		nodePublicKey,
		command,
		paramsJSON,
		AgentCommandStatusPending,
		priority,
		timeoutSeconds,
	).Scan(&cmd.ID, &cmd.CreatedAt, &cmd.UpdatedAt); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (s *Storage) GetNextPendingAgentCommand(nodePublicKey string) (*AgentCommand, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
		SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1 AND status = $2
		ORDER BY priority DESC, created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, nodePublicKey, AgentCommandStatusPending)

	cmd, err := scanAgentCommandRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	now := time.Now()
	if _, err := tx.Exec(`
		UPDATE agent_commands
		SET status = $2, sent_at = $3, updated_at = $3
		WHERE id = $1
	`, cmd.ID, AgentCommandStatusSent, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	cmd.Status = AgentCommandStatusSent
	cmd.SentAt = &now
	cmd.UpdatedAt = now
	return cmd, nil
}

func (s *Storage) UpdateAgentCommandStatus(commandID, status, message string, result map[string]string) error {
	if commandID == "" {
		return errors.New("command id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}

	if result == nil {
		result = map[string]string{}
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}

	query := `
		UPDATE agent_commands
		SET status = $2::varchar,
		    message = $3,
		    result = $4,
		    updated_at = NOW(),
		    acknowledged_at = CASE
		        WHEN $2::varchar = 'acknowledged' AND acknowledged_at IS NULL THEN NOW()
		        ELSE acknowledged_at
		    END,
		    completed_at = CASE
		        WHEN $2::varchar IN ('completed', 'failed') THEN NOW()
		        ELSE completed_at
		    END
		WHERE id = $1
	`

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(query, commandID, status, message, resultJSON); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		UPDATE policy_deliveries
		SET command_status = $2::varchar,
		    last_error = CASE
		        WHEN $2::varchar = 'failed' THEN $3
		        WHEN $2::varchar = 'completed' THEN ''
		        ELSE last_error
		    END,
		    updated_at = NOW(),
		    completed_at = CASE
		        WHEN $2::varchar IN ('completed', 'failed') THEN NOW()
		        ELSE completed_at
		    END
		WHERE command_id = $1
	`, commandID, status, message); err != nil {
		return err
	}

	if err := s.syncNodeControlStateForCommandTx(tx, commandID, status, message, result); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Non-blocking alert/audit integration for terminal statuses
	if status == AgentCommandStatusCompleted || status == AgentCommandStatusFailed {
		s.emitCommandAlertAndAudit(commandID, status, message)
	}

	return nil
}

// emitCommandAlertAndAudit generates alerts and audit events after a command reaches a terminal status.
// All writes are non-blocking: errors are logged but never returned.
func (s *Storage) emitCommandAlertAndAudit(commandID, status, errorMsg string) {
	// Fetch node info (tenant_id, node_id) and command type via JOIN
	var (
		tenantID    uuid.UUID
		nodeID      uuid.UUID
		commandType string
	)
	err := s.db.QueryRow(`
		SELECT n.tenant_id, n.id, ac.command
		FROM agent_commands ac
		JOIN nodes n ON n.public_key = ac.node_public_key
		WHERE ac.id = $1
	`, commandID).Scan(&tenantID, &nodeID, &commandType)
	if err != nil {
		log.Printf("[agent_commands] failed to fetch node info for command %s alert/audit: %v", commandID, err)
		return
	}

	// Command-level alerts and audit events
	switch status {
	case AgentCommandStatusFailed:
		if err := s.GenerateSyncFailedAlert(tenantID, nodeID, commandID, errorMsg); err != nil {
			log.Printf("[agent_commands] failed to generate sync_failed alert for command %s: %v", commandID, err)
		}
		if _, err := s.CreateAuditEvent(&AuditEvent{
			TenantID:  tenantID,
			NodeID:    &nodeID,
			EventType: "command_failed",
			Actor:     "system",
			Summary:   fmt.Sprintf("命令执行失败: %s", commandType),
			Detail:    map[string]interface{}{"command_id": commandID, "error": errorMsg},
		}); err != nil {
			log.Printf("[agent_commands] failed to create command_failed audit event for command %s: %v", commandID, err)
		}

	case AgentCommandStatusCompleted:
		if _, err := s.CreateAuditEvent(&AuditEvent{
			TenantID:  tenantID,
			NodeID:    &nodeID,
			EventType: "command_completed",
			Actor:     "system",
			Summary:   fmt.Sprintf("命令执行成功: %s", commandType),
			Detail:    map[string]interface{}{"command_id": commandID},
		}); err != nil {
			log.Printf("[agent_commands] failed to create command_completed audit event for command %s: %v", commandID, err)
		}
	}

	// PolicyDelivery cascade alerts and audit events
	s.emitPolicyDeliveryAlertAndAudit(commandID, status, errorMsg, tenantID, nodeID)
}

// emitPolicyDeliveryAlertAndAudit generates alerts and audit events for policy deliveries
// affected by a command status change. All writes are non-blocking.
func (s *Storage) emitPolicyDeliveryAlertAndAudit(commandID, status, errorMsg string, tenantID, nodeID uuid.UUID) {
	rows, err := s.db.Query(`
		SELECT policy_domain, policy_ref
		FROM policy_deliveries
		WHERE command_id = $1
	`, commandID)
	if err != nil {
		log.Printf("[agent_commands] failed to query policy deliveries for command %s: %v", commandID, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var domain, ref string
		if err := rows.Scan(&domain, &ref); err != nil {
			log.Printf("[agent_commands] failed to scan policy delivery for command %s: %v", commandID, err)
			continue
		}

		switch status {
		case AgentCommandStatusFailed:
			if err := s.GeneratePolicyFailedAlert(tenantID, nodeID, domain, ref, errorMsg); err != nil {
				log.Printf("[agent_commands] failed to generate policy_failed alert for command %s domain %s: %v", commandID, domain, err)
			}
			if _, err := s.CreateAuditEvent(&AuditEvent{
				TenantID:  tenantID,
				NodeID:    &nodeID,
				EventType: "policy_failed",
				Actor:     "system",
				Summary:   fmt.Sprintf("策略下发失败: %s/%s", domain, ref),
				Detail:    map[string]interface{}{"command_id": commandID, "policy_domain": domain, "policy_ref": ref, "error": errorMsg},
			}); err != nil {
				log.Printf("[agent_commands] failed to create policy_failed audit event for command %s: %v", commandID, err)
			}

		case AgentCommandStatusCompleted:
			if _, err := s.CreateAuditEvent(&AuditEvent{
				TenantID:  tenantID,
				NodeID:    &nodeID,
				EventType: "policy_delivered",
				Actor:     "system",
				Summary:   fmt.Sprintf("策略下发成功: %s/%s", domain, ref),
				Detail:    map[string]interface{}{"command_id": commandID, "policy_domain": domain, "policy_ref": ref},
			}); err != nil {
				log.Printf("[agent_commands] failed to create policy_delivered audit event for command %s: %v", commandID, err)
			}
		}
	}

	if err := rows.Err(); err != nil {
		log.Printf("[agent_commands] error iterating policy deliveries for command %s: %v", commandID, err)
	}
}

func (s *Storage) CountIncompleteAgentCommands(nodePublicKey string) (int, error) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM agent_commands
		WHERE node_public_key = $1
		  AND status IN ($2, $3, $4)
	`
	err := s.db.QueryRow(query,
		nodePublicKey,
		AgentCommandStatusPending,
		AgentCommandStatusSent,
		AgentCommandStatusAcknowledged,
	).Scan(&count)
	return count, err
}

func (s *Storage) GetLastAgentCommandID(nodePublicKey string) (string, error) {
	var commandID string
	err := s.db.QueryRow(`
		SELECT id
		FROM agent_commands
		WHERE node_public_key = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, nodePublicKey).Scan(&commandID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return commandID, err
}

func (s *Storage) GetLastAgentCommand(nodePublicKey string) (*AgentCommand, error) {
	row := s.db.QueryRow(`
		SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, nodePublicKey)

	cmd, err := scanAgentCommandRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return cmd, nil
}

func (s *Storage) ListRecentAgentCommands(nodePublicKey string, limit int) ([]*AgentCommand, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.Query(`
		SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, nodePublicKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	commands := make([]*AgentCommand, 0, limit)
	for rows.Next() {
		cmd, err := scanAgentCommandRow(rows)
		if err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return commands, nil
}

func scanAgentCommandRow(row interface {
	Scan(dest ...interface{}) error
}) (*AgentCommand, error) {
	var (
		cmd            AgentCommand
		paramsJSON     []byte
		resultJSON     []byte
		message        string
		sentAt         sql.NullTime
		acknowledgedAt sql.NullTime
		completedAt    sql.NullTime
	)

	err := row.Scan(
		&cmd.ID,
		&cmd.NodePublicKey,
		&cmd.Command,
		&paramsJSON,
		&cmd.Status,
		&message,
		&cmd.Priority,
		&cmd.TimeoutSeconds,
		&cmd.CreatedAt,
		&cmd.UpdatedAt,
		&sentAt,
		&acknowledgedAt,
		&completedAt,
		&resultJSON,
	)
	if err != nil {
		return nil, err
	}

	cmd.Message = message
	cmd.RawParams = map[string]interface{}{}
	if len(paramsJSON) > 0 {
		if err := json.Unmarshal(paramsJSON, &cmd.RawParams); err != nil {
			return nil, err
		}
	}
	cmd.Params = stringifyCommandParams(cmd.RawParams)

	cmd.Result = map[string]string{}
	if len(resultJSON) > 0 {
		if err := json.Unmarshal(resultJSON, &cmd.Result); err != nil {
			return nil, err
		}
	}

	if sentAt.Valid {
		cmd.SentAt = &sentAt.Time
	}
	if acknowledgedAt.Valid {
		cmd.AcknowledgedAt = &acknowledgedAt.Time
	}
	if completedAt.Valid {
		cmd.CompletedAt = &completedAt.Time
	}

	return &cmd, nil
}

func stringifyCommandParams(params map[string]interface{}) map[string]string {
	if len(params) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(params))
	for key, value := range params {
		switch v := value.(type) {
		case nil:
			out[key] = ""
		case string:
			out[key] = v
		case bool:
			if v {
				out[key] = "true"
			} else {
				out[key] = "false"
			}
		default:
			bytes, err := json.Marshal(v)
			if err != nil {
				out[key] = ""
			} else {
				out[key] = string(bytes)
			}
		}
	}

	return out
}
