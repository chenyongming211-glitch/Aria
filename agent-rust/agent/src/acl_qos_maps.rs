use aria_shared::{
    FirewallConfig, PolicyKey, PolicyValue, PortKey, QosConfig, QosKey, QosStatsValue,
    RuleStatsValue, TapConfig, TokenBucket, TAP_ID_UNASSIGNED,
};
use aya::maps::{HashMap as AyaHashMap, MapData, PerCpuHashMap, PerCpuValues};
use aya::util::nr_cpus;
use aya::Ebpf;
use aya::Pod;

pub const ACTION_ALLOW: u8 = 0;
pub const ACTION_DROP: u8 = 1;
pub const PORT_ACTION_DROP: u8 = 1;
pub const PORT_ACTION_PASS: u8 = 2;

#[derive(Debug, Clone, Copy)]
pub struct TapMapRuntime {
    pub tap_id: u32,
    pub policy_generation: u32,
}

impl Default for TapMapRuntime {
    fn default() -> Self {
        Self {
            tap_id: TAP_ID_UNASSIGNED,
            policy_generation: 0,
        }
    }
}

impl TapMapRuntime {
    pub fn with_generation(self, policy_generation: u32) -> Self {
        Self {
            policy_generation,
            ..self
        }
    }
}

pub struct AclQosMapHandles {
    policy_table: AyaHashMap<MapData, PolicyKey, PolicyValue>,
    port_bitmap_pool: AyaHashMap<MapData, PortKey, u8>,
    rule_stats: PerCpuHashMap<MapData, PolicyKey, RuleStatsValue>,
    acl_firewall_config: AyaHashMap<MapData, u32, FirewallConfig>,
    acl_tap_config: AyaHashMap<MapData, u32, TapConfig>,
    qos_config: AyaHashMap<MapData, QosKey, QosConfig>,
    qos_token_bucket: AyaHashMap<MapData, QosKey, TokenBucket>,
    qos_stats: PerCpuHashMap<MapData, QosKey, QosStatsValue>,
    qos_firewall_config: AyaHashMap<MapData, u32, FirewallConfig>,
    qos_tap_config: AyaHashMap<MapData, u32, TapConfig>,
}

impl AclQosMapHandles {
    pub fn new(acl_ebpf: &mut Ebpf, qos_ebpf: &mut Ebpf) -> Result<Self, String> {
        Ok(Self {
            policy_table: take_hash_map(acl_ebpf, "POLICY_TABLE")?,
            port_bitmap_pool: take_hash_map(acl_ebpf, "PORT_BITMAP_POOL")?,
            rule_stats: take_per_cpu_hash_map(acl_ebpf, "RULE_STATS")?,
            acl_firewall_config: take_hash_map(acl_ebpf, "FIREWALL_CONFIG")?,
            acl_tap_config: take_hash_map(acl_ebpf, "TAP_CONFIG_MAP")?,
            qos_config: take_hash_map(qos_ebpf, "QOS_CONFIG")?,
            qos_token_bucket: take_hash_map(qos_ebpf, "QOS_TOKEN_BUCKET")?,
            qos_stats: take_per_cpu_hash_map(qos_ebpf, "QOS_STATS")?,
            qos_firewall_config: take_hash_map(qos_ebpf, "FIREWALL_CONFIG")?,
            qos_tap_config: take_hash_map(qos_ebpf, "TAP_CONFIG_MAP")?,
        })
    }
}

fn take_hash_map<K, V>(ebpf: &mut Ebpf, name: &str) -> Result<AyaHashMap<MapData, K, V>, String>
where
    K: aya::Pod,
    V: aya::Pod,
{
    ebpf.take_map(name)
        .ok_or_else(|| format!("map not found: {}", name))?
        .try_into()
        .map_err(|e| format!("convert {} to HashMap: {:?}", name, e))
}

