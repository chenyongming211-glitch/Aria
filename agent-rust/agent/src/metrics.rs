use anyhow::Result;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, OnceLock};
use std::time::Instant;
use metrics::{counter, gauge};
use metrics_exporter_prometheus::PrometheusBuilder;

static START_TIME: OnceLock<Instant> = OnceLock::new();

pub fn init_metrics(addr: &str) -> Result<()> {
    let socket_addr: SocketAddr = addr.parse()?;
    let builder = PrometheusBuilder::new();
    builder
        .with_http_listener(socket_addr)
        .install()?;
    
    START_TIME.set(Instant::now()).ok();
    
    tracing::info!("Prometheus metrics server started on {}", addr);
    Ok(())
}

pub fn record_wireguard_peer_stats(
    public_key: &str,
    endpoint: Option<&str>,
    rx_bytes: u64,
    tx_bytes: u64,
    last_handshake_secs: Option<u64>,
) {
    let peer_id = &public_key[..16.min(public_key.len())];
    
    gauge!("wireguard_peer_rx_bytes", "peer" => peer_id.to_string()).set(rx_bytes as f64);
    gauge!("wireguard_peer_tx_bytes", "peer" => peer_id.to_string()).set(tx_bytes as f64);
    
    if let Some(ep) = endpoint {
        gauge!("wireguard_peer_endpoint", "peer" => peer_id.to_string(), "endpoint" => ep.to_string()).set(1.0);
    }
    
    if let Some(hs) = last_handshake_secs {
        gauge!("wireguard_peer_last_handshake_secs", "peer" => peer_id.to_string()).set(hs as f64);
        
        let is_active = hs > 0 && hs < 180;
        gauge!("wireguard_peer_active", "peer" => peer_id.to_string()).set(if is_active { 1.0 } else { 0.0 });
    }
}

pub fn record_wireguard_totals(
    total_peers: usize,
    active_peers: usize,
    total_rx_bytes: u64,
    total_tx_bytes: u64,
) {
    gauge!("wireguard_total_peers").set(total_peers as f64);
    gauge!("wireguard_active_peers").set(active_peers as f64);
    counter!("wireguard_total_rx_bytes").absolute(total_rx_bytes);
    counter!("wireguard_total_tx_bytes").absolute(total_tx_bytes);
}

#[allow(dead_code)]
pub fn record_acl_packet_stats(packets_passed: u64, packets_dropped: u64) {
    counter!("acl_packets_passed_total").absolute(packets_passed);
    counter!("acl_packets_dropped_total").absolute(packets_dropped);
}

#[allow(dead_code)]
pub fn record_qos_byte_stats(bytes_passed: u64, bytes_dropped: u64) {
    counter!("qos_bytes_passed_total").absolute(bytes_passed);
    counter!("qos_bytes_dropped_total").absolute(bytes_dropped);
}

pub fn record_acl_rule_stats(rule_id: u32, action: &str, packets: u64, bytes: u64) {
    counter!("acl_rule_packets", "rule_id" => rule_id.to_string(), "action" => action.to_string())
        .absolute(packets);
    counter!("acl_rule_bytes", "rule_id" => rule_id.to_string(), "action" => action.to_string())
        .absolute(bytes);
}

pub fn record_qos_rule_stats(rule_type: &str, rule_id: u32, passed_bytes: u64, dropped_bytes: u64) {
    counter!("qos_rule_bytes_passed", "rule_type" => rule_type.to_string(), "rule_id" => rule_id.to_string())
        .absolute(passed_bytes);
    counter!("qos_rule_bytes_dropped", "rule_type" => rule_type.to_string(), "rule_id" => rule_id.to_string())
        .absolute(dropped_bytes);
}

#[allow(dead_code)]
pub fn record_ebpf_map_size(map_name: &str, size: usize) {
    gauge!("ebpf_map_size", "map" => map_name.to_string()).set(size as f64);
}

pub fn record_agent_uptime() -> u64 {
    if let Some(start) = START_TIME.get() {
        let uptime = start.elapsed().as_secs();
        gauge!("agent_uptime_secs").set(uptime as f64);
        uptime
    } else {
        0
    }
}

pub fn record_grpc_error() {
    counter!("grpc_error_total").increment(1);
}

pub fn record_config_reload() {
    counter!("config_reload_total").increment(1);
}

pub fn record_sync_success(peer_count: usize) {
    counter!("grpc_sync_success_total").increment(1);
    gauge!("sync_peer_count").set(peer_count as f64);
}

pub fn record_sync_failure() {
    counter!("grpc_sync_failure_total").increment(1);
}

pub fn record_cleanup_failure(map_name: &str, count: u64) {
    counter!("cleanup_failure_total", "map" => map_name.to_string())
        .increment(count);
}

pub fn record_wireguard_peer_change(action: &str) {
    counter!("wireguard_peer_changes_total", "action" => action.to_string())
        .increment(1);
}

pub fn record_acl_rule_count(count: usize) {
    gauge!("acl_rule_count").set(count as f64);
}

#[allow(dead_code)]
pub fn record_qos_rule_count(count: usize) {
    gauge!("qos_rule_count").set(count as f64);
}

pub fn record_config_reload_failure() {
    counter!("config_reload_failure_total").increment(1);
}

#[allow(dead_code)]
pub struct MetricsCounters {
    pub acl_passed: AtomicU64,
    pub acl_dropped: AtomicU64,
    pub qos_passed: AtomicU64,
    pub qos_dropped: AtomicU64,
}

#[allow(dead_code)]
impl MetricsCounters {
    pub fn new() -> Arc<Self> {
        Arc::new(Self {
            acl_passed: AtomicU64::new(0),
            acl_dropped: AtomicU64::new(0),
            qos_passed: AtomicU64::new(0),
            qos_dropped: AtomicU64::new(0),
        })
    }

    pub fn inc_acl_passed(&self, count: u64) {
        self.acl_passed.fetch_add(count, Ordering::Relaxed);
    }

    pub fn inc_acl_dropped(&self, count: u64) {
        self.acl_dropped.fetch_add(count, Ordering::Relaxed);
    }

    pub fn inc_qos_passed(&self, count: u64) {
        self.qos_passed.fetch_add(count, Ordering::Relaxed);
    }

    pub fn inc_qos_dropped(&self, count: u64) {
        self.qos_dropped.fetch_add(count, Ordering::Relaxed);
    }

    pub fn get_acl_stats(&self) -> (u64, u64) {
        (
            self.acl_passed.load(Ordering::Relaxed),
            self.acl_dropped.load(Ordering::Relaxed),
        )
    }

    pub fn get_qos_stats(&self) -> (u64, u64) {
        (
            self.qos_passed.load(Ordering::Relaxed),
            self.qos_dropped.load(Ordering::Relaxed),
        )
    }

    pub fn flush_to_prometheus(&self) {
        let (acl_passed, acl_dropped) = self.get_acl_stats();
        let (qos_passed, qos_dropped) = self.get_qos_stats();
        
        record_acl_packet_stats(acl_passed, acl_dropped);
        record_qos_byte_stats(qos_passed, qos_dropped);
    }
}

impl Default for MetricsCounters {
    fn default() -> Self {
        Self {
            acl_passed: AtomicU64::new(0),
            acl_dropped: AtomicU64::new(0),
            qos_passed: AtomicU64::new(0),
            qos_dropped: AtomicU64::new(0),
        }
    }
}
