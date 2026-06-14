package grpc

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	controllerstorage "aria/pkg/controllerstorage"
	"aria/pkg/grpc/agentpb"
)

type agentPolicySnapshot struct {
	IPGroups       []*agentpb.IPGroup
	ACLRules       []*agentpb.ACLRule
	QoSRules       []*agentpb.QoSRule
	BlacklistRules []*agentpb.BlacklistRule
}

func compileAgentPolicySnapshot(
	legacyACLRules interface{},
	qosRules []*agentpb.QoSRule,
	blacklistRules []*agentpb.BlacklistRule,
) (*agentPolicySnapshot, error) {
	aclRules, err := aclRulesFromLegacyPayload(legacyACLRules)
	if err != nil {
		return nil, err
	}

	compiled := &agentPolicySnapshot{}
	for _, rule := range aclRules {
		rules, err := compileACLRule(rule)
		if err != nil {
			return nil, err
		}
		compiled.ACLRules = append(compiled.ACLRules, rules...)
	}
	for _, rule := range blacklistRules {
		rules, err := compileBlacklistRule(rule)
		if err != nil {
			return nil, err
		}
		compiled.ACLRules = append(compiled.ACLRules, rules...)
	}
	for _, rule := range qosRules {
		compiledRule, err := compileQoSRule(rule)
		if err != nil {
			return nil, err
		}
		compiled.QoSRules = append(compiled.QoSRules, compiledRule)
	}

	return compiled, nil
}

func compileAgentPolicySnapshotWithGroups(
	groups []*controllerstorage.IPGroupRecord,
	aclRuleRecords []*controllerstorage.ACLRuleRecord,
	qosRuleRecords []*controllerstorage.QoSRuleRecord,
	blacklistRules []*agentpb.BlacklistRule,
) (*agentPolicySnapshot, error) {
	compiled := &agentPolicySnapshot{}
	referencedGroups := map[string]struct{}{}

	aclRuleRecords = sortedACLRuleRecords(aclRuleRecords)
	for _, record := range aclRuleRecords {
		if record == nil || !record.Enabled {
			continue
		}
		rule := aclRecordToProto(record)
		recordACLGroupRefs(record, referencedGroups)
		rules, err := compileACLRule(rule)
		if err != nil {
			return nil, err
		}
		compiled.ACLRules = append(compiled.ACLRules, rules...)
	}

	for _, rule := range blacklistRules {
		rules, err := compileBlacklistRule(rule)
		if err != nil {
			return nil, err
		}
		compiled.ACLRules = append(compiled.ACLRules, rules...)
	}

	qosRuleRecords = sortedQoSRuleRecords(qosRuleRecords)
	for _, record := range qosRuleRecords {
		if record == nil || !record.Enabled {
			continue
		}
		rule := qosRecordToProto(record)
		recordQoSGroupRefs(record, referencedGroups)
		compiledRule, err := compileQoSRule(rule)
		if err != nil {
			return nil, err
		}
		compiled.QoSRules = append(compiled.QoSRules, compiledRule)
	}

	ipGroups, err := compileReferencedIPGroups(groups, referencedGroups)
	if err != nil {
		return nil, err
	}
	compiled.IPGroups = ipGroups
	return compiled, nil
}