fn take_per_cpu_hash_map<K, V>(
    ebpf: &mut Ebpf,
    name: &str,
) -> Result<PerCpuHashMap<MapData, K, V>, String>
where
    K: aya::Pod,
    V: aya::Pod,
{
    ebpf.take_map(name)
        .ok_or_else(|| format!("map not found: {}", name))?
        .try_into()
        .map_err(|e| format!("convert {} to PerCpuHashMap: {:?}", name, e))
}

fn zero_per_cpu_values<V>() -> Result<PerCpuValues<V>, String>
where
    V: Pod + Default + Copy,
{
    let cpus = nr_cpus().map_err(|(_, error)| format!("nr_cpus: {}", error))?;
    PerCpuValues::try_from(vec![V::default(); cpus])
        .map_err(|error| format!("per-cpu zero values: {}", error))
}

pub fn stored_policy_action(action: u8, has_port_filter: bool) -> u8 {
    if has_port_filter {
        match action {
            ACTION_ALLOW => ACTION_DROP,
            ACTION_DROP => ACTION_ALLOW,
            _ => action,
        }
    } else {
        action
    }
}

fn encode_port_action(action: u8) -> Result<u8, String> {
    match action {
        ACTION_ALLOW => Ok(PORT_ACTION_PASS),
        ACTION_DROP => Ok(PORT_ACTION_DROP),
        _ => Err(format!("invalid ACL action {}: must be 0 or 1", action)),
    }
}

pub fn parse_ports(ports_str: &str, default_action: u8) -> Result<Vec<(u16, u16, u8)>, String> {
    let default_bpf_action = encode_port_action(default_action)?;
    let mut rules = Vec::new();

    for raw in ports_str.split(',') {
        let part = raw.trim();
        if part.is_empty() {
            continue;
        }

        let parts: Vec<&str> = part.split(':').collect();
        if parts.len() > 2 {
            return Err(format!("invalid port filter '{}'", part));
        }
        let rule_action = match parts.get(1) {
            Some(raw_action) => encode_port_action(
                raw_action
                    .trim()
                    .parse::<u8>()
                    .map_err(|_| format!("invalid port action '{}'", raw_action))?,
            )?,
            None => default_bpf_action,
        };

        let range = parts[0].trim();
        if let Some((start, end)) = range.split_once('-') {
            let start = start
                .trim()
                .parse::<u16>()
                .map_err(|_| format!("invalid port '{}'", start))?;
            let end = end
                .trim()
                .parse::<u16>()
                .map_err(|_| format!("invalid port '{}'", end))?;
            if start > end {
                return Err(format!("invalid port range {}-{}", start, end));
            }
            rules.push((start, end, rule_action));
        } else {
            let port = range
                .parse::<u16>()
                .map_err(|_| format!("invalid port '{}'", range))?;
            rules.push((port, port, rule_action));
        }
    }

    Ok(rules)
}

pub fn validate_policy_ports(proto: u8, ports: Option<&str>) -> Result<(), String> {
    let Some(ports) = ports else {
        return Ok(());
    };
    let ports = ports.trim();
    if ports.is_empty() || ports.eq_ignore_ascii_case("all") {
        return Ok(());
    }

    match proto {
        6 | 17 => Ok(()),
        0 => Err("port filters require tcp or udp protocol".to_string()),
        other => Err(format!("protocol {} does not support port filters", other)),
    }
}

