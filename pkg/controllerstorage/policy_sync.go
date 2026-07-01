package controllerstorage

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
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

type IPGroupPolicySyncBuilder func(tx *PolicyMutationTx, updated *IPGroupRecord, nodes []*Node) ([]PolicySyncRequest, error)

type PolicyMutationTx struct {
	tx *sql.Tx
}

func (m *PolicyMutationTx) CreateTenantNodeACLRule(rule *ACLRuleRecord) (*ACLRuleRecord, error) {
	return createTenantNodeACLRule(m.tx, rule, false)
}

func (m *PolicyMutationTx) UpdateTenantNodeACLRule(tenantID, nodeID, ruleID uuid.UUID, rule *ACLRuleRecord) (*ACLRuleRecord, error) {
	return updateTenantNodeACLRule(m.tx, tenantID, nodeID, ruleID, rule, false)
}

func (m *PolicyMutationTx) DeleteTenantNodeACLRuleByID(tenantID, nodeID, ruleID uuid.UUID) error {
	return deleteTenantNodeACLRuleByID(m.tx, tenantID, nodeID, ruleID, false)
}

func (m *PolicyMutationTx) CreateTenantNodeQoSRule(rule *QoSRuleRecord) (*QoSRuleRecord, error) {
	return createTenantNodeQoSRule(m.tx, rule, false)
}

func (m *PolicyMutationTx) UpdateTenantNodeQoSRule(tenantID, nodeID, ruleID uuid.UUID, rule *QoSRuleRecord) (*QoSRuleRecord, error) {
	return updateTenantNodeQoSRule(m.tx, tenantID, nodeID, ruleID, rule, false)
}

func (m *PolicyMutationTx) DeleteTenantNodeQoSRule(tenantID, nodeID, ruleID uuid.UUID) error {
	return deleteTenantNodeQoSRule(m.tx, tenantID, nodeID, ruleID, false)
}

func (m *PolicyMutationTx) ResolvePolicyGroupRef(tenantID uuid.UUID, explicit uuid.NullUUID, directCIDR string) (uuid.NullUUID, error) {
	return resolvePolicyGroupRefWith(m.tx, tenantID, explicit, directCIDR)
}

func (m *PolicyMutationTx) GetIPGroup(tenantID, groupID uuid.UUID) (*IPGroupRecord, error) {
	return getIPGroupWith(m.tx, tenantID, groupID)
}

func (m *PolicyMutationTx) ListTenantNodeACLRules(tenantID, nodeID uuid.UUID) ([]*ACLRuleRecord, error) {
	return listTenantNodeACLRules(m.tx, tenantID, nodeID, false)
}

func (m *PolicyMutationTx) ListTenantNodeQoSRules(tenantID, nodeID uuid.UUID) ([]*QoSRuleRecord, error) {
	return listTenantNodeQoSRules(m.tx, tenantID, nodeID, false)
}

func (m *PolicyMutationTx) CreateTenantNodeBlacklistRule(rule *BlacklistRuleRecord) (*BlacklistRuleRecord, error) {
	return createTenantNodeBlacklistRule(m.tx, rule, false)
}

func (m *PolicyMutationTx) DeleteTenantNodeBlacklistRuleByID(tenantID, nodeID uuid.UUID, scope string, ruleID uuid.UUID) error {
	return deleteTenantNodeBlacklistRuleByID(m.tx, tenantID, nodeID, scope, ruleID, false)
}

