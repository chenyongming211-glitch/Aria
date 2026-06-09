use std::sync::{Arc, Mutex as StdMutex};

use aya::Ebpf;
use thiserror::Error;

use crate::acl_qos_maps::{
    add_policy_to_maps, add_qos_rule_to_maps, cleanup_root_qdisc, delete_policy_from_maps,
    delete_port_set_from_maps, delete_qos_rule_from_maps, ensure_fq_qdisc, get_qos_stats,
    get_rule_stats, sync_runtime_config, AclQosMapHandles, TapMapRuntime,
};
use crate::acl_qos_state::{
    requested_directions, FirewallState, GroupInfo, QosRuleInfo,
};
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

#[derive(Debug, Clone)]
pub struct AclRuleSpec {
    pub src_group: String,
    pub dst_group: String,
    pub proto: u8,
    pub action: u8,
    pub direction: u8,
    pub ports: Option<String>,
}

#[derive(Debug, Clone)]
pub struct QosRuleSpec {
    pub group: String,
    pub direction: u8,
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    pub mode: u8,
}

#[derive(Debug, Clone, Default)]
pub struct AclQosSnapshot {
    pub acl_rules: Vec<AclRuleSpec>,
    pub qos_rules: Vec<QosRuleSpec>,
    pub acl_enabled: bool,
    pub qos_enabled: bool,
}

pub struct AclQosManager {
    maps: AclQosMapHandles,
    identity_mgr: Arc<StdMutex<IdentityManager>>,
    state: FirewallState,
    runtime: TapMapRuntime,
    interfaces: Vec<String>,
    _acl_ebpf: Ebpf,
    _qos_ebpf: Ebpf,
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
        self.clear_all_acl_rules()?;
        self.clear_all_qos_rules()?;

        self.state.acl_enabled = snapshot.acl_enabled;
        self.state.qos_enabled = snapshot.qos_enabled;

        for rule in snapshot.acl_rules {
            self.apply_policy_by_group(
                &rule.src_group,
                &rule.dst_group,
                rule.proto,
                rule.action,
                rule.ports.as_deref(),
                rule.direction,
            )?;
        }

        let mut has_shaping = false;
        for rule in snapshot.qos_rules {
            if rule.mode == 1 {
                has_shaping = true;
            }
            self.apply_qos_by_group(
                &rule.group,
                rule.direction,
                rule.rate_bps,
                rule.burst_bytes,
                rule.priority,
                rule.mode,
            )?;
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
        .map_err(AclQosError::Kernel)
    }

    pub fn get_all_rule_stats(&self) -> Result<Vec<(u32, &'static str, u64, u64)>, AclQosError> {
        let stats = get_rule_stats(&self.maps, self.runtime).map_err(AclQosError::Kernel)?;
        Ok(stats
            .into_iter()
            .enumerate()
            .map(|(idx, stat)| {
                let action = if stat.dropped_packets > 0 { "drop" } else { "pass" };
                (idx as u32 + 1, action, stat.packets, stat.bytes)
            })
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

    fn apply_policy_by_group(
        &mut self,
        src_group: &str,
        dst_group: &str,
        proto: u8,
        action: u8,
        ports: Option<&str>,
        direction: u8,
    ) -> Result<(), AclQosError> {
        let src_id = self.ensure_group(src_group)?;
        let dst_id = self.ensure_group(dst_group)?;
        for dir in requested_directions(direction) {
            let result = self
                .state
                .apply_add_rule(src_id, dst_id, proto, action, ports, dir)
                .map_err(AclQosError::Validation)?;
            add_policy_to_maps(
                &mut self.maps,
                self.runtime,
                src_id,
                dst_id,
                proto,
                action,
                ports,
                result.bitmap_idx,
                result.is_new_port_set,
                dir,
            )
            .map_err(AclQosError::Kernel)?;
            if let Some((idx, ports_normalized)) = result.old_port_set_released {
                let _ = delete_port_set_from_maps(&mut self.maps, self.runtime, idx, &ports_normalized);
            }
        }
        Ok(())
    }

    fn apply_qos_by_group(
        &mut self,
        group: &str,
        direction: u8,
        rate_bps: u64,
        burst_bytes: u64,
        priority: u8,
        mode: u8,
    ) -> Result<(), AclQosError> {
        let group_id = self.ensure_group(group)?;
        for dir in requested_directions(direction) {
            self.state
                .qos_rules
                .retain(|r| !(r.group_id == group_id && r.direction == dir));
            self.state.qos_rules.push(QosRuleInfo {
                group_name: normalize_group_name(group),
                group_id,
                direction: dir,
                rate_bps,
                burst_bytes,
                priority,
                mode,
            });
            add_qos_rule_to_maps(
                &mut self.maps,
                self.runtime,
                group_id,
                dir,
                rate_bps,
                burst_bytes,
                priority,
                mode,
                true,
            )
            .map_err(AclQosError::Kernel)?;
        }
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