pub fn add_policy_to_maps(
    handles: &mut AclQosMapHandles,
    runtime: TapMapRuntime,
    src_id: u32,
    dst_id: u32,
    proto: u8,
    action: u8,
    priority: u16,
    ports: Option<&str>,
    bitmap_idx: Option<u32>,
    is_new_port_set: bool,
    direction: u8,
) -> Result<(), String> {
    validate_policy_ports(proto, ports)?;

    let is_all_ports = ports
        .map(|p| {
            let p = p.trim();
            p.is_empty() || p.eq_ignore_ascii_case("all")
        })
        .unwrap_or(true);
    let has_port_filter = ports.is_some() && !is_all_ports;

    if is_new_port_set {
        let idx =
            bitmap_idx.ok_or_else(|| "new port set requested without bitmap_idx".to_string())?;
        if let Some(ports_str) = ports {
            for (start, end, rule_action) in parse_ports(ports_str, action)? {
                for port in start..=end {
                    let key = PortKey {
                        tap_id: runtime.tap_id,
                        generation: runtime.policy_generation,
                        idx,
                        port,
                        pad: 0,
                    };
                    handles
                        .port_bitmap_pool
                        .insert(key, rule_action, 0)
                        .map_err(|e| format!("PORT_BITMAP_POOL insert: {:?}", e))?;
                }
            }
        }
    }

    let key = PolicyKey {
        tap_id: runtime.tap_id,
        generation: runtime.policy_generation,
        src_id,
        dst_id,
        proto,
        direction,
        pad: [0; 2],
    };
    let value = PolicyValue {
        action: stored_policy_action(action, has_port_filter),
        has_port_filter: has_port_filter as u8,
        priority,
        bitmap_idx: bitmap_idx.unwrap_or(0),
    };
    handles
        .policy_table
        .insert(key, value, 0)
        .map_err(|e| format!("POLICY_TABLE insert: {:?}", e))?;
    handles
        .rule_stats
        .insert(key, zero_per_cpu_values::<RuleStatsValue>()?, 0)
        .map_err(|e| format!("RULE_STATS insert: {:?}", e))
}

pub fn delete_port_set_from_maps(
    handles: &mut AclQosMapHandles,
    runtime: TapMapRuntime,
    bitmap_idx: u32,
    ports_normalized: &str,
) -> Result<(), String> {
    for (start, end, _) in parse_ports(ports_normalized, ACTION_ALLOW)? {
        for port in start..=end {
            let key = PortKey {
                tap_id: runtime.tap_id,
                generation: runtime.policy_generation,
                idx: bitmap_idx,
                port,
                pad: 0,
            };
            let _ = handles.port_bitmap_pool.remove(&key);
        }
    }
    Ok(())
}

pub fn add_qos_rule_to_maps(
    handles: &mut AclQosMapHandles,
    runtime: TapMapRuntime,
    group_id: u32,
    direction: u8,
    rate_bps: u64,
    burst_bytes: u64,
    priority: u8,
    mode: u8,
) -> Result<(), String> {
    let key = QosKey {
        tap_id: runtime.tap_id,
        generation: runtime.policy_generation,
        group_id,
        direction,
        pad: [0; 3],
    };
    let config = QosConfig {
        rate_bps,
        burst_bytes,
        priority,
        mode,
        pad: [0; 6],
    };

    let current = handles.qos_config.get(&key, 0).ok();
    if current != Some(config) {
        let _ = handles.qos_token_bucket.remove(&key);
        handles
            .qos_config
            .insert(key, config, 0)
            .map_err(|e| format!("QOS_CONFIG insert: {:?}", e))?;
        handles
            .qos_stats
            .insert(key, zero_per_cpu_values::<QosStatsValue>()?, 0)
            .map_err(|e| format!("QOS_STATS insert: {:?}", e))?;
    }
    Ok(())
}