func (m *PolicyMutationTx) UpdateTenantNodeRoutes(nodeID uuid.UUID, tenantID uuid.UUID, routes []string) error {
	result, err := m.tx.Exec(
		`UPDATE nodes SET advertised_routes = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
		pq.Array(routes), nodeID, tenantID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Storage) UpdateTenantNodeAdvertisedRoutes(nodeID uuid.UUID, tenantID uuid.UUID, routes []string) error {
	result, err := s.db.Exec(
		`UPDATE nodes SET advertised_routes = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND status NOT IN ('deleted', 'suspended', 'banned')`,
		pq.Array(routes), nodeID, tenantID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Storage) MutatePolicyAndQueueSync(mutate func(*PolicyMutationTx) (PolicySyncRequest, error)) (*PolicySyncResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(tx, "MutatePolicyAndQueueSync")
		}
	}()

	req, err := mutate(&PolicyMutationTx{tx: tx})
	if err != nil {
		return nil, err
	}

	result, err := s.queuePolicySyncTx(tx, req)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	s.recordCommandQueuedAudit(req, result.Command)
	return result, nil
}

func (s *Storage) UpdateIPGroupAndQueuePolicySyncs(tenantID, groupID uuid.UUID, group *IPGroupRecord, build IPGroupPolicySyncBuilder) (*IPGroupRecord, []*PolicySyncResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(tx, "UpdateIPGroupAndQueuePolicySyncs")
		}
	}()

	updated, err := updateIPGroupWith(tx, tenantID, groupID, group)
	if err != nil {
		return nil, nil, err
	}

	nodes, err := s.listNodesReferencingIPGroupWith(tx, tenantID, groupID)
	if err != nil {
		return nil, nil, err
	}

	var requests []PolicySyncRequest
	if build != nil {
		requests, err = build(&PolicyMutationTx{tx: tx}, updated, nodes)
		if err != nil {
			return nil, nil, err
		}
	}

	results := make([]*PolicySyncResult, 0, len(requests))
	for _, req := range requests {
		result, err := s.queuePolicySyncTx(tx, req)
		if err != nil {
			return nil, nil, err
		}
		results = append(results, result)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	committed = true

	for i, req := range requests {
		if i < len(results) && results[i] != nil {
			s.recordCommandQueuedAudit(req, results[i].Command)
		}
	}
	return updated, results, nil
}

func (s *Storage) QueuePolicySync(req PolicySyncRequest) (*PolicySyncResult, error) {
	if err := normalizePolicySyncRequest(&req); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(tx, "QueuePolicySync")
		}
	}()

	result, err := s.queuePolicySyncTx(tx, req)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	s.recordCommandQueuedAudit(req, result.Command)
	return result, nil
}

func normalizePolicySyncRequest(req *PolicySyncRequest) error {
	req.NodePublicKey = strings.TrimSpace(req.NodePublicKey)
	req.Domain = strings.TrimSpace(req.Domain)
	req.Action = strings.TrimSpace(req.Action)
	req.PolicyRef = strings.TrimSpace(req.PolicyRef)
	req.PolicyName = strings.TrimSpace(req.PolicyName)
	req.DesiredStateVersion = strings.TrimSpace(req.DesiredStateVersion)

	if req.TenantID == uuid.Nil {
		return errors.New("tenant id is required")
	}
	if req.NodeID == uuid.Nil {
		return errors.New("node id is required")
	}
	if req.NodePublicKey == "" {
		return errors.New("node public key is required")
	}
	if req.Domain == "" {
		return errors.New("policy domain is required")
	}
	if req.Action == "" {
		return errors.New("policy action is required")
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
	return nil
}

func (s *Storage) queuePolicySyncTx(tx *sql.Tx, req PolicySyncRequest) (*PolicySyncResult, error) {
	if err := normalizePolicySyncRequest(&req); err != nil {
		return nil, err
	}

	state, err := s.upsertNodeDesiredState(tx, req.TenantID, req.NodeID, req.DesiredStateVersion, req.DesiredMetadata)
	if err != nil {
		return nil, err
	}

	if err := markSupersededPolicySyncsTx(tx, req); err != nil {
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

	return &PolicySyncResult{
		DesiredStateVersion: req.DesiredStateVersion,
		ControlState:        state,
		Command:             cmd,
		Delivery:            delivery,
	}, nil
}

func markSupersededPolicySyncsTx(tx roleExec, req PolicySyncRequest) error {
	message := fmt.Sprintf("superseded by desired state %s", req.DesiredStateVersion)

	if _, err := tx.Exec(`
		UPDATE agent_commands ac
		SET status = $2,
		    message = $3,
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE ac.node_public_key = $1
		  AND ac.status IN ($4, $5, $6)
		  AND EXISTS (
		      SELECT 1
		      FROM policy_deliveries pd
		      WHERE pd.command_id = ac.id
		        AND pd.tenant_id = $7
		        AND pd.node_id = $8
		        AND pd.command_status IN ($4, $5, $6)
		  )
	`,
		req.NodePublicKey,
		AgentCommandStatusStale,
		message,
		AgentCommandStatusPending,
		AgentCommandStatusSent,
		AgentCommandStatusAcknowledged,
		req.TenantID,
		req.NodeID,
	); err != nil {
		return err
	}

	_, err := tx.Exec(`
		UPDATE policy_deliveries
		SET command_status = $3,
		    last_error = $4,
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE tenant_id = $1
		  AND node_id = $2
		  AND command_status IN ($5, $6, $7)
	`,
		req.TenantID,
		req.NodeID,
		AgentCommandStatusStale,
		message,
		AgentCommandStatusPending,
		AgentCommandStatusSent,
		AgentCommandStatusAcknowledged,
	)
	return err
}

func (s *Storage) recordCommandQueuedAudit(req PolicySyncRequest, cmd *AgentCommand) {
	if s == nil || cmd == nil {
		return
	}
	if _, err := s.CreateAuditEvent(&AuditEvent{
		TenantID:  req.TenantID,
		NodeID:    &req.NodeID,
		EventType: AuditCommandQueued,
		Actor:     "controller",
		Summary:   fmt.Sprintf("Command queued: %s", cmd.Command),
		Detail: map[string]interface{}{
			"command_id":            cmd.ID,
			"command":               cmd.Command,
			"policy_domain":         req.Domain,
			"policy_ref":            req.PolicyRef,
			"desired_state_version": req.DesiredStateVersion,
		},
	}); err != nil {
		log.Printf("[policy_sync] failed to create command.queued audit event for command %s: %v", cmd.ID, err)
	}
}
