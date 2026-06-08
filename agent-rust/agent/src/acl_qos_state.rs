use std::collections::HashMap;

use serde::{Deserialize, Serialize};

pub const DIRECTION_INGRESS: u8 = 0;
pub const DIRECTION_EGRESS: u8 = 1;
pub const DIRECTION_BOTH: u8 = 2;

pub const ACTION_ALLOW: u8 = 0;
pub const ACTION_DROP: u8 = 1;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct GroupInfo {
    pub id: u32,
    pub name: String,
    pub cidrs: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct RuleInfo {
    pub name: Option<String>,
    pub src_group_id: u32,
    pub dst_group_id: u32,
    pub proto: u8,
    pub action: u8,
    pub ports: Option<String>,
    pub bitmap_idx: Option<u32>,
    #[serde(default)]
    pub direction: u8,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct QosRuleInfo {
    pub group_name: String,
    pub group_id: u32,
    pub direction: u8,
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    #[serde(default)]
    pub mode: u8,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct PortSetInfo {
    pub bitmap_idx: u32,
    pub ports_normalized: String,
    pub ref_count: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FirewallState {
    pub groups: HashMap<String, GroupInfo>,
    pub rules: Vec<RuleInfo>,
    pub next_group_id: u32,
    pub next_bitmap_idx: u32,
    #[serde(default)]
    pub port_sets: HashMap<String, PortSetInfo>,
    #[serde(default)]
    pub free_bitmap_indices: Vec<u32>,
    #[serde(default = "default_max_port_policies")]
    pub max_port_policies: u32,
    #[serde(default)]
    pub tap_id: u32,
    #[serde(default)]
    pub attached_iface: Option<String>,
    #[serde(default)]
    pub qos_rules: Vec<QosRuleInfo>,
    #[serde(default = "default_true")]
    pub acl_enabled: bool,
    #[serde(default = "default_true")]
    pub qos_enabled: bool,
}

impl Default for FirewallState {
    fn default() -> Self {
        Self {
            groups: HashMap::new(),
            rules: Vec::new(),
            next_group_id: 1,
            next_bitmap_idx: 0,
            port_sets: HashMap::new(),
            free_bitmap_indices: Vec::new(),
            max_port_policies: default_max_port_policies(),
            tap_id: 0,
            attached_iface: None,
            qos_rules: Vec::new(),
            acl_enabled: true,
            qos_enabled: true,
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AddRuleResult {
    pub bitmap_idx: Option<u32>,
    pub is_new_port_set: bool,
    pub old_port_set_released: Option<(u32, String)>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RemoveRuleResult {
    pub bitmap_idx: Option<u32>,
    pub port_set_released: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub enum AclQosWalEntry {
    AddRule {
        src_id: u32,
        dst_id: u32,
        proto: u8,
        action: u8,
        ports: Option<String>,
        direction: u8,
    },
    RemoveRule {
        src_id: u32,
        dst_id: u32,
        proto: u8,
        direction: u8,
    },
    AddQos {
        group_name: String,
        group_id: u32,
        direction: u8,
        rate_bps: u64,
        burst_bytes: u64,
        priority: u8,
        #[serde(default)]
        mode: u8,
    },
    DeleteQos {
        group_id: u32,
        direction: u8,
    },
    UpdateConfig {
        acl: Option<bool>,
        qos: Option<bool>,
    },
}

fn default_max_port_policies() -> u32 {
    16_384
}

fn default_true() -> bool {
    true
}

pub fn direction_from_string(direction: &str) -> Result<u8, String> {
    match direction.trim().to_ascii_lowercase().as_str() {
        "ingress" | "in" => Ok(DIRECTION_INGRESS),
        "egress" | "out" => Ok(DIRECTION_EGRESS),
        "both" | "all" => Ok(DIRECTION_BOTH),
        other => Err(format!(
            "invalid direction '{}': must be ingress, egress, or both",
            other
        )),
    }
}

pub fn direction_to_string(direction: u8) -> String {
    match direction {
        DIRECTION_INGRESS => "ingress".to_string(),
        DIRECTION_EGRESS => "egress".to_string(),
        DIRECTION_BOTH => "both".to_string(),
        other => other.to_string(),
    }
}

pub fn requested_directions(direction: u8) -> Vec<u8> {
    if direction == DIRECTION_BOTH {
        vec![DIRECTION_INGRESS, DIRECTION_EGRESS]
    } else {
        vec![direction]
    }
}

pub fn normalize_ports(ports_str: &str, default_action: u8) -> Result<String, String> {
    let mut entries: Vec<(u16, u16, u8)> = Vec::new();
    for raw_part in ports_str.split(',') {
        let part = raw_part.trim();
        if part.is_empty() {
            continue;
        }

        let parts: Vec<&str> = part.split(':').collect();
        if parts.len() > 2 {
            return Err(format!("invalid port filter '{}'", part));
        }

        let action = match parts.get(1) {
            Some(raw_action) => {
                let parsed = raw_action
                    .trim()
                    .parse::<u8>()
                    .map_err(|_| format!("invalid action '{}': must be 0 or 1", raw_action))?;
                if parsed > ACTION_DROP {
                    return Err(format!("invalid action {}: must be 0 or 1", parsed));
                }
                parsed
            }
            None => default_action,
        };

        let range = parts[0].trim();
        if range.contains('-') {
            let bounds: Vec<&str> = range.split('-').collect();
            if bounds.len() != 2 {
                return Err(format!("invalid port range '{}'", range));
            }
            let start = bounds[0]
                .trim()
                .parse::<u16>()
                .map_err(|_| format!("invalid port '{}'", bounds[0]))?;
            let end = bounds[1]
                .trim()
                .parse::<u16>()
                .map_err(|_| format!("invalid port '{}'", bounds[1]))?;
            if start > end {
                return Err(format!("invalid port range: {}-{}", start, end));
            }
            entries.push((start, end, action));
        } else {
            let port = range
                .parse::<u16>()
                .map_err(|_| format!("invalid port '{}'", range))?;
            entries.push((port, port, action));
        }
    }

    entries.sort();
    Ok(entries
        .iter()
        .map(|(start, end, action)| {
            if start == end {
                format!("{}:{}", start, action)
            } else {
                format!("{}-{}:{}", start, end, action)
            }
        })
        .collect::<Vec<_>>()
        .join(","))
}

fn release_port_set(
    port_sets: &mut HashMap<String, PortSetInfo>,
    free_indices: &mut Vec<u32>,
    bitmap_idx: u32,
) {
    let key_to_remove = port_sets
        .iter()
        .find(|(_, ps)| ps.bitmap_idx == bitmap_idx)
        .map(|(key, _)| key.clone());

    if let Some(key) = key_to_remove {
        if let Some(ps) = port_sets.get_mut(&key) {
            ps.ref_count = ps.ref_count.saturating_sub(1);
            if ps.ref_count == 0 {
                free_indices.push(bitmap_idx);
                port_sets.remove(&key);
            }
        }
    }
}

impl FirewallState {
    pub fn apply_add_rule(
        &mut self,
        src_group_id: u32,
        dst_group_id: u32,
        proto: u8,
        action: u8,
        ports: Option<&str>,
        direction: u8,
    ) -> Result<AddRuleResult, String> {
        let mut result = AddRuleResult {
            bitmap_idx: None,
            is_new_port_set: false,
            old_port_set_released: None,
        };

        let stored_ports = ports.map(|p| {
            let trimmed = p.trim();
            if trimmed.eq_ignore_ascii_case("all") {
                "all".to_string()
            } else {
                trimmed.to_string()
            }
        });

        let (bitmap_idx, is_new) = if let Some(p) = stored_ports
            .as_deref()
            .filter(|p| !p.is_empty() && !p.eq_ignore_ascii_case("all"))
        {
            let normalized = normalize_ports(p, action)?;
            if let Some(existing) = self.port_sets.get_mut(&normalized) {
                existing.ref_count += 1;
                (Some(existing.bitmap_idx), false)
            } else {
                let idx = if let Some(recycled) = self.free_bitmap_indices.pop() {
                    recycled
                } else {
                    if self.next_bitmap_idx >= self.max_port_policies {
                        return Err(format!(
                            "port set limit ({}) reached",
                            self.max_port_policies
                        ));
                    }
                    let idx = self.next_bitmap_idx;
                    self.next_bitmap_idx += 1;
                    idx
                };

                self.port_sets.insert(
                    normalized.clone(),
                    PortSetInfo {
                        bitmap_idx: idx,
                        ports_normalized: normalized,
                        ref_count: 1,
                    },
                );
                (Some(idx), true)
            }
        } else {
            (None, false)
        };

        if let Some(existing) = self.rules.iter_mut().find(|r| {
            r.src_group_id == src_group_id
                && r.dst_group_id == dst_group_id
                && r.proto == proto
                && r.direction == direction
        }) {
            if let Some(old_idx) = existing.bitmap_idx {
                if bitmap_idx != Some(old_idx) {
                    let old_ports_normalized = self
                        .port_sets
                        .iter()
                        .find(|(_, ps)| ps.bitmap_idx == old_idx)
                        .map(|(_, ps)| ps.ports_normalized.clone());
                    release_port_set(&mut self.port_sets, &mut self.free_bitmap_indices, old_idx);
                    if self.free_bitmap_indices.contains(&old_idx) {
                        if let Some(ports_norm) = old_ports_normalized {
                            result.old_port_set_released = Some((old_idx, ports_norm));
                        }
                    }
                } else if let Some(key) = self
                    .port_sets
                    .iter()
                    .find(|(_, ps)| ps.bitmap_idx == old_idx)
                    .map(|(key, _)| key.clone())
                {
                    if let Some(ps) = self.port_sets.get_mut(&key) {
                        ps.ref_count = ps.ref_count.saturating_sub(1);
                    }
                }
            }
            existing.action = action;
            existing.ports = stored_ports;
            existing.bitmap_idx = bitmap_idx;
        } else {
            self.rules.push(RuleInfo {
                name: None,
                src_group_id,
                dst_group_id,
                proto,
                action,
                ports: stored_ports,
                bitmap_idx,
                direction,
            });
        }

        result.bitmap_idx = bitmap_idx;
        result.is_new_port_set = is_new;
        Ok(result)
    }

    pub fn apply_remove_rule(
        &mut self,
        src_group_id: u32,
        dst_group_id: u32,
        proto: u8,
        direction: u8,
    ) -> Result<RemoveRuleResult, String> {
        let Some(pos) = self.rules.iter().position(|r| {
            r.src_group_id == src_group_id
                && r.dst_group_id == dst_group_id
                && r.proto == proto
                && r.direction == direction
        }) else {
            return Err(format!(
                "policy not found: src_id={}, dst_id={}, proto={}, direction={}",
                src_group_id, dst_group_id, proto, direction
            ));
        };

        let rule = self.rules.remove(pos);
        let mut result = RemoveRuleResult {
            bitmap_idx: rule.bitmap_idx,
            port_set_released: None,
        };

        if let Some(idx) = rule.bitmap_idx {
            let ports_normalized = self
                .port_sets
                .iter()
                .find(|(_, ps)| ps.bitmap_idx == idx)
                .map(|(_, ps)| ps.ports_normalized.clone());
            release_port_set(&mut self.port_sets, &mut self.free_bitmap_indices, idx);
            if self.free_bitmap_indices.contains(&idx) {
                result.port_set_released = ports_normalized;
            }
        }

        Ok(result)
    }
}

pub fn apply_acl_qos_wal_entry(state: &mut FirewallState, entry: AclQosWalEntry) -> bool {
    match entry {
        AclQosWalEntry::AddRule {
            src_id,
            dst_id,
            proto,
            action,
            ports,
            direction,
        } => state
            .apply_add_rule(src_id, dst_id, proto, action, ports.as_deref(), direction)
            .is_ok(),
        AclQosWalEntry::RemoveRule {
            src_id,
            dst_id,
            proto,
            direction,
        } => state.apply_remove_rule(src_id, dst_id, proto, direction).is_ok(),
        AclQosWalEntry::AddQos {
            group_name,
            group_id,
            direction,
            rate_bps,
            burst_bytes,
            priority,
            mode,
        } => {
            state
                .qos_rules
                .retain(|r| !(r.group_id == group_id && r.direction == direction));
            state.qos_rules.push(QosRuleInfo {
                group_name,
                group_id,
                direction,
                rate_bps,
                burst_bytes,
                priority,
                mode,
            });
            true
        }
        AclQosWalEntry::DeleteQos {
            group_id,
            direction,
        } => {
            state
                .qos_rules
                .retain(|r| !(r.group_id == group_id && r.direction == direction));
            true
        }
        AclQosWalEntry::UpdateConfig { acl, qos } => {
            if let Some(enabled) = acl {
                state.acl_enabled = enabled;
            }
            if let Some(enabled) = qos {
                state.qos_enabled = enabled;
            }
            true
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn port_sets_are_reused_and_ref_counted() {
        let mut state = FirewallState::default();

        let first = state
            .apply_add_rule(1, 2, 6, ACTION_DROP, Some("80-82,443:0"), DIRECTION_INGRESS)
            .expect("first rule");
        let second = state
            .apply_add_rule(3, 4, 6, ACTION_DROP, Some("443:0,80-82:1"), DIRECTION_INGRESS)
            .expect("second rule");

        assert_eq!(first.bitmap_idx, second.bitmap_idx);
        assert!(first.is_new_port_set);
        assert!(!second.is_new_port_set);

        let normalized = normalize_ports("80-82,443:0", ACTION_DROP).unwrap();
        assert_eq!(state.port_sets.get(&normalized).unwrap().ref_count, 2);
    }

    #[test]
    fn remove_rule_releases_last_port_set_reference() {
        let mut state = FirewallState::default();
        let added = state
            .apply_add_rule(1, 2, 6, ACTION_ALLOW, Some("443"), DIRECTION_INGRESS)
            .expect("add rule");

        let removed = state
            .apply_remove_rule(1, 2, 6, DIRECTION_INGRESS)
            .expect("remove rule");

        assert_eq!(removed.bitmap_idx, added.bitmap_idx);
        assert_eq!(removed.port_set_released.as_deref(), Some("443:0"));
        assert!(state.port_sets.is_empty());
        assert_eq!(state.free_bitmap_indices, vec![added.bitmap_idx.unwrap()]);
    }

    #[test]
    fn both_direction_expands_to_ingress_and_egress() {
        assert_eq!(direction_from_string("both").unwrap(), DIRECTION_BOTH);
        assert_eq!(
            requested_directions(DIRECTION_BOTH),
            vec![DIRECTION_INGRESS, DIRECTION_EGRESS]
        );
    }

    #[test]
    fn wal_replays_qos_and_runtime_config() {
        let mut state = FirewallState::default();

        assert!(apply_acl_qos_wal_entry(
            &mut state,
            AclQosWalEntry::AddQos {
                group_name: "cidr:10.0.0.0/24".to_string(),
                group_id: 7,
                direction: DIRECTION_EGRESS,
                rate_bps: 100_000_000,
                burst_bytes: 1_000_000,
                priority: 3,
                mode: 1,
            },
        ));
        assert_eq!(state.qos_rules.len(), 1);

        assert!(apply_acl_qos_wal_entry(
            &mut state,
            AclQosWalEntry::UpdateConfig {
                acl: Some(false),
                qos: Some(true),
            },
        ));
        assert!(!state.acl_enabled);
        assert!(state.qos_enabled);
    }
}