pub fn sync_runtime_config(
    handles: &mut AclQosMapHandles,
    runtime: TapMapRuntime,
    acl_enabled: Option<bool>,
    qos_enabled: Option<bool>,
) -> Result<(), String> {
    if runtime.tap_id == TAP_ID_UNASSIGNED {
        let key = 0u32;
        let previous_acl = handles.acl_firewall_config.get(&key, 0).ok();
        let previous_qos = handles.qos_firewall_config.get(&key, 0).ok();
        let acl_cfg = merge_firewall_config(previous_acl, runtime, acl_enabled, qos_enabled);
        let qos_cfg = merge_firewall_config(previous_qos, runtime, acl_enabled, qos_enabled);

        handles
            .acl_firewall_config
            .insert(key, acl_cfg, 0)
            .map_err(|e| format!("ACL FIREWALL_CONFIG insert: {:?}", e))?;
        if let Err(error) = handles.qos_firewall_config.insert(key, qos_cfg, 0) {
            restore_firewall_config(&mut handles.acl_firewall_config, key, previous_acl)
                .map_err(|rollback| {
                    format!(
                        "QoS FIREWALL_CONFIG insert: {:?}; ACL rollback failed: {}",
                        error, rollback
                    )
                })?;
            return Err(format!("QoS FIREWALL_CONFIG insert: {:?}", error));
        }
        return Ok(());
    }

    let previous_acl = handles.acl_tap_config.get(&runtime.tap_id, 0).ok();
    let previous_qos = handles.qos_tap_config.get(&runtime.tap_id, 0).ok();
    let acl_cfg = merge_tap_config(previous_acl, runtime, acl_enabled, qos_enabled);
    let qos_cfg = merge_tap_config(previous_qos, runtime, acl_enabled, qos_enabled);

    handles
        .acl_tap_config
        .insert(runtime.tap_id, acl_cfg, 0)
        .map_err(|e| format!("ACL TAP_CONFIG_MAP insert: {:?}", e))?;
    if let Err(error) = handles.qos_tap_config.insert(runtime.tap_id, qos_cfg, 0) {
        restore_tap_config(&mut handles.acl_tap_config, runtime.tap_id, previous_acl).map_err(
            |rollback| {
                format!(
                    "QoS TAP_CONFIG_MAP insert: {:?}; ACL rollback failed: {}",
                    error, rollback
                )
            },
        )?;
        return Err(format!("QoS TAP_CONFIG_MAP insert: {:?}", error));
    }
    Ok(())
}

fn merge_firewall_config(
    existing: Option<FirewallConfig>,
    runtime: TapMapRuntime,
    acl_enabled: Option<bool>,
    qos_enabled: Option<bool>,
) -> FirewallConfig {
    let mut config = existing.unwrap_or_else(|| FirewallConfig {
        conntrack_enabled: 1,
        monitoring_enabled: 1,
        num_cpus: 1,
        qos_enabled: 0,
        acl_enabled: 1,
        mirror_enabled: 0,
        tcprt_enabled: 0,
        ssl_enabled: 0,
        lb_enabled: 0,
        policy_generation: runtime.policy_generation,
    });
    if let Some(enabled) = acl_enabled {
        config.acl_enabled = enabled as u8;
    }
    if let Some(enabled) = qos_enabled {
        config.qos_enabled = enabled as u8;
    }
    config.policy_generation = runtime.policy_generation;
    config
}

fn merge_tap_config(
    existing: Option<TapConfig>,
    runtime: TapMapRuntime,
    acl_enabled: Option<bool>,
    qos_enabled: Option<bool>,
) -> TapConfig {
    let mut config = existing.unwrap_or_else(|| TapConfig {
        conntrack_enabled: 1,
        monitoring_enabled: 1,
        acl_enabled: 1,
        qos_enabled: 0,
        mirror_enabled: 0,
        tcprt_enabled: 0,
        lb_enabled: 0,
        pad: [0; 1],
        policy_generation: runtime.policy_generation,
    });
    if let Some(enabled) = acl_enabled {
        config.acl_enabled = enabled as u8;
    }
    if let Some(enabled) = qos_enabled {
        config.qos_enabled = enabled as u8;
    }
    config.policy_generation = runtime.policy_generation;
    config
}

fn restore_firewall_config(
    map: &mut AyaHashMap<MapData, u32, FirewallConfig>,
    key: u32,
    previous: Option<FirewallConfig>,
) -> Result<(), String> {
    match previous {
        Some(config) => map
            .insert(key, config, 0)
            .map_err(|e| format!("FIREWALL_CONFIG restore: {:?}", e)),
        None => {
            let _ = map.remove(&key);
            Ok(())
        }
    }
}

fn restore_tap_config(
    map: &mut AyaHashMap<MapData, u32, TapConfig>,
    key: u32,
    previous: Option<TapConfig>,
) -> Result<(), String> {
    match previous {
        Some(config) => map
            .insert(key, config, 0)
            .map_err(|e| format!("TAP_CONFIG_MAP restore: {:?}", e)),
        None => {
            let _ = map.remove(&key);
            Ok(())
        }
    }
}

