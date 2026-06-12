use aya::maps::{HashMap as AyaHashMap, MapData, PerCpuHashMap, PerCpuValues};
use aya::util::nr_cpus;
use aya::Ebpf;
use aya::Pod;
use aria_shared::{
    FirewallConfig, PolicyKey, PolicyValue, PortKey, QosConfig, QosKey, QosStatsValue,
    RuleStatsValue, TapConfig, TokenBucket, TAP_ID_UNASSIGNED,
};

pub const ACTION_ALLOW: u8 = 0;
pub const ACTION_DROP: u8 = 1;
pub const PORT_ACTION_DROP: u8 = 1;
pub const PORT_ACTION_PASS: u8 = 2;

#[derive(Debug, Clone, Copy)]
pub struct TapMapRuntime {
    pub tap_id: u32,
}

impl Default for TapMapRuntime {
    fn default() -> Self {
        Self {
            tap_id: TAP_ID_UNASSIGNED,
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

fn take_hash_map<K, V>(
    ebpf: &mut Ebpf,
    name: &str,
) -> Result<AyaHashMap<MapData, K, V>, String>
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
        let idx = bitmap_idx.ok_or_else(|| "new port set requested without bitmap_idx".to_string())?;
        if let Some(ports_str) = ports {
            for (start, end, rule_action) in parse_ports(ports_str, action)? {
                for port in start..=end {
                    let key = PortKey {
                        tap_id: runtime.tap_id,
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
        src_id,
        dst_id,
        proto,
        direction,
        pad: [0; 2],
    };
    let value = PolicyValue {
        action: stored_policy_action(action, has_port_filter),
        has_port_filter: has_port_filter as u8,
        pad1: [0; 2],
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

pub fn delete_policy_from_maps(
    handles: &mut AclQosMapHandles,
    runtime: TapMapRuntime,
    src_id: u32,
    dst_id: u32,
    proto: u8,
    direction: u8,
) -> Result<(), String> {
    let key = PolicyKey {
        tap_id: runtime.tap_id,
        src_id,
        dst_id,
        proto,
        direction,
        pad: [0; 2],
    };
    match handles.policy_table.remove(&key) {
        Ok(()) => {
            let _ = handles.rule_stats.remove(&key);
            Ok(())
        }
        Err(e) if format!("{:?}", e).contains("KeyNotFound") => Ok(()),
        Err(e) => Err(format!("POLICY_TABLE remove: {:?}", e)),
    }
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
    user_qos_enabled: bool,
) -> Result<(), String> {
    let key = QosKey {
        tap_id: runtime.tap_id,
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
    let bucket = TokenBucket {
        lock: 0,
        pad: 0,
        tokens: burst_bytes,
        last_refill_ns: 0,
        last_edt: 0,
    };

    let _ = handles.qos_token_bucket.remove(&key);
    handles
        .qos_token_bucket
        .insert(key, bucket, 0)
        .map_err(|e| format!("QOS_TOKEN_BUCKET insert: {:?}", e))?;
    handles
        .qos_config
        .insert(key, config, 0)
        .map_err(|e| format!("QOS_CONFIG insert: {:?}", e))?;
    handles
        .qos_stats
        .insert(key, zero_per_cpu_values::<QosStatsValue>()?, 0)
        .map_err(|e| format!("QOS_STATS insert: {:?}", e))?;
    sync_runtime_config(handles, runtime, None, Some(user_qos_enabled))
}

pub fn delete_qos_rule_from_maps(
    handles: &mut AclQosMapHandles,
    runtime: TapMapRuntime,
    group_id: u32,
    direction: u8,
    user_qos_enabled: bool,
) -> Result<(), String> {
    let key = QosKey {
        tap_id: runtime.tap_id,
        group_id,
        direction,
        pad: [0; 3],
    };
    let _ = handles.qos_config.remove(&key);
    let _ = handles.qos_token_bucket.remove(&key);
    let _ = handles.qos_stats.remove(&key);
    sync_runtime_config(handles, runtime, None, Some(user_qos_enabled && has_qos_rules(handles, runtime)))
}

pub fn sync_runtime_config(
    handles: &mut AclQosMapHandles,
    runtime: TapMapRuntime,
    acl_enabled: Option<bool>,
    qos_enabled: Option<bool>,
) -> Result<(), String> {
    let firewall_cfg = FirewallConfig {
        conntrack_enabled: 1,
        monitoring_enabled: 1,
        num_cpus: 1,
        qos_enabled: qos_enabled.unwrap_or(false) as u8,
        acl_enabled: acl_enabled.unwrap_or(true) as u8,
        mirror_enabled: 0,
        tcprt_enabled: 0,
        ssl_enabled: 0,
        lb_enabled: 0,
    };

    if runtime.tap_id == TAP_ID_UNASSIGNED {
        handles
            .acl_firewall_config
            .insert(0u32, firewall_cfg, 0)
            .map_err(|e| format!("ACL FIREWALL_CONFIG insert: {:?}", e))?;
        handles
            .qos_firewall_config
            .insert(0u32, firewall_cfg, 0)
            .map_err(|e| format!("QoS FIREWALL_CONFIG insert: {:?}", e))?;
        return Ok(());
    }

    let tap_cfg = TapConfig {
        conntrack_enabled: 1,
        monitoring_enabled: 1,
        acl_enabled: acl_enabled.unwrap_or(true) as u8,
        qos_enabled: qos_enabled.unwrap_or(false) as u8,
        mirror_enabled: 0,
        tcprt_enabled: 0,
        lb_enabled: 0,
        pad: [0; 1],
    };
    handles
        .acl_tap_config
        .insert(runtime.tap_id, tap_cfg, 0)
        .map_err(|e| format!("ACL TAP_CONFIG_MAP insert: {:?}", e))?;
    handles
        .qos_tap_config
        .insert(runtime.tap_id, tap_cfg, 0)
        .map_err(|e| format!("QoS TAP_CONFIG_MAP insert: {:?}", e))
}

fn has_qos_rules(handles: &AclQosMapHandles, runtime: TapMapRuntime) -> bool {
    handles.qos_config.iter().any(|item| {
        item.map(|(key, _)| key.tap_id == runtime.tap_id)
            .unwrap_or(false)
    })
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
        if key.tap_id != runtime.tap_id {
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
        if key.tap_id != runtime.tap_id {
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

pub fn ensure_fq_qdisc(iface: &str) -> Result<(), String> {
    let output = std::process::Command::new("tc")
        .args(["qdisc", "replace", "dev", iface, "root", "fq", "flow_limit", "1000"])
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
