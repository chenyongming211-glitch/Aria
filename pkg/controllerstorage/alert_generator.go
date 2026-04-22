package controllerstorage

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// GenerateNodeOfflineAlert creates a node_offline alert if one does not already exist (idempotent).
// Errors are logged but not returned to avoid blocking the caller.
func (s *Storage) GenerateNodeOfflineAlert(tenantID, nodeID uuid.UUID, hostname string) error {
	// Check for existing active node_offline alert (idempotent)
	existing, err := s.GetActiveAlertByNodeAndType(tenantID, nodeID, "node_offline")
	if err != nil {
		log.Printf("[alert_generator] failed to check existing node_offline alert for node %s: %v", nodeID, err)
		return fmt.Errorf("failed to check existing alert: %w", err)
	}
	if existing != nil {
		return nil
	}

	alert := &Alert{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		AlertType: "node_offline",
		Severity:  "critical",
		Title:     fmt.Sprintf("节点 %s 离线", hostname),
		Context:   map[string]interface{}{"hostname": hostname},
	}
	_, err = s.CreateAlert(alert)
	if err != nil {
		log.Printf("[alert_generator] failed to create node_offline alert for node %s: %v", nodeID, err)
		return fmt.Errorf("failed to create node_offline alert: %w", err)
	}

	// Create audit event (non-blocking: log errors but don't return them)
	auditEvent := &AuditEvent{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		EventType: "alert_created",
		Actor:     "system",
		Summary:   fmt.Sprintf("告警创建: 节点 %s 离线", hostname),
	}
	if _, err := s.CreateAuditEvent(auditEvent); err != nil {
		log.Printf("[alert_generator] failed to create audit event for node_offline alert on node %s: %v", nodeID, err)
	}

	return nil
}

// ResolveNodeOfflineAlert finds and resolves an active node_offline alert for the given node.
// If no active alert exists, returns nil. Creates an alert_resolved audit event on success.
func (s *Storage) ResolveNodeOfflineAlert(tenantID, nodeID uuid.UUID) error {
	existing, err := s.GetActiveAlertByNodeAndType(tenantID, nodeID, "node_offline")
	if err != nil {
		return fmt.Errorf("failed to find active node_offline alert: %w", err)
	}
	if existing == nil {
		return nil
	}

	if _, err := s.ResolveAlert(existing.ID); err != nil {
		return fmt.Errorf("failed to resolve node_offline alert %s: %w", existing.ID, err)
	}

	// Create audit event (non-blocking)
	auditEvent := &AuditEvent{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		EventType: "alert_resolved",
		Actor:     "system",
		Summary:   "告警解除: 节点离线告警已自动解除",
	}
	if _, err := s.CreateAuditEvent(auditEvent); err != nil {
		log.Printf("[alert_generator] failed to create audit event for resolved node_offline alert on node %s: %v", nodeID, err)
	}

	return nil
}

// GenerateSyncFailedAlert creates a sync_failed alert when an agent command fails.
// Also creates an alert_created audit event. Idempotent check applied.
func (s *Storage) GenerateSyncFailedAlert(tenantID, nodeID uuid.UUID, commandID, errorMsg string) error {
	// 检查是否已有活跃的相同类型告警
	existing, _ := s.GetActiveAlertByNodeAndType(tenantID, nodeID, "sync_failed")
	if existing != nil {
		return nil
	}

	alert := &Alert{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		AlertType: "sync_failed",
		Severity:  "warning",
		Title:     "命令执行失败",
		Context: map[string]interface{}{
			"command_id": commandID,
			"error":      errorMsg,
		},
	}
	if _, err := s.CreateAlert(alert); err != nil {
		log.Printf("[alert_generator] failed to create sync_failed alert for node %s: %v", nodeID, err)
		return fmt.Errorf("failed to create sync_failed alert: %w", err)
	}

	auditEvent := &AuditEvent{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		EventType: "alert_created",
		Actor:     "system",
		Summary:   "告警创建: 命令执行失败",
	}
	if _, err := s.CreateAuditEvent(auditEvent); err != nil {
		log.Printf("[alert_generator] failed to create audit event for sync_failed alert on node %s: %v", nodeID, err)
	}

	return nil
}

