package controllerstorage

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

type PolicySyncRequest struct {
	TenantID            uuid.UUID
	NodeID              uuid.UUID
	NodePublicKey       string
	Domain              string
	Action              string
	PolicyRef           string
	PolicyName          string
	DesiredStateVersion string
	DesiredMetadata     map[string]interface{}
	CommandParams       map[string]interface{}
	DeliveryMetadata    map[string]interface{}
	Priority            int
	TimeoutSeconds      int
}

type PolicySyncResult struct {
	DesiredStateVersion string
	ControlState        *NodeControlState
	Command             *AgentCommand
	Delivery            *PolicyDelivery
}

func (s *Storage) QueuePolicySync(req PolicySyncRequest) (*PolicySyncResult, error) {
	req.NodePublicKey = strings.TrimSpace(req.NodePublicKey)
	req.Domain = strings.TrimSpace(req.Domain)
	req.Action = strings.TrimSpace(req.Action)
	req.PolicyRef = strings.TrimSpace(req.PolicyRef)
	req.PolicyName = strings.TrimSpace(req.PolicyName)
	req.DesiredStateVersion = strings.TrimSpace(req.DesiredStateVersion)

	if req.TenantID == uuid.Nil {
		return nil, errors.New("tenant id is required")
	}
	if req.NodeID == uuid.Nil {
		return nil, errors.New("node id is required")
	}
	if req.NodePublicKey == "" {
		return nil, errors.New("node public key is required")
	}
	if req.Domain == "" {
		return nil, errors.New("policy domain is required")
	}
	if req.Action == "" {
		return nil, errors.New("policy action is required")
	}
	if req.DesiredStateVersion == "" {
		req.DesiredStateVersion = NewDesiredStateVersion()
	}
	if req.DesiredMetadata == nil {
		req.DesiredMetadata = map[string]interface{}{}
	}
	if req.CommandParams == nil {
		req.CommandParams = map[string]interface{}{}
	}
	if req.DeliveryMetadata == nil {
		req.DeliveryMetadata = map[string]interface{}{}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	state, err := s.upsertNodeDesiredState(tx, req.TenantID, req.NodeID, req.DesiredStateVersion, req.DesiredMetadata)
	if err != nil {
		return nil, err
	}

	cmd, err := queueAgentCommand(tx, req.NodePublicKey, "sync", req.CommandParams, req.Priority, req.TimeoutSeconds)
	if err != nil {
		return nil, err
	}

	var delivery *PolicyDelivery
	if req.PolicyRef != "" {
		delivery, err = createPolicyDelivery(tx, &PolicyDelivery{
			TenantID:      req.TenantID,
			NodeID:        req.NodeID,
			PolicyDomain:  req.Domain,
			PolicyRef:     req.PolicyRef,
			PolicyName:    req.PolicyName,
			Action:        req.Action,
			CommandID:     cmd.ID,
			CommandStatus: cmd.Status,
			Metadata:      req.DeliveryMetadata,
		})
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	return &PolicySyncResult{
		DesiredStateVersion: req.DesiredStateVersion,
		ControlState:        state,
		Command:             cmd,
		Delivery:            delivery,
	}, nil
}
