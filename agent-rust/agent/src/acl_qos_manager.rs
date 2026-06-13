use std::collections::HashMap;
use std::net::IpAddr;
use std::sync::{Arc, Mutex as StdMutex};

use aya::Ebpf;
use thiserror::Error;

use crate::acl_qos_maps::{
    add_policy_to_maps, add_qos_rule_to_maps, cleanup_root_qdisc, delete_policy_from_maps,
    delete_port_set_from_maps, delete_qos_rule_from_maps, ensure_fq_qdisc, get_qos_stats,
    get_rule_stats, sync_runtime_config, AclQosMapHandles, TapMapRuntime,
};
use crate::acl_qos_state::{requested_directions, FirewallState, GroupInfo, QosRuleInfo};
use crate::identity::{parse_single_ip, IdentityManager, ID_WILDCARD};

#[derive(Error, Debug)]
pub enum AclQosError {
    #[error("Identity error: {0}")]
    Identity(#[from] crate::identity::IdentityError),
    #[error("Validation error: {0}")]
    Validation(String),
    #[error("Kernel map error: {0}")]
    Kernel(String),
    #[error("Lock error")]
    Lock,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AclRuleSpec {
    pub id: String,
    pub src_group: String,
    pub dst_group: String,
    pub proto: u8,
    pub action: u8,
    pub priority: u16,
    pub direction: u8,
    pub ports: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct QosRuleSpec {
    pub id: String,
    pub group: String,
    pub direction: u8,
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    pub mode: u8,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct AclQosSnapshot {
    pub acl_rules: Vec<AclRuleSpec>,
    pub qos_rules: Vec<QosRuleSpec>,
    pub acl_enabled: bool,
    pub qos_enabled: bool,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct CompiledAclRule {
    rule_id: String,
    src_group_id: u32,
    dst_group_id: u32,
    proto: u8,
    action: u8,
    priority: u16,
    direction: u8,
    ports: Option<String>,
    order: usize,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct CompiledQosRule {
    rule_id: String,
    group_name: String,
    group_id: u32,
    direction: u8,
    rate_bps: u64,
    burst_bytes: u64,
    priority: u8,
    mode: u8,
    order: usize,
}

pub struct AclQosManager {
    maps: AclQosMapHandles,
    identity_mgr: Arc<StdMutex<IdentityManager>>,
    state: FirewallState,
    last_snapshot: Option<AclQosSnapshot>,
    runtime: TapMapRuntime,
    interfaces: Vec<String>,
    _acl_ebpf: Ebpf,
    _qos_ebpf: Ebpf,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct AclRuleRuntimeStats {
    pub id: String,
    pub packets: u64,
    pub bytes: u64,
    pub dropped_packets: u64,
    pub dropped_bytes: u64,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct QosRuleRuntimeStats {
    pub id: String,
    pub passed_bytes: u64,
    pub dropped_bytes: u64,
    pub shaped_bytes: u64,
}

fn merge_acl_rule_runtime_stats(
    stats_by_id: &mut HashMap<String, AclRuleRuntimeStats>,
    ordered_ids: &mut Vec<String>,
    id: &str,
    packets: u64,
    bytes: u64,
    dropped_packets: u64,
    dropped_bytes: u64,
) {
    let entry = stats_by_id.entry(id.to_string()).or_insert_with(|| {
        ordered_ids.push(id.to_string());
        AclRuleRuntimeStats {
            id: id.to_string(),
            ..Default::default()
        }
    });
    entry.packets = entry.packets.saturating_add(packets);
    entry.bytes = entry.bytes.saturating_add(bytes);
    entry.dropped_packets = entry.dropped_packets.saturating_add(dropped_packets);
    entry.dropped_bytes = entry.dropped_bytes.saturating_add(dropped_bytes);
}

fn merge_qos_rule_runtime_stats(
    stats_by_id: &mut HashMap<String, QosRuleRuntimeStats>,
    ordered_ids: &mut Vec<String>,
    id: &str,
    passed_bytes: u64,
    dropped_bytes: u64,
    shaped_bytes: u64,
) {
    let entry = stats_by_id.entry(id.to_string()).or_insert_with(|| {
        ordered_ids.push(id.to_string());
        QosRuleRuntimeStats {
            id: id.to_string(),
            ..Default::default()
        }
    });
    entry.passed_bytes = entry.passed_bytes.saturating_add(passed_bytes);
    entry.dropped_bytes = entry.dropped_bytes.saturating_add(dropped_bytes);
    entry.shaped_bytes = entry.shaped_bytes.saturating_add(shaped_bytes);
}

fn compile_acl_rules_for_groups(
    rules: &[AclRuleSpec],
    groups: &HashMap<String, GroupInfo>,
) -> Result<Vec<CompiledAclRule>, AclQosError> {
    let mut selected: HashMap<(u32, u32, u8, u8), CompiledAclRule> = HashMap::new();

    for (order, rule) in rules.iter().enumerate() {
        let src_groups = matching_groups(groups, &rule.src_group)?;
        let dst_groups = matching_groups(groups, &rule.dst_group)?;
        for direction in requested_directions(rule.direction) {
            for (_, src_group_id) in &src_groups {
                for (_, dst_group_id) in &dst_groups {
                    let candidate = CompiledAclRule {
                        rule_id: rule.id.clone(),
                        src_group_id: *src_group_id,
                        dst_group_id: *dst_group_id,
                        proto: rule.proto,
                        action: rule.action,
                        priority: rule.priority,
                        direction,
                        ports: rule.ports.clone(),
                        order,
                    };
                    let key = (*src_group_id, *dst_group_id, rule.proto, direction);
                    let replace = selected
                        .get(&key)
                        .map(|existing| acl_candidate_wins(existing, &candidate))
                        .unwrap_or(true);
                    if replace {
                        selected.insert(key, candidate);
                    }
                }
            }
        }
    }

    let mut compiled: Vec<_> = selected.into_values().collect();
    compiled.sort_by_key(|rule| {
        (
            rule.order,
            rule.src_group_id,
            rule.dst_group_id,
            rule.proto,
            rule.direction,
        )
    });
    Ok(compiled)
}

fn compile_qos_rules_for_groups(
    rules: &[QosRuleSpec],
    groups: &HashMap<String, GroupInfo>,
) -> Result<Vec<CompiledQosRule>, AclQosError> {
    let mut selected: HashMap<(u32, u8), CompiledQosRule> = HashMap::new();

    for (order, rule) in rules.iter().enumerate() {
        let group_matches = matching_groups(groups, &rule.group)?;
        for direction in requested_directions(rule.direction) {
            for (group_name, group_id) in &group_matches {
                let candidate = CompiledQosRule {
                    rule_id: rule.id.clone(),
                    group_name: group_name.clone(),
                    group_id: *group_id,
                    direction,
                    rate_bps: rule.rate_bps,
                    burst_bytes: rule.burst_bytes,
                    priority: rule.priority,
                    mode: rule.mode,
                    order,
                };
                let key = (*group_id, direction);
                let replace = selected
                    .get(&key)
                    .map(|existing| qos_candidate_wins(existing, &candidate))
                    .unwrap_or(true);
                if replace {
                    selected.insert(key, candidate);
                }
            }
        }
    }

    let mut compiled: Vec<_> = selected.into_values().collect();
    compiled.sort_by_key(|rule| (rule.order, rule.group_id, rule.direction));
    Ok(compiled)
}

fn acl_candidate_wins(existing: &CompiledAclRule, candidate: &CompiledAclRule) -> bool {
    candidate.priority < existing.priority
        || (candidate.priority == existing.priority && candidate.order < existing.order)
}

fn qos_candidate_wins(existing: &CompiledQosRule, candidate: &CompiledQosRule) -> bool {
    candidate.priority < existing.priority
        || (candidate.priority == existing.priority && candidate.order < existing.order)
}

fn matching_groups(
    groups: &HashMap<String, GroupInfo>,
    raw: &str,
) -> Result<Vec<(String, u32)>, AclQosError> {
    let name = normalize_group_name(raw);
    if name == "any" {
        return Ok(vec![("any".to_string(), ID_WILDCARD)]);
    }

    let cidr = normalize_cidr(&name)?;
    let mut matches = Vec::new();
    for group in groups.values() {
        let mut matched = false;
        for group_cidr in &group.cidrs {
            if cidr_contains(&cidr, group_cidr)? {
                matched = true;
                break;
            }
        }
        if matched {
            matches.push((group.name.clone(), group.id));
        }
    }
    matches.sort_by_key(|(_, id)| *id);
    matches.dedup_by_key(|(_, id)| *id);
    if matches.is_empty() {
        return Err(AclQosError::Validation(format!(
            "no runtime group matches CIDR '{}'",
            cidr
        )));
    }
    Ok(matches)
}

#[derive(Clone, Copy)]
struct ParsedCidr {
    network: IpAddr,
    prefix_len: u8,
}

fn parse_runtime_cidr(cidr: &str) -> Result<ParsedCidr, AclQosError> {
    let Some((ip, prefix)) = cidr.split_once('/') else {
        return Err(AclQosError::Validation(format!("invalid CIDR '{}'", cidr)));
    };
    let network: IpAddr = ip
        .parse()
        .map_err(|_| AclQosError::Validation(format!("invalid CIDR IP '{}'", ip)))?;
    let prefix_len: u8 = prefix
        .parse()
        .map_err(|_| AclQosError::Validation(format!("invalid CIDR prefix '{}'", cidr)))?;
    let max_prefix = if network.is_ipv4() { 32 } else { 128 };
    if prefix_len > max_prefix {
        return Err(AclQosError::Validation(format!(
            "invalid CIDR prefix '{}'",
            cidr
        )));
    }
    Ok(ParsedCidr {
        network,
        prefix_len,
    })
}

fn cidr_contains(parent: &str, child: &str) -> Result<bool, AclQosError> {
    let parent = parse_runtime_cidr(parent)?;
    let child = parse_runtime_cidr(child)?;
    let (parent_bits, parent_max) = ip_bits(parent.network);
    let (child_bits, child_max) = ip_bits(child.network);
    if parent_max != child_max || parent.prefix_len > child.prefix_len {
        return Ok(false);
    }
    Ok(mask_bits(parent_bits, parent.prefix_len, parent_max)
        == mask_bits(child_bits, parent.prefix_len, child_max))
}

fn ip_bits(ip: IpAddr) -> (u128, u8) {
    match ip {
        IpAddr::V4(addr) => (u32::from(addr) as u128, 32),
        IpAddr::V6(addr) => (u128::from(addr), 128),
    }
}

fn mask_bits(bits: u128, prefix_len: u8, max_prefix: u8) -> u128 {
    if prefix_len == 0 {
        return 0;
    }
    let full_mask = if max_prefix == 128 {
        u128::MAX
    } else {
        (1u128 << max_prefix) - 1
    };
    let host_bits = max_prefix - prefix_len;
    let host_mask = if host_bits == 128 {
        u128::MAX
    } else {
        (1u128 << host_bits) - 1
    };
    bits & (full_mask ^ host_mask)
}

impl AclQosManager {
    pub fn new(
        mut acl_ebpf: Ebpf,
        mut qos_ebpf: Ebpf,
        identity_mgr: Arc<StdMutex<IdentityManager>>,
        interfaces: Vec<String>,
    ) -> Result<Self, AclQosError> {
        let mut manager = Self {
            maps: AclQosMapHandles::new(&mut acl_ebpf, &mut qos_ebpf)
                .map_err(AclQosError::Kernel)?,
            identity_mgr,
            state: FirewallState::default(),
            last_snapshot: None,
            runtime: TapMapRuntime::default(),
            interfaces,
            _acl_ebpf: acl_ebpf,
            _qos_ebpf: qos_ebpf,
        };
        sync_runtime_config(&mut manager.maps, manager.runtime, Some(true), Some(false))
            .map_err(AclQosError::Kernel)?;
        Ok(manager)
    }

    pub fn apply_snapshot(&mut self, snapshot: AclQosSnapshot) -> Result<(), AclQosError> {
        if self.last_snapshot.as_ref() == Some(&snapshot) {
            return Ok(());
        }

        let snapshot_cache = snapshot.clone();
        self.clear_all_acl_rules()?;
        self.clear_all_qos_rules()?;
        self.ensure_snapshot_groups(&snapshot)?;
        let compiled_acl_rules =
            compile_acl_rules_for_groups(&snapshot.acl_rules, &self.state.groups)?;
        let compiled_qos_rules =
            compile_qos_rules_for_groups(&snapshot.qos_rules, &self.state.groups)?;

        self.state.acl_enabled = snapshot.acl_enabled;
        self.state.qos_enabled = snapshot.qos_enabled;

        for rule in compiled_acl_rules {
            self.apply_compiled_policy(&rule)?;
        }

        let mut has_shaping = false;
        for rule in compiled_qos_rules {
            if rule.mode == 1 {
                has_shaping = true;
            }
            self.apply_compiled_qos(&rule)?;
        }

        if has_shaping {
            for iface in &self.interfaces {
                ensure_fq_qdisc(iface).map_err(AclQosError::Kernel)?;
            }
        } else {
            for iface in &self.interfaces {
                let _ = cleanup_root_qdisc(iface);
            }
        }

        let qos_enabled = snapshot.qos_enabled && !self.state.qos_rules.is_empty();
        sync_runtime_config(
            &mut self.maps,
            self.runtime,
            Some(snapshot.acl_enabled),
            Some(qos_enabled),
        )
        .map_err(AclQosError::Kernel)?;

        self.last_snapshot = Some(snapshot_cache);
        Ok(())
    }

    pub fn get_all_rule_stats(&self) -> Result<Vec<(u32, &'static str, u64, u64)>, AclQosError> {
        let stats = get_rule_stats(&self.maps, self.runtime).map_err(AclQosError::Kernel)?;
        Ok(stats
            .into_iter()
            .enumerate()
            .map(|(idx, stat)| {
                let action = if stat.dropped_packets > 0 {
                    "drop"
                } else {
                    "pass"
                };
                (idx as u32 + 1, action, stat.packets, stat.bytes)
            })
            .collect())
    }

    pub fn get_acl_rule_runtime_stats(&self) -> Result<Vec<AclRuleRuntimeStats>, AclQosError> {
        let stats = get_rule_stats(&self.maps, self.runtime).map_err(AclQosError::Kernel)?;
        let stats_by_key: HashMap<(u32, u32, u8, u8), _> = stats
            .into_iter()
            .map(|stat| {
                (
                    (
                        stat.key.src_id,
                        stat.key.dst_id,
                        stat.key.proto,
                        stat.key.direction,
                    ),
                    stat,
                )
            })
            .collect();

        let mut result_by_id: HashMap<String, AclRuleRuntimeStats> = HashMap::new();
        let mut ordered_ids = Vec::new();
        for rule in &self.state.rules {
            if rule.rule_id.trim().is_empty() {
                continue;
            }
            let key = (
                rule.src_group_id,
                rule.dst_group_id,
                rule.proto,
                rule.direction,
            );
            let Some(stat) = stats_by_key.get(&key) else {
                continue;
            };
            merge_acl_rule_runtime_stats(
                &mut result_by_id,
                &mut ordered_ids,
                &rule.rule_id,
                stat.packets,
                stat.bytes,
                stat.dropped_packets,
                stat.dropped_bytes,
            );
        }
        Ok(ordered_ids
            .into_iter()
            .filter_map(|id| result_by_id.remove(&id))
            .collect())
    }

    pub fn get_all_qos_stats(&self) -> Result<Vec<(&'static str, u32, u64, u64)>, AclQosError> {
        let stats = get_qos_stats(&self.maps, self.runtime).map_err(AclQosError::Kernel)?;
        Ok(stats
            .into_iter()
            .map(|stat| {
                (
                    "group",
                    stat.key.group_id,
                    stat.passed_bytes + stat.shaped_bytes,
                    stat.dropped_bytes,
                )
            })
            .collect())
    }

    pub fn get_qos_rule_runtime_stats(&self) -> Result<Vec<QosRuleRuntimeStats>, AclQosError> {
        let stats = get_qos_stats(&self.maps, self.runtime).map_err(AclQosError::Kernel)?;
        let stats_by_key: HashMap<(u32, u8), _> = stats
            .into_iter()
            .map(|stat| ((stat.key.group_id, stat.key.direction), stat))
            .collect();

        let mut result_by_id: HashMap<String, QosRuleRuntimeStats> = HashMap::new();
        let mut ordered_ids = Vec::new();
        for rule in &self.state.qos_rules {
            if rule.rule_id.trim().is_empty() {
                continue;
            }
            let key = (rule.group_id, rule.direction);
            let Some(stat) = stats_by_key.get(&key) else {
                continue;
            };
            merge_qos_rule_runtime_stats(
                &mut result_by_id,
                &mut ordered_ids,
                &rule.rule_id,
                stat.passed_bytes,
                stat.dropped_bytes,
                stat.shaped_bytes,
            );
        }
        Ok(ordered_ids
            .into_iter()
            .filter_map(|id| result_by_id.remove(&id))
            .collect())
    }

    fn ensure_snapshot_groups(&mut self, snapshot: &AclQosSnapshot) -> Result<(), AclQosError> {
        for rule in &snapshot.acl_rules {
            self.ensure_group(&rule.src_group)?;
            self.ensure_group(&rule.dst_group)?;
        }
        for rule in &snapshot.qos_rules {
            self.ensure_group(&rule.group)?;
        }
        Ok(())
    }

    fn apply_compiled_policy(&mut self, rule: &CompiledAclRule) -> Result<(), AclQosError> {
        let result = self
            .state
            .apply_add_rule(
                &rule.rule_id,
                rule.src_group_id,
                rule.dst_group_id,
                rule.proto,
                rule.action,
                rule.priority,
                rule.ports.as_deref(),
                rule.direction,
            )
            .map_err(AclQosError::Validation)?;
        add_policy_to_maps(
            &mut self.maps,
            self.runtime,
            rule.src_group_id,
            rule.dst_group_id,
            rule.proto,
            rule.action,
            rule.priority,
            rule.ports.as_deref(),
            result.bitmap_idx,
            result.is_new_port_set,
            rule.direction,
        )
        .map_err(AclQosError::Kernel)?;
        if let Some((idx, ports_normalized)) = result.old_port_set_released {
            let _ = delete_port_set_from_maps(&mut self.maps, self.runtime, idx, &ports_normalized);
        }
        Ok(())
    }

    fn apply_compiled_qos(&mut self, rule: &CompiledQosRule) -> Result<(), AclQosError> {
        self.state
            .qos_rules
            .retain(|r| !(r.group_id == rule.group_id && r.direction == rule.direction));
        self.state.qos_rules.push(QosRuleInfo {
            rule_id: rule.rule_id.clone(),
            group_name: rule.group_name.clone(),
            group_id: rule.group_id,
            direction: rule.direction,
            rate_bps: rule.rate_bps,
            burst_bytes: rule.burst_bytes,
            priority: rule.priority,
            mode: rule.mode,
        });
        add_qos_rule_to_maps(
            &mut self.maps,
            self.runtime,
            rule.group_id,
            rule.direction,
            rule.rate_bps,
            rule.burst_bytes,
            rule.priority,
            rule.mode,
            true,
        )
        .map_err(AclQosError::Kernel)?;
        Ok(())
    }

    fn clear_all_acl_rules(&mut self) -> Result<(), AclQosError> {
        let rules = self.state.rules.clone();
        for rule in rules {
            delete_policy_from_maps(
                &mut self.maps,
                self.runtime,
                rule.src_group_id,
                rule.dst_group_id,
                rule.proto,
                rule.direction,
            )
            .map_err(AclQosError::Kernel)?;
        }
        let port_sets = self.state.port_sets.clone();
        for port_set in port_sets.values() {
            let _ = delete_port_set_from_maps(
                &mut self.maps,
                self.runtime,
                port_set.bitmap_idx,
                &port_set.ports_normalized,
            );
        }
        self.state.rules.clear();
        self.state.port_sets.clear();
        self.state.free_bitmap_indices.clear();
        self.state.next_bitmap_idx = 0;
        Ok(())
    }

    fn clear_all_qos_rules(&mut self) -> Result<(), AclQosError> {
        let rules = self.state.qos_rules.clone();
        for rule in rules {
            delete_qos_rule_from_maps(
                &mut self.maps,
                self.runtime,
                rule.group_id,
                rule.direction,
                true,
            )
            .map_err(AclQosError::Kernel)?;
        }
        self.state.qos_rules.clear();
        Ok(())
    }

    fn ensure_group(&mut self, raw: &str) -> Result<u32, AclQosError> {
        let name = normalize_group_name(raw);
        if name == "any" {
            return Ok(ID_WILDCARD);
        }
        if let Some(group) = self.state.groups.get(&name) {
            return Ok(group.id);
        }

        let cidr = normalize_cidr(&name)?;
        let id = {
            let mut identity = self.identity_mgr.lock().map_err(|_| AclQosError::Lock)?;
            identity.assign_id(&cidr)?
        };
        self.state.groups.insert(
            name.clone(),
            GroupInfo {
                id,
                name: name.clone(),
                cidrs: vec![cidr],
            },
        );
        Ok(id)
    }
}

fn normalize_group_name(raw: &str) -> String {
    let trimmed = raw.trim();
    if trimmed.is_empty()
        || trimmed == "any"
        || trimmed == "0"
        || trimmed == "0.0.0.0/0"
        || trimmed == "::/0"
    {
        "any".to_string()
    } else {
        trimmed.to_string()
    }
}

fn normalize_cidr(raw: &str) -> Result<String, AclQosError> {
    let name = normalize_group_name(raw);
    if name == "any" {
        return Ok(name);
    }
    if name.contains('/') {
        return Ok(name);
    }
    let entry = parse_single_ip(&name)?;
    Ok(format!("{}/{}", entry.network, entry.prefix_len))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn acl_runtime_stats_merge_by_rule_id() {
        let mut stats = HashMap::new();
        let mut ordered_ids = Vec::new();

        merge_acl_rule_runtime_stats(&mut stats, &mut ordered_ids, "rule-a", 10, 1000, 2, 200);
        merge_acl_rule_runtime_stats(&mut stats, &mut ordered_ids, "rule-b", 1, 100, 0, 0);
        merge_acl_rule_runtime_stats(&mut stats, &mut ordered_ids, "rule-a", 5, 500, 1, 50);

        assert_eq!(
            ordered_ids,
            vec!["rule-a".to_string(), "rule-b".to_string()]
        );
        let merged = stats.get("rule-a").expect("rule-a stats");
        assert_eq!(merged.packets, 15);
        assert_eq!(merged.bytes, 1500);
        assert_eq!(merged.dropped_packets, 3);
        assert_eq!(merged.dropped_bytes, 250);
    }

    #[test]
    fn qos_runtime_stats_merge_by_rule_id() {
        let mut stats = HashMap::new();
        let mut ordered_ids = Vec::new();

        merge_qos_rule_runtime_stats(&mut stats, &mut ordered_ids, "qos-a", 1000, 100, 10);
        merge_qos_rule_runtime_stats(&mut stats, &mut ordered_ids, "qos-a", 500, 50, 5);

        assert_eq!(ordered_ids, vec!["qos-a".to_string()]);
        let merged = stats.get("qos-a").expect("qos-a stats");
        assert_eq!(merged.passed_bytes, 1500);
        assert_eq!(merged.dropped_bytes, 150);
        assert_eq!(merged.shaped_bytes, 15);
    }

    #[test]
    fn acl_compiler_expands_parent_cidr_to_existing_child_identity() {
        let groups = test_groups(&[("10.0.0.0/8", 1), ("10.1.1.1/32", 2)]);
        let rules = vec![
            AclRuleSpec {
                id: "deny-wide".to_string(),
                src_group: "10.0.0.0/8".to_string(),
                dst_group: "any".to_string(),
                proto: 1,
                action: 1,
                priority: 10,
                direction: 0,
                ports: None,
            },
            AclRuleSpec {
                id: "allow-specific-lower-priority".to_string(),
                src_group: "10.1.1.1/32".to_string(),
                dst_group: "any".to_string(),
                proto: 1,
                action: 0,
                priority: 100,
                direction: 0,
                ports: None,
            },
        ];

        let compiled = compile_acl_rules_for_groups(&rules, &groups).expect("compiled ACL");
        let child = compiled
            .iter()
            .find(|rule| rule.src_group_id == 2 && rule.dst_group_id == ID_WILDCARD)
            .expect("child runtime ACL entry");

        assert_eq!(child.rule_id, "deny-wide");
        assert_eq!(child.action, 1);
    }

    #[test]
    fn acl_compiler_prefers_specific_child_when_it_has_higher_priority() {
        let groups = test_groups(&[("10.0.0.0/8", 1), ("10.1.1.1/32", 2)]);
        let rules = vec![
            AclRuleSpec {
                id: "deny-wide".to_string(),
                src_group: "10.0.0.0/8".to_string(),
                dst_group: "any".to_string(),
                proto: 1,
                action: 1,
                priority: 10,
                direction: 0,
                ports: None,
            },
            AclRuleSpec {
                id: "allow-specific-higher-priority".to_string(),
                src_group: "10.1.1.1/32".to_string(),
                dst_group: "any".to_string(),
                proto: 1,
                action: 0,
                priority: 5,
                direction: 0,
                ports: None,
            },
        ];

        let compiled = compile_acl_rules_for_groups(&rules, &groups).expect("compiled ACL");
        let child = compiled
            .iter()
            .find(|rule| rule.src_group_id == 2 && rule.dst_group_id == ID_WILDCARD)
            .expect("child runtime ACL entry");

        assert_eq!(child.rule_id, "allow-specific-higher-priority");
        assert_eq!(child.action, 0);
    }

    #[test]
    fn qos_compiler_expands_parent_cidr_and_uses_priority_for_child_identity() {
        let groups = test_groups(&[("100.64.0.0/24", 1), ("100.64.0.2/32", 2)]);
        let rules = vec![
            QosRuleSpec {
                id: "wide-limit".to_string(),
                group: "100.64.0.0/24".to_string(),
                direction: 1,
                rate_bps: 1_000_000,
                burst_bytes: 1500,
                priority: 10,
                mode: 0,
            },
            QosRuleSpec {
                id: "specific-lower-priority".to_string(),
                group: "100.64.0.2/32".to_string(),
                direction: 1,
                rate_bps: 10_000_000,
                burst_bytes: 1500,
                priority: 100,
                mode: 0,
            },
        ];

        let compiled = compile_qos_rules_for_groups(&rules, &groups).expect("compiled QoS");
        let child = compiled
            .iter()
            .find(|rule| rule.group_id == 2 && rule.direction == 1)
            .expect("child runtime QoS entry");

        assert_eq!(child.rule_id, "wide-limit");
        assert_eq!(child.rate_bps, 1_000_000);
    }

    #[test]
    fn qos_compiler_prefers_specific_child_when_it_has_higher_priority() {
        let groups = test_groups(&[("100.64.0.0/24", 1), ("100.64.0.2/32", 2)]);
        let rules = vec![
            QosRuleSpec {
                id: "wide-limit".to_string(),
                group: "100.64.0.0/24".to_string(),
                direction: 1,
                rate_bps: 1_000_000,
                burst_bytes: 1500,
                priority: 10,
                mode: 0,
            },
            QosRuleSpec {
                id: "specific-higher-priority".to_string(),
                group: "100.64.0.2/32".to_string(),
                direction: 1,
                rate_bps: 10_000_000,
                burst_bytes: 1500,
                priority: 5,
                mode: 0,
            },
        ];

        let compiled = compile_qos_rules_for_groups(&rules, &groups).expect("compiled QoS");
        let child = compiled
            .iter()
            .find(|rule| rule.group_id == 2 && rule.direction == 1)
            .expect("child runtime QoS entry");

        assert_eq!(child.rule_id, "specific-higher-priority");
        assert_eq!(child.rate_bps, 10_000_000);
    }

    fn test_groups(entries: &[(&str, u32)]) -> HashMap<String, GroupInfo> {
        entries
            .iter()
            .map(|(cidr, id)| {
                let name = normalize_group_name(cidr);
                (
                    name.clone(),
                    GroupInfo {
                        id: *id,
                        name,
                        cidrs: vec![normalize_cidr(cidr).expect("test CIDR")],
                    },
                )
            })
            .collect()
    }
}
