package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
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
		SET status = $2,
		    message = $3,
		    result = $4,
		    updated_at = NOW(),
		    acknowledged_at = CASE
		        WHEN $2 = 'acknowledged' AND acknowledged_at IS NULL THEN NOW()
		        ELSE acknowledged_at
		    END,
		    completed_at = CASE
		        WHEN $2 IN ('completed', 'failed') THEN NOW()
		        ELSE completed_at
		    END
		WHERE id = $1
	`

	_, err = s.db.Exec(query, commandID, status, message, resultJSON)
	return err
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