// GeneratePolicyFailedAlert creates a policy_failed alert when a policy delivery fails.
// Also creates an alert_created audit event. Idempotent check applied.
func (s *Storage) GeneratePolicyFailedAlert(tenantID, nodeID uuid.UUID, domain, ref, errorMsg string) error {
	// 检查是否已有活跃的相同类型告警
	existing, _ := s.GetActiveAlertByNodeAndType(tenantID, nodeID, "policy_failed")
	if existing != nil {
		return nil
	}

	alert := &Alert{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		AlertType: "policy_failed",
		Severity:  "warning",
		Title:     "策略下发失败",
		Context: map[string]interface{}{
			"policy_domain": domain,
			"policy_ref":    ref,
			"error":         errorMsg,
		},
	}
	if _, err := s.CreateAlert(alert); err != nil {
		log.Printf("[alert_generator] failed to create policy_failed alert for node %s: %v", nodeID, err)
		return fmt.Errorf("failed to create policy_failed alert: %w", err)
	}

	auditEvent := &AuditEvent{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		EventType: "alert_created",
		Actor:     "system",
		Summary:   "告警创建: 策略下发失败",
	}
	if _, err := s.CreateAuditEvent(auditEvent); err != nil {
		log.Printf("[alert_generator] failed to create audit event for policy_failed alert on node %s: %v", nodeID, err)
	}

	return nil
}

func (s *Storage) GenerateCertificateExpiringAlert(tenantID, nodeID uuid.UUID, hostname string, notAfter time.Time) error {
	existing, err := s.GetActiveAlertByNodeAndType(tenantID, nodeID, "certificate_expiring")
	if err != nil {
		return fmt.Errorf("failed to check existing certificate_expiring alert: %w", err)
	}
	if existing != nil {
		return nil
	}

	alert := &Alert{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		AlertType: "certificate_expiring",
		Severity:  "warning",
		Title:     "节点证书即将到期",
		Message:   fmt.Sprintf("节点 %s 的客户端证书将在 %s 过期", hostname, notAfter.UTC().Format(time.RFC3339)),
		Context: map[string]interface{}{
			"hostname": hostname,
			"not_after": notAfter.UTC().Format(time.RFC3339),
		},
	}
	if _, err := s.CreateAlert(alert); err != nil {
		return fmt.Errorf("failed to create certificate_expiring alert: %w", err)
	}
	return nil
}

func (s *Storage) ResolveCertificateExpiringAlert(tenantID, nodeID uuid.UUID) error {
	existing, err := s.GetActiveAlertByNodeAndType(tenantID, nodeID, "certificate_expiring")
	if err != nil {
		return fmt.Errorf("failed to find active certificate_expiring alert: %w", err)
	}
	if existing == nil {
		return nil
	}
	if _, err := s.ResolveAlert(existing.ID); err != nil {
		return fmt.Errorf("failed to resolve certificate_expiring alert %s: %w", existing.ID, err)
	}
	return nil
}

func (s *Storage) GenerateCertificateExpiredAlert(tenantID, nodeID uuid.UUID, hostname string, notAfter time.Time) error {
	existing, err := s.GetActiveAlertByNodeAndType(tenantID, nodeID, "certificate_expired")
	if err != nil {
		return fmt.Errorf("failed to check existing certificate_expired alert: %w", err)
	}
	if existing != nil {
		return nil
	}

	alert := &Alert{
		TenantID:  tenantID,
		NodeID:    &nodeID,
		AlertType: "certificate_expired",
		Severity:  "critical",
		Title:     "节点证书已过期",
		Message:   fmt.Sprintf("节点 %s 的客户端证书已于 %s 过期", hostname, notAfter.UTC().Format(time.RFC3339)),
		Context: map[string]interface{}{
			"hostname": hostname,
			"not_after": notAfter.UTC().Format(time.RFC3339),
		},
	}
	if _, err := s.CreateAlert(alert); err != nil {
		return fmt.Errorf("failed to create certificate_expired alert: %w", err)
	}
	return nil
}