pub fn cleanup_policy_generation(
    handles: &mut AclQosMapHandles,
    runtime: TapMapRuntime,
) -> Result<(), String> {
    let policy_keys: Vec<PolicyKey> = handles
        .policy_table
        .iter()
        .filter_map(|item| item.ok().map(|(key, _)| key))
        .filter(|key| key.tap_id == runtime.tap_id && key.generation == runtime.policy_generation)
        .collect();
    for key in policy_keys {
        let _ = handles.policy_table.remove(&key);
        let _ = handles.rule_stats.remove(&key);
    }

    let stat_keys: Vec<PolicyKey> = handles
        .rule_stats
        .iter()
        .filter_map(|item| item.ok().map(|(key, _)| key))
        .filter(|key| key.tap_id == runtime.tap_id && key.generation == runtime.policy_generation)
        .collect();
    for key in stat_keys {
        let _ = handles.rule_stats.remove(&key);
    }

    let port_keys: Vec<PortKey> = handles
        .port_bitmap_pool
        .iter()
        .filter_map(|item| item.ok().map(|(key, _)| key))
        .filter(|key| key.tap_id == runtime.tap_id && key.generation == runtime.policy_generation)
        .collect();
    for key in port_keys {
        let _ = handles.port_bitmap_pool.remove(&key);
    }

    let qos_keys: Vec<QosKey> = handles
        .qos_config
        .iter()
        .filter_map(|item| item.ok().map(|(key, _)| key))
        .filter(|key| key.tap_id == runtime.tap_id && key.generation == runtime.policy_generation)
        .collect();
    for key in qos_keys {
        let _ = handles.qos_config.remove(&key);
        let _ = handles.qos_token_bucket.remove(&key);
        let _ = handles.qos_stats.remove(&key);
    }

    let qos_stat_keys: Vec<QosKey> = handles
        .qos_stats
        .iter()
        .filter_map(|item| item.ok().map(|(key, _)| key))
        .filter(|key| key.tap_id == runtime.tap_id && key.generation == runtime.policy_generation)
        .collect();
    for key in qos_stat_keys {
        let _ = handles.qos_stats.remove(&key);
    }

    Ok(())
}

#[derive(Debug, Clone)]
pub struct AclRuleStat {
    pub key: PolicyKey,
    pub packets: u64,
    pub bytes: u64,
    pub dropped_packets: u64,
    pub dropped_bytes: u64,
}

#[derive(Debug, Clone)]
pub struct QosRuleStat {
    pub key: QosKey,
    pub passed_packets: u64,
    pub passed_bytes: u64,
    pub dropped_packets: u64,
    pub dropped_bytes: u64,
    pub shaped_packets: u64,
    pub shaped_bytes: u64,
}

pub fn get_rule_stats(
    handles: &AclQosMapHandles,
    runtime: TapMapRuntime,
) -> Result<Vec<AclRuleStat>, String> {
    let mut entries = Vec::new();
    for item in handles.rule_stats.iter() {
        let Ok((key, values)) = item else {
            continue;
        };
        if key.tap_id != runtime.tap_id || key.generation != runtime.policy_generation {
            continue;
        }
        let mut stat = AclRuleStat {
            key,
            packets: 0,
            bytes: 0,
            dropped_packets: 0,
            dropped_bytes: 0,
        };
        for value in values.iter() {
            stat.packets += value.packets;
            stat.bytes += value.bytes;
            stat.dropped_packets += value.dropped_packets;
            stat.dropped_bytes += value.dropped_bytes;
        }
        entries.push(stat);
    }
    Ok(entries)
}