func sortedACLRuleRecords(rules []*controllerstorage.ACLRuleRecord) []*controllerstorage.ACLRuleRecord {
	sorted := append([]*controllerstorage.ACLRuleRecord(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	return sorted
}

func sortedQoSRuleRecords(rules []*controllerstorage.QoSRuleRecord) []*controllerstorage.QoSRuleRecord {
	sorted := append([]*controllerstorage.QoSRuleRecord(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	return sorted
}

func aclRecordToProto(record *controllerstorage.ACLRuleRecord) *agentpb.ACLRule {
	rule := &agentpb.ACLRule{
		Id:        record.ID.String(),
		SrcNet:    strings.TrimSpace(record.SrcCIDR),
		DstNet:    strings.TrimSpace(record.DstCIDR),
		Protocol:  uint32(record.Protocol),
		Action:    record.Action,
		Direction: record.Direction,
		Ports:     record.Ports,
		Priority:  uint32(record.Priority),
	}
	if record.DstPort > 0 {
		rule.MinPort = uint32(record.DstPort)
		rule.MaxPort = uint32(record.DstPort)
	}
	if record.SrcGroupID.Valid {
		rule.SrcGroupId = record.SrcGroupID.UUID.String()
	}
	if record.DstGroupID.Valid {
		rule.DstGroupId = record.DstGroupID.UUID.String()
	}
	return rule
}

func qosRecordToProto(record *controllerstorage.QoSRuleRecord) *agentpb.QoSRule {
	rule := &agentpb.QoSRule{
		Id:            record.ID.String(),
		SrcIp:         strings.TrimSpace(record.SrcCIDR),
		DstIp:         strings.TrimSpace(record.DstCIDR),
		SrcPort:       uint32(record.SrcPort),
		DstPort:       uint32(record.DstPort),
		Protocol:      uint32(record.Protocol),
		BandwidthMbps: uint64(record.BandwidthMbps),
		Direction:     record.Direction,
		RateBps:       record.RateBps,
		BurstBytes:    record.BurstBytes,
		Priority:      uint32(record.Priority),
		Mode:          record.Mode,
	}
	if record.GroupID.Valid {
		rule.GroupId = record.GroupID.UUID.String()
	}
	return rule
}

func recordACLGroupRefs(record *controllerstorage.ACLRuleRecord, refs map[string]struct{}) {
	if record == nil {
		return
	}
	if record.SrcGroupID.Valid {
		refs[record.SrcGroupID.UUID.String()] = struct{}{}
	}
	if record.DstGroupID.Valid {
		refs[record.DstGroupID.UUID.String()] = struct{}{}
	}
}

func recordQoSGroupRefs(record *controllerstorage.QoSRuleRecord, refs map[string]struct{}) {
	if record == nil {
		return
	}
	if record.GroupID.Valid {
		refs[record.GroupID.UUID.String()] = struct{}{}
	}
}

func compileReferencedIPGroups(groups []*controllerstorage.IPGroupRecord, refs map[string]struct{}) ([]*agentpb.IPGroup, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	groups = sortedIPGroups(groups)
	compiled := make([]*agentpb.IPGroup, 0, len(refs))
	seen := map[string]struct{}{}
	for _, group := range groups {
		if group == nil {
			continue
		}
		groupID := group.ID.String()
		if _, ok := refs[groupID]; !ok {
			continue
		}
		compiled = append(compiled, compileIPGroup(group))
		seen[groupID] = struct{}{}
	}

	for groupID := range refs {
		if _, ok := seen[groupID]; !ok {
			return nil, fmt.Errorf("policy references missing IP group %s", groupID)
		}
	}
	return compiled, nil
}

func sortedIPGroups(groups []*controllerstorage.IPGroupRecord) []*controllerstorage.IPGroupRecord {
	sorted := append([]*controllerstorage.IPGroupRecord(nil), groups...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.ID.String() < right.ID.String()
	})
	return sorted
}

func compileIPGroup(group *controllerstorage.IPGroupRecord) *agentpb.IPGroup {
	cidrs := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		if cidr := strings.TrimSpace(member.CIDR); cidr != "" {
			cidrs = append(cidrs, cidr)
		}
	}
	sort.Strings(cidrs)
	return &agentpb.IPGroup{
		Id:    group.ID.String(),
		Name:  group.Name,
		Cidrs: cidrs,
		Kind:  group.Kind,
	}
}

func aclRulesFromLegacyPayload(payload interface{}) ([]*agentpb.ACLRule, error) {
	if payload == nil {
		return nil, nil
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal ACL sync payload: %w", err)
	}

	var ruleList []map[string]interface{}
	if err := json.Unmarshal(encoded, &ruleList); err != nil {
		return nil, fmt.Errorf("decode ACL sync payload: %w", err)
	}

	rules := make([]*agentpb.ACLRule, 0, len(ruleList))
	for _, raw := range ruleList {
		rules = append(rules, &agentpb.ACLRule{
			Id:        getString(raw, "id"),
			SrcNet:    getString(raw, "src_net"),
			DstNet:    getString(raw, "dst_net"),
			Protocol:  getUint32(raw, "protocol"),
			MinPort:   getUint32(raw, "min_port"),
			MaxPort:   getUint32(raw, "max_port"),
			Action:    getString(raw, "action"),
			Direction: getString(raw, "direction"),
			Ports:     getString(raw, "ports"),
			Priority:  getUint32(raw, "priority"),
		})
	}
	return rules, nil
}

func compileACLRule(rule *agentpb.ACLRule) ([]*agentpb.ACLRule, error) {
	if rule == nil {
		return nil, nil
	}

	action, err := normalizeACLAction(rule.GetAction())
	if err != nil {
		return nil, err
	}
	direction, err := normalizePolicyDirection(rule.GetDirection(), "ingress")
	if err != nil {
		return nil, err
	}
	ports, err := normalizeACLPorts(rule.GetPorts(), rule.GetMinPort(), rule.GetMaxPort())
	if err != nil {
		return nil, err
	}

	protocol := rule.GetProtocol()
	if protocol > math.MaxUint8 {
		return nil, fmt.Errorf("ACL protocol %d exceeds uint8", protocol)
	}

	protocols := []uint32{protocol}
	if ports != "" {
		switch protocol {
		case 0:
			protocols = []uint32{6, 17}
		case 6, 17:
		default:
			return nil, fmt.Errorf("ACL port filters require tcp or udp protocol, got %d", protocol)
		}
	}

	compiled := make([]*agentpb.ACLRule, 0, len(protocols))
	for _, proto := range protocols {
		compiled = append(compiled, &agentpb.ACLRule{
			Id:         rule.GetId(),
			SrcNet:     aclNetFallback(rule.GetSrcNet(), rule.GetSrcGroupId()),
			DstNet:     aclNetFallback(rule.GetDstNet(), rule.GetDstGroupId()),
			Protocol:   proto,
			Action:     action,
			Direction:  direction,
			Ports:      ports,
			SrcGroupId: rule.GetSrcGroupId(),
			DstGroupId: rule.GetDstGroupId(),
			Priority:   rule.GetPriority(),
		})
	}
	return compiled, nil
}

func compileBlacklistRule(rule *agentpb.BlacklistRule) ([]*agentpb.ACLRule, error) {
	if rule == nil {
		return nil, nil
	}

	switch strings.ToLower(strings.TrimSpace(rule.GetScope())) {
	case "src":
		if strings.TrimSpace(rule.GetCidr()) == "" {
			return nil, fmt.Errorf("source blacklist rule requires cidr")
		}
		return []*agentpb.ACLRule{{
			SrcNet:    cidrOrAny(rule.GetCidr()),
			DstNet:    "any",
			Protocol:  0,
			Action:    "deny",
			Direction: "ingress",
		}}, nil
	case "dst":
		if strings.TrimSpace(rule.GetCidr()) == "" {
			return nil, fmt.Errorf("destination blacklist rule requires cidr")
		}
		return []*agentpb.ACLRule{{
			SrcNet:    "any",
			DstNet:    cidrOrAny(rule.GetCidr()),
			Protocol:  0,
			Action:    "deny",
			Direction: "ingress",
		}}, nil
	case "ports":
		if rule.GetPort() == 0 || rule.GetPort() > math.MaxUint16 {
			return nil, fmt.Errorf("port blacklist rule requires port in 1..65535")
		}
		port := fmt.Sprintf("%d", rule.GetPort())
		return []*agentpb.ACLRule{
			{SrcNet: "any", DstNet: "any", Protocol: 6, Action: "deny", Direction: "ingress", Ports: port},
			{SrcNet: "any", DstNet: "any", Protocol: 17, Action: "deny", Direction: "ingress", Ports: port},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported blacklist scope %q", rule.GetScope())
	}
}

func compileQoSRule(rule *agentpb.QoSRule) (*agentpb.QoSRule, error) {
	if rule == nil {
		return nil, nil
	}

	direction, err := normalizePolicyDirection(rule.GetDirection(), inferredQoSDirection(rule))
	if err != nil {
		return nil, err
	}
	group := qosGroupForDirection(rule, direction)
	if strings.TrimSpace(rule.GetGroupId()) != "" {
		group = qosCIDRFallbackForDirection(rule, direction)
	}

	rateBps := rule.GetRateBps()
	if rateBps == 0 && rule.GetBandwidthMbps() > 0 {
		rateBps = rule.GetBandwidthMbps() * 1_000_000
	}
	if rateBps == 0 {
		return nil, fmt.Errorf("QoS rule requires rate_bps or bandwidth_mbps")
	}

	priority := rule.GetPriority()
	if priority > math.MaxUint8 {
		return nil, fmt.Errorf("QoS priority %d exceeds uint8", priority)
	}

	mode, err := normalizeQoSMode(rule.GetMode())
	if err != nil {
		return nil, err
	}

	compiled := &agentpb.QoSRule{
		Id:            rule.GetId(),
		Protocol:      rule.GetProtocol(),
		BandwidthMbps: rule.GetBandwidthMbps(),
		Direction:     direction,
		RateBps:       rateBps,
		BurstBytes:    rule.GetBurstBytes(),
		Priority:      priority,
		Mode:          mode,
		GroupId:       rule.GetGroupId(),
	}
	if compiled.BurstBytes == 0 {
		compiled.BurstBytes = defaultQoSBurst(rateBps)
	}
	switch direction {
	case "ingress":
		if group != "" {
			compiled.SrcIp = group
		}
	case "egress", "both":
		if group != "" {
			compiled.DstIp = group
		}
	default:
		return nil, fmt.Errorf("invalid normalized QoS direction %q", direction)
	}
	return compiled, nil
}

func aclNetFallback(cidr, groupID string) string {
	if strings.TrimSpace(groupID) != "" {
		return strings.TrimSpace(cidr)
	}
	return cidrOrAny(cidr)
}

func normalizeACLAction(action string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "accept", "pass", "allow":
		return "allow", nil
	case "drop", "deny":
		return "deny", nil
	default:
		return "", fmt.Errorf("invalid ACL action %q", action)
	}
}

func normalizePolicyDirection(direction, fallback string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(direction))
	if value == "" {
		value = fallback
	}
	switch value {
	case "ingress", "in":
		return "ingress", nil
	case "egress", "out":
		return "egress", nil
	case "both", "all":
		return "both", nil
	default:
		return "", fmt.Errorf("invalid policy direction %q", direction)
	}
}

func normalizeACLPorts(ports string, minPort, maxPort uint32) (string, error) {
	trimmed := strings.TrimSpace(ports)
	if trimmed != "" {
		if strings.EqualFold(trimmed, "all") {
			return "", nil
		}
		return trimmed, nil
	}

	if minPort == 0 && (maxPort == 0 || maxPort == math.MaxUint16) {
		return "", nil
	}
	if minPort > math.MaxUint16 || maxPort > math.MaxUint16 {
		return "", fmt.Errorf("ACL port range must be within 0..65535")
	}
	if maxPort == 0 {
		maxPort = minPort
	}
	if minPort > maxPort {
		return "", fmt.Errorf("invalid ACL port range %d-%d", minPort, maxPort)
	}
	if minPort == maxPort {
		return fmt.Sprintf("%d", minPort), nil
	}
	return fmt.Sprintf("%d-%d", minPort, maxPort), nil
}

func cidrOrAny(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "0" || trimmed == "0.0.0.0/0" || trimmed == "::/0" {
		return "any"
	}
	return trimmed
}

func inferredQoSDirection(rule *agentpb.QoSRule) string {
	if strings.TrimSpace(rule.GetSrcIp()) != "" && strings.TrimSpace(rule.GetDstIp()) == "" {
		return "ingress"
	}
	return "egress"
}

func qosGroupForDirection(rule *agentpb.QoSRule, direction string) string {
	switch direction {
	case "ingress":
		return cidrOrAny(rule.GetSrcIp())
	default:
		dst := cidrOrAny(rule.GetDstIp())
		if dst == "any" {
			return cidrOrAny(rule.GetSrcIp())
		}
		return dst
	}
}

func qosCIDRFallbackForDirection(rule *agentpb.QoSRule, direction string) string {
	switch direction {
	case "ingress":
		return strings.TrimSpace(rule.GetSrcIp())
	default:
		dst := strings.TrimSpace(rule.GetDstIp())
		if dst != "" {
			return dst
		}
		return strings.TrimSpace(rule.GetSrcIp())
	}
}

func normalizeQoSMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "policing", "shaping":
		return "policing", nil
	default:
		return "", fmt.Errorf("invalid QoS mode %q", mode)
	}
}

func defaultQoSBurst(rateBps uint64) uint64 {
	burst := rateBps / 8 / 10
	if burst < 1500 {
		return 1500
	}
	return burst
}
