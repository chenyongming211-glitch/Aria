use std::collections::HashMap;
use std::net::IpAddr;
use std::sync::{Arc, Mutex as StdMutex};

use aya::Ebpf;
use thiserror::Error;

use crate::acl_qos_maps::{
    add_policy_to_maps, add_qos_rule_to_maps, cleanup_policy_generation, cleanup_root_qdisc,
    delete_policy_from_maps, delete_port_set_from_maps, delete_qos_rule_from_maps, ensure_fq_qdisc,
    get_qos_stats, get_rule_stats, sync_runtime_config, AclQosMapHandles, TapMapRuntime,
};
use crate::acl_qos_state::{
    requested_directions, FirewallState, GroupInfo, QosRuleInfo, DIRECTION_EGRESS,
};
use crate::identity::{parse_single_ip, IdentityManager, RuntimeIPGroup, ID_WILDCARD};

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
    pub src_group_id: String,
    pub dst_group_id: String,
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
    pub group_id: String,
    pub direction: u8,
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    pub mode: u8,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IPGroupSpec {
    pub id: String,
    pub name: String,
    pub cidrs: Vec<String>,
    pub kind: String,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct AclQosSnapshot {
    pub ip_groups: Vec<IPGroupSpec>,
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
    specificity: u16,
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
    specificity: u16,
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
        let src_groups = matching_groups(groups, group_selector(&rule.src_group_id, &rule.src_group))?;
        let dst_groups = matching_groups(groups, group_selector(&rule.dst_group_id, &rule.dst_group))?;
        for direction in requested_directions(rule.direction) {
            for src_group in &src_groups {
                for dst_group in &dst_groups {
                    let candidate = CompiledAclRule {
                        rule_id: rule.id.clone(),
                        src_group_id: src_group.id,
                        dst_group_id: dst_group.id,
                        proto: rule.proto,
                        action: rule.action,
                        priority: rule.priority,
                        specificity: src_group.specificity + dst_group.specificity,
                        direction,
                        ports: rule.ports.clone(),
                        order,
                    };
                    let key = (src_group.id, dst_group.id, rule.proto, direction);
                    match selected.get(&key) {
                        Some(existing) if acl_candidate_is_ambiguous(existing, &candidate) => {
                            return Err(AclQosError::Validation(format!(
                                "ambiguous ACL rules '{}' and '{}' for runtime key {:?}",
                                existing.rule_id, candidate.rule_id, key
                            )));
                        }
                        Some(existing) if acl_candidate_wins(existing, &candidate) => {
                            selected.insert(key, candidate);
                        }
                        None => {
                            selected.insert(key, candidate);
                        }
                        _ => {}
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
        let group_matches = matching_groups(groups, group_selector(&rule.group_id, &rule.group))?;
        for direction in requested_directions(rule.direction) {
            for group in &group_matches {
                let candidate = CompiledQosRule {
                    rule_id: rule.id.clone(),
                    group_name: group.name.clone(),
                    group_id: group.id,
                    direction,
                    rate_bps: rule.rate_bps,
                    burst_bytes: rule.burst_bytes,
                    priority: rule.priority,
                    specificity: group.specificity,
                    mode: rule.mode,
                    order,
                };
                let key = (group.id, direction);
                match selected.get(&key) {
                    Some(existing) if qos_candidate_is_ambiguous(existing, &candidate) => {
                        return Err(AclQosError::Validation(format!(
                            "ambiguous QoS rules '{}' and '{}' for runtime key {:?}",
                            existing.rule_id, candidate.rule_id, key
                        )));
                    }
                    Some(existing) if qos_candidate_wins(existing, &candidate) => {
                        selected.insert(key, candidate);
                    }
                    None => {
                        selected.insert(key, candidate);
                    }
                    _ => {}
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
        || (candidate.priority == existing.priority && candidate.specificity > existing.specificity)
}

fn qos_candidate_wins(existing: &CompiledQosRule, candidate: &CompiledQosRule) -> bool {
    candidate.priority < existing.priority
        || (candidate.priority == existing.priority && candidate.specificity > existing.specificity)
}

fn acl_candidate_is_ambiguous(existing: &CompiledAclRule, candidate: &CompiledAclRule) -> bool {
    existing.priority == candidate.priority
        && existing.specificity == candidate.specificity
        && (existing.rule_id != candidate.rule_id
            || existing.action != candidate.action
            || existing.ports != candidate.ports)
}

fn qos_candidate_is_ambiguous(existing: &CompiledQosRule, candidate: &CompiledQosRule) -> bool {
    existing.priority == candidate.priority
        && existing.specificity == candidate.specificity
        && (existing.rule_id != candidate.rule_id
            || existing.rate_bps != candidate.rate_bps
            || existing.burst_bytes != candidate.burst_bytes
            || existing.mode != candidate.mode)
}

fn group_selector<'a>(group_id: &'a str, fallback: &'a str) -> &'a str {
    let trimmed = group_id.trim();
    if trimmed.is_empty() {
        fallback
    } else {
        trimmed
    }
}

pub fn next_policy_generation(current: u32) -> u32 {
    match current {
        0 => 1,
        u32::MAX => 1,
        value => value + 1,
    }
}

fn compiled_qos_rule_requires_fq(rule: &CompiledQosRule) -> bool {
    rule.direction == DIRECTION_EGRESS
}

fn qos_rules_require_fq(rules: &[QosRuleInfo]) -> bool {
    rules.iter().any(|rule| rule.direction == DIRECTION_EGRESS)
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct GroupMatch {
    name: String,
    id: u32,
    specificity: u16,
}

fn matching_groups(groups: &HashMap<String, GroupInfo>, raw: &str) -> Result<Vec<GroupMatch>, AclQosError> {
    let name = normalize_group_name(raw);
    if name == "any" {
        return Ok(vec![GroupMatch {
            name: "any".to_string(),
            id: ID_WILDCARD,
            specificity: 0,
        }]);
    }

    if let Some(group) = groups.get(&name) {
        return matching_groups_for_cidrs(groups, &group.cidrs);
    }

    let cidr = normalize_cidr(&name)?;
    let parsed = parse_runtime_cidr(&cidr)?;
    matching_groups_for_parents(groups, &[(cidr, parsed.prefix_len)])
}

fn matching_groups_for_cidrs(
    groups: &HashMap<String, GroupInfo>,
    cidrs: &[String],
) -> Result<Vec<GroupMatch>, AclQosError> {
    let mut parents = Vec::new();
    for cidr in cidrs {
        let normalized = normalize_cidr(cidr)?;
        let parsed = parse_runtime_cidr(&normalized)?;
        parents.push((normalized, parsed.prefix_len));
    }
    matching_groups_for_parents(groups, &parents)
}

fn matching_groups_for_parents(
    groups: &HashMap<String, GroupInfo>,
    parents: &[(String, u8)],
) -> Result<Vec<GroupMatch>, AclQosError> {
    let mut matches: HashMap<u32, GroupMatch> = HashMap::new();
    for group in groups.values() {
        let mut best_specificity: Option<u8> = None;
        for group_cidr in &group.cidrs {
            for (parent_cidr, parent_prefix_len) in parents {
                if cidr_contains(parent_cidr, group_cidr)? {
                    best_specificity = Some(best_specificity.unwrap_or(0).max(*parent_prefix_len));
                }
            }
        }
        if let Some(specificity) = best_specificity {
            matches.insert(
                group.id,
                GroupMatch {
                    name: group.name.clone(),
                    id: group.id,
                    specificity: u16::from(specificity),
                },
            );
        }
    }
    let mut matches: Vec<_> = matches.into_values().collect();
    matches.sort_by_key(|group| group.id);
    if matches.is_empty() {
        return Err(AclQosError::Validation(format!(
            "no runtime group matches selector '{}'",
            parents
                .iter()
                .map(|(cidr, _)| cidr.as_str())
                .collect::<Vec<_>>()
                .join(",")
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
        let previous_runtime = self.runtime;
        let next_generation = next_policy_generation(previous_runtime.policy_generation);
        let next_runtime = previous_runtime.with_generation(next_generation);
        let identity_metadata = {
            let identity = self.identity_mgr.lock().map_err(|_| AclQosError::Lock)?;
            identity.metadata_snapshot()
        };

        let candidate_state = match self.apply_snapshot_generation(&snapshot, next_runtime) {
            Ok(state) => state,
            Err(error) => {
                let _ = cleanup_policy_generation(&mut self.maps, next_runtime);
                if let Ok(mut identity) = self.identity_mgr.lock() {
                    identity.cleanup_generation(next_generation);
                    identity.restore_metadata(identity_metadata);
                }
                return Err(error);
            }
        };

        let _ = cleanup_policy_generation(&mut self.maps, previous_runtime);
        if previous_runtime.policy_generation != next_runtime.policy_generation {
            if let Ok(mut identity) = self.identity_mgr.lock() {
                identity.cleanup_generation(previous_runtime.policy_generation);
            }
        }
        self.runtime = next_runtime;
        self.state = candidate_state;
        if !qos_rules_require_fq(&self.state.qos_rules) {
            for iface in &self.interfaces {
                let _ = cleanup_root_qdisc(iface);
            }
        }
        self.last_snapshot = Some(snapshot_cache);
        Ok(())
    }

    fn apply_snapshot_generation(
        &mut self,
        snapshot: &AclQosSnapshot,
        runtime: TapMapRuntime,
    ) -> Result<FirewallState, AclQosError> {
        let mut candidate_state = self.state.clone();
        self.ensure_snapshot_groups(&mut candidate_state, snapshot, runtime.policy_generation)?;
        let compiled_acl_rules =
            compile_acl_rules_for_groups(&snapshot.acl_rules, &candidate_state.groups)?;
        let compiled_qos_rules =
            compile_qos_rules_for_groups(&snapshot.qos_rules, &candidate_state.groups)?;

        Self::reset_policy_state(&mut candidate_state, snapshot, runtime.policy_generation);

        for rule in compiled_acl_rules {
            Self::apply_compiled_policy(
                &mut self.maps,
                runtime,
                &mut candidate_state,
                &rule,
            )?;
        }

        let mut requires_fq = false;
        for rule in compiled_qos_rules {
            if compiled_qos_rule_requires_fq(&rule) {
                requires_fq = true;
            }
            Self::apply_compiled_qos(&mut self.maps, runtime, &mut candidate_state, &rule)?;
        }

        if requires_fq {
            for iface in &self.interfaces {
                ensure_fq_qdisc(iface).map_err(AclQosError::Kernel)?;
            }
        }

        let qos_enabled = snapshot.qos_enabled && !candidate_state.qos_rules.is_empty();
        sync_runtime_config(
            &mut self.maps,
            runtime,
            Some(snapshot.acl_enabled),
            Some(qos_enabled),
        )
        .map_err(AclQosError::Kernel)?;

        Ok(candidate_state)
    }

    fn reset_policy_state(
        state: &mut FirewallState,
        snapshot: &AclQosSnapshot,
        policy_generation: u32,
    ) {
        state.rules.clear();
        state.port_sets.clear();
        state.free_bitmap_indices.clear();
        state.next_bitmap_idx = 0;
        state.qos_rules.clear();
        state.acl_enabled = snapshot.acl_enabled;
        state.qos_enabled = snapshot.qos_enabled;
        state.policy_generation = policy_generation;
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

    fn ensure_snapshot_groups(
        &mut self,
        state: &mut FirewallState,
        snapshot: &AclQosSnapshot,
        policy_generation: u32,
    ) -> Result<(), AclQosError> {
        self.replace_snapshot_ip_groups(state, policy_generation, &snapshot.ip_groups)?;
        for rule in &snapshot.acl_rules {
            if rule.src_group_id.trim().is_empty() {
                self.ensure_group(state, policy_generation, &rule.src_group)?;
            }
            if rule.dst_group_id.trim().is_empty() {
                self.ensure_group(state, policy_generation, &rule.dst_group)?;
            }
        }
        for rule in &snapshot.qos_rules {
            if rule.group_id.trim().is_empty() {
                self.ensure_group(state, policy_generation, &rule.group)?;
            }
        }
        Ok(())
    }

    fn replace_snapshot_ip_groups(
        &mut self,
        state: &mut FirewallState,
        policy_generation: u32,
        groups: &[IPGroupSpec],
    ) -> Result<(), AclQosError> {
        let runtime_groups: Vec<_> = groups
            .iter()
            .filter(|group| !group.id.trim().is_empty())
            .map(|group| RuntimeIPGroup {
                key: group.id.trim().to_string(),
                cidrs: group.cidrs.clone(),
            })
            .collect();

        let group_ids = {
            let mut identity = self.identity_mgr.lock().map_err(|_| AclQosError::Lock)?;
            identity.replace_groups_for_generation(policy_generation, &runtime_groups)?
        };
        let active_product_keys: std::collections::HashSet<String> =
            group_ids.keys().cloned().collect();
        let active_product_cidrs = normalized_product_group_cidrs(groups)?;
        state.groups.retain(|key, group| {
            if active_product_keys.contains(key) {
                return true;
            }
            is_legacy_group_key(key)
                && !group
                    .cidrs
                    .iter()
                    .any(|cidr| active_product_cidrs.contains(cidr))
        });

        for group in groups {
            let key = group.id.trim();
            if key.is_empty() {
                continue;
            }
            let id = *group_ids.get(key).ok_or_else(|| {
                AclQosError::Validation(format!("missing runtime id for IP group '{}'", key))
            })?;
            let cidrs = group
                .cidrs
                .iter()
                .map(|cidr| normalize_cidr(cidr))
                .collect::<Result<Vec<_>, _>>()?;
            state.groups.insert(
                key.to_string(),
                GroupInfo {
                    id,
                    name: key.to_string(),
                    cidrs,
                },
            );
        }
        Ok(())
    }

    fn apply_compiled_policy(
        maps: &mut AclQosMapHandles,
        runtime: TapMapRuntime,
        state: &mut FirewallState,
        rule: &CompiledAclRule,
    ) -> Result<(), AclQosError> {
        let result = state
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
            maps,
            runtime,
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
            let _ = delete_port_set_from_maps(maps, runtime, idx, &ports_normalized);
        }
        Ok(())
    }

    fn apply_compiled_qos(
        maps: &mut AclQosMapHandles,
        runtime: TapMapRuntime,
        state: &mut FirewallState,
        rule: &CompiledQosRule,
    ) -> Result<(), AclQosError> {
        state
            .qos_rules
            .retain(|r| !(r.group_id == rule.group_id && r.direction == rule.direction));
        state.qos_rules.push(QosRuleInfo {
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
            maps,
            runtime,
            rule.group_id,
            rule.direction,
            rule.rate_bps,
            rule.burst_bytes,
            rule.priority,
            rule.mode,
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

    fn ensure_group(
        &mut self,
        state: &mut FirewallState,
        policy_generation: u32,
        raw: &str,
    ) -> Result<u32, AclQosError> {
        let name = normalize_group_name(raw);
        if name == "any" {
            return Ok(ID_WILDCARD);
        }
        if let Some(group) = state.groups.get(&name) {
            return Ok(group.id);
        }

        let cidr = normalize_cidr(&name)?;
        let id = {
            let mut identity = self.identity_mgr.lock().map_err(|_| AclQosError::Lock)?;
            identity.assign_id_for_generation(policy_generation, &cidr)?
        };
        state.groups.insert(
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

fn is_legacy_group_key(key: &str) -> bool {
    let key = key.trim();
    key == "any" || normalize_cidr(key).is_ok()
}

fn normalized_product_group_cidrs(
    groups: &[IPGroupSpec],
) -> Result<std::collections::HashSet<String>, AclQosError> {
    let mut cidrs = std::collections::HashSet::new();
    for group in groups {
        if group.id.trim().is_empty() {
            continue;
        }
        for cidr in &group.cidrs {
            cidrs.insert(normalize_cidr(cidr)?);
        }
    }
    Ok(cidrs)
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
    fn policy_generation_advances_without_reusing_zero() {
        assert_eq!(next_policy_generation(0), 1);
        assert_eq!(next_policy_generation(1), 2);
        assert_eq!(next_policy_generation(u32::MAX), 1);
    }

    #[test]
    fn reset_policy_state_keeps_groups_but_replaces_policy_runtime() {
        let mut state = FirewallState::default();
        state.groups.insert(
            "office".to_string(),
            GroupInfo {
                id: 42,
                name: "office".to_string(),
                cidrs: vec!["100.64.0.2/32".to_string()],
            },
        );
        state
            .apply_add_rule("old", 42, ID_WILDCARD, 1, 1, 10, None, 0)
            .expect("old rule");
        state.qos_rules.push(QosRuleInfo {
            rule_id: "old-qos".to_string(),
            group_name: "office".to_string(),
            group_id: 42,
            direction: 1,
            rate_bps: 1_000_000,
            burst_bytes: 1500,
            priority: 100,
            mode: 0,
        });

        AclQosManager::reset_policy_state(
            &mut state,
            &AclQosSnapshot {
                acl_enabled: false,
                qos_enabled: true,
                ..Default::default()
            },
            9,
        );

        assert_eq!(state.groups.get("office").map(|group| group.id), Some(42));
        assert!(state.rules.is_empty());
        assert!(state.qos_rules.is_empty());
        assert!(!state.acl_enabled);
        assert!(state.qos_enabled);
        assert_eq!(state.policy_generation, 9);
    }

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
    fn only_egress_shaping_qos_rules_require_fq_for_edt_pacing() {
        assert!(!qos_rules_require_fq(&[]));
        assert!(!qos_rules_require_fq(&[QosRuleInfo {
            rule_id: "ingress".to_string(),
            group_name: "office".to_string(),
            group_id: 10,
            direction: 0,
            rate_bps: 5_000_000,
            burst_bytes: 625_000,
            priority: 100,
            mode: 0,
        }]));
        assert!(!qos_rules_require_fq(&[QosRuleInfo {
            rule_id: "egress-policing".to_string(),
            group_name: "office".to_string(),
            group_id: 10,
            direction: DIRECTION_EGRESS,
            rate_bps: 5_000_000,
            burst_bytes: 625_000,
            priority: 100,
            mode: 0,
        }]));
        assert!(qos_rules_require_fq(&[QosRuleInfo {
            rule_id: "egress-shaping".to_string(),
            group_name: "office".to_string(),
            group_id: 10,
            direction: DIRECTION_EGRESS,
            rate_bps: 5_000_000,
            burst_bytes: 625_000,
            priority: 100,
            mode: 1,
        }]));
    }

    #[test]
    fn acl_compiler_expands_parent_cidr_to_existing_child_identity() {
        let groups = test_groups(&[("10.0.0.0/8", 1), ("10.1.1.1/32", 2)]);
        let rules = vec![
            AclRuleSpec {
                id: "deny-wide".to_string(),
                src_group: "10.0.0.0/8".to_string(),
                dst_group: "any".to_string(),
                src_group_id: String::new(),
                dst_group_id: String::new(),
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
                src_group_id: String::new(),
                dst_group_id: String::new(),
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
                src_group_id: String::new(),
                dst_group_id: String::new(),
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
                src_group_id: String::new(),
                dst_group_id: String::new(),
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
                group_id: String::new(),
                direction: 1,
                rate_bps: 1_000_000,
                burst_bytes: 1500,
                priority: 10,
                mode: 0,
            },
            QosRuleSpec {
                id: "specific-lower-priority".to_string(),
                group: "100.64.0.2/32".to_string(),
                group_id: String::new(),
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
                group_id: String::new(),
                direction: 1,
                rate_bps: 1_000_000,
                burst_bytes: 1500,
                priority: 10,
                mode: 0,
            },
            QosRuleSpec {
                id: "specific-higher-priority".to_string(),
                group: "100.64.0.2/32".to_string(),
                group_id: String::new(),
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

    #[test]
    fn qos_product_group_with_multiple_cidrs_compiles_to_one_runtime_group_id() {
        let mut groups = HashMap::new();
        groups.insert(
            "office-id".to_string(),
            GroupInfo {
                id: 7,
                name: "office-id".to_string(),
                cidrs: vec![
                    "10.10.0.0/16".to_string(),
                    "192.168.1.0/24".to_string(),
                ],
            },
        );

        let rules = vec![QosRuleSpec {
            id: "qos-1".to_string(),
            group: String::new(),
            group_id: "office-id".to_string(),
            direction: 1,
            rate_bps: 10_000_000,
            burst_bytes: 1500,
            priority: 10,
            mode: 0,
        }];

        let compiled = compile_qos_rules_for_groups(&rules, &groups).expect("compile QoS");
        assert_eq!(compiled.len(), 1);
        assert_eq!(compiled[0].group_id, 7);
        assert_eq!(compiled[0].rule_id, "qos-1");
    }

    #[test]
    fn acl_equal_priority_prefers_more_specific_group() {
        let groups = test_groups(&[("10.0.0.0/8", 1), ("10.10.1.10/32", 2)]);
        let rules = vec![
            AclRuleSpec {
                id: "deny-wide".to_string(),
                src_group: "10.0.0.0/8".to_string(),
                dst_group: "any".to_string(),
                src_group_id: String::new(),
                dst_group_id: String::new(),
                proto: 1,
                action: 1,
                priority: 100,
                direction: 0,
                ports: None,
            },
            AclRuleSpec {
                id: "allow-specific".to_string(),
                src_group: "10.10.1.10/32".to_string(),
                dst_group: "any".to_string(),
                src_group_id: String::new(),
                dst_group_id: String::new(),
                proto: 1,
                action: 0,
                priority: 100,
                direction: 0,
                ports: None,
            },
        ];

        let compiled = compile_acl_rules_for_groups(&rules, &groups).expect("compile ACL");
        let child = compiled
            .iter()
            .find(|rule| rule.src_group_id == 2 && rule.dst_group_id == ID_WILDCARD)
            .expect("specific runtime ACL entry");
        assert_eq!(child.rule_id, "allow-specific");
        assert_eq!(child.action, 0);
    }

    #[test]
    fn qos_equal_priority_prefers_more_specific_group() {
        let groups = test_groups(&[("100.64.0.0/24", 1), ("100.64.0.3/32", 2)]);
        let rules = vec![
            QosRuleSpec {
                id: "wide-limit".to_string(),
                group: "100.64.0.0/24".to_string(),
                group_id: String::new(),
                direction: 1,
                rate_bps: 1_000_000,
                burst_bytes: 1500,
                priority: 100,
                mode: 0,
            },
            QosRuleSpec {
                id: "specific-limit".to_string(),
                group: "100.64.0.3/32".to_string(),
                group_id: String::new(),
                direction: 1,
                rate_bps: 10_000_000,
                burst_bytes: 1500,
                priority: 100,
                mode: 0,
            },
        ];

        let compiled = compile_qos_rules_for_groups(&rules, &groups).expect("compile QoS");
        let child = compiled
            .iter()
            .find(|rule| rule.group_id == 2 && rule.direction == 1)
            .expect("specific runtime QoS entry");
        assert_eq!(child.rule_id, "specific-limit");
        assert_eq!(child.rate_bps, 10_000_000);
    }

    #[test]
    fn acl_equal_priority_equal_specificity_is_rejected() {
        let groups = test_groups(&[("10.10.1.10/32", 1)]);
        let rules = vec![
            AclRuleSpec {
                id: "deny-a".to_string(),
                src_group: "10.10.1.10/32".to_string(),
                dst_group: "any".to_string(),
                src_group_id: String::new(),
                dst_group_id: String::new(),
                proto: 1,
                action: 1,
                priority: 100,
                direction: 0,
                ports: None,
            },
            AclRuleSpec {
                id: "allow-b".to_string(),
                src_group: "10.10.1.10/32".to_string(),
                dst_group: "any".to_string(),
                src_group_id: String::new(),
                dst_group_id: String::new(),
                proto: 1,
                action: 0,
                priority: 100,
                direction: 0,
                ports: None,
            },
        ];

        let err = compile_acl_rules_for_groups(&rules, &groups).expect_err("ambiguous ACL");
        assert!(format!("{err}").contains("ambiguous ACL rules"));
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