pub fn get_qos_stats(
    handles: &AclQosMapHandles,
    runtime: TapMapRuntime,
) -> Result<Vec<QosRuleStat>, String> {
    let mut entries = Vec::new();
    for item in handles.qos_stats.iter() {
        let Ok((key, values)) = item else {
            continue;
        };
        if key.tap_id != runtime.tap_id || key.generation != runtime.policy_generation {
            continue;
        }
        let mut stat = QosRuleStat {
            key,
            passed_packets: 0,
            passed_bytes: 0,
            dropped_packets: 0,
            dropped_bytes: 0,
            shaped_packets: 0,
            shaped_bytes: 0,
        };
        for value in values.iter() {
            stat.passed_packets += value.passed_packets;
            stat.passed_bytes += value.passed_bytes;
            stat.dropped_packets += value.dropped_packets;
            stat.dropped_bytes += value.dropped_bytes;
            stat.shaped_packets += value.shaped_packets;
            stat.shaped_bytes += value.shaped_bytes;
        }
        entries.push(stat);
    }
    Ok(entries)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn firewall_config_partial_update_preserves_other_switches() {
        let existing = FirewallConfig {
            conntrack_enabled: 1,
            monitoring_enabled: 1,
            num_cpus: 1,
            qos_enabled: 0,
            acl_enabled: 0,
            mirror_enabled: 0,
            tcprt_enabled: 0,
            ssl_enabled: 0,
            lb_enabled: 0,
            policy_generation: 7,
        };

        let config = merge_firewall_config(
            Some(existing),
            TapMapRuntime {
                tap_id: TAP_ID_UNASSIGNED,
                policy_generation: 8,
            },
            None,
            Some(true),
        );

        assert_eq!(config.policy_generation, 8);
        assert_eq!(config.acl_enabled, 0);
        assert_eq!(config.qos_enabled, 1);
    }

    #[test]
    fn tap_config_generation_update_preserves_acl_and_qos_switches() {
        let existing = TapConfig {
            conntrack_enabled: 1,
            monitoring_enabled: 1,
            acl_enabled: 0,
            qos_enabled: 1,
            mirror_enabled: 0,
            tcprt_enabled: 0,
            lb_enabled: 0,
            pad: [0; 1],
            policy_generation: 15,
        };

        let config = merge_tap_config(
            Some(existing),
            TapMapRuntime {
                tap_id: 42,
                policy_generation: 16,
            },
            None,
            None,
        );

        assert_eq!(config.policy_generation, 16);
        assert_eq!(config.acl_enabled, 0);
        assert_eq!(config.qos_enabled, 1);
    }

    #[test]
    fn missing_runtime_config_uses_product_defaults() {
        let runtime = TapMapRuntime {
            tap_id: TAP_ID_UNASSIGNED,
            policy_generation: 3,
        };

        let firewall = merge_firewall_config(None, runtime, None, None);
        assert_eq!(firewall.policy_generation, 3);
        assert_eq!(firewall.acl_enabled, 1);
        assert_eq!(firewall.qos_enabled, 0);

        let tap = merge_tap_config(None, runtime, None, None);
        assert_eq!(tap.policy_generation, 3);
        assert_eq!(tap.acl_enabled, 1);
        assert_eq!(tap.qos_enabled, 0);
    }
}

pub fn ensure_fq_qdisc(iface: &str) -> Result<(), String> {
    let output = std::process::Command::new("tc")
        .args([
            "qdisc",
            "replace",
            "dev",
            iface,
            "root",
            "fq",
            "flow_limit",
            "1000",
        ])
        .output()
        .map_err(|e| format!("failed to run tc qdisc replace fq: {}", e))?;
    if output.status.success() {
        Ok(())
    } else {
        Err(String::from_utf8_lossy(&output.stderr).trim().to_string())
    }
}

pub fn cleanup_root_qdisc(iface: &str) -> Result<(), String> {
    let output = std::process::Command::new("tc")
        .args(["qdisc", "del", "dev", iface, "root"])
        .output()
        .map_err(|e| format!("failed to run tc qdisc del root: {}", e))?;
    if output.status.success() {
        return Ok(());
    }
    let stderr = String::from_utf8_lossy(&output.stderr);
    if stderr.contains("Cannot delete qdisc with handle of zero")
        || stderr.contains("No such file or directory")
        || stderr.trim().is_empty()
    {
        Ok(())
    } else {
        Err(stderr.trim().to_string())
    }
}
