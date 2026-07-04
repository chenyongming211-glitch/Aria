use anyhow::{Context, Result};
use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::metadata::{Ascii, MetadataValue};
use tonic::transport::{Channel, ClientTlsConfig, Endpoint};
use tonic::Streaming;

// 引入生成的 protobuf 代码
pub mod aria {
    tonic::include_proto!("aria.agent");
}

use aria::controller_service_client::ControllerServiceClient;

pub type GrpcCommandRequest = aria::CommandRequest;
pub type GrpcCommandResponse = aria::CommandResponse;

const UNARY_RPC_TIMEOUT: Duration = Duration::from_secs(30);

pub fn unary_rpc_timeout() -> Option<Duration> {
    Some(UNARY_RPC_TIMEOUT)
}

pub fn command_stream_rpc_timeout() -> Option<Duration> {
    None
}

fn apply_unary_timeout<T>(request: &mut tonic::Request<T>) {
    if let Some(timeout) = unary_rpc_timeout() {
        request.set_timeout(timeout);
    }
}

fn authorization_metadata_value(token: &str) -> Result<MetadataValue<Ascii>> {
    format!("Bearer {}", token.trim())
        .parse()
        .context("invalid runtime token metadata")
}

#[derive(Debug, Clone)]
pub struct RegisterResult {
    pub assigned_ip: String,
    pub node_id: Option<String>,
    pub runtime_token: Option<String>,
    pub runtime_token_expires_at: Option<i64>,
    pub certificate_pem: Option<String>,
    pub certificate_ca: Option<String>,
    pub certificate_not_after: Option<i64>,
    pub certificate_renew_before: Option<i64>,
}

/// 获取内核版本
fn get_kernel_version() -> String {
    std::fs::read_to_string("/proc/sys/kernel/osrelease")
        .unwrap_or_else(|_| "unknown".to_string())
        .trim()
        .to_string()
}

/// 检测是否支持 AES-NI
fn has_aesni_support() -> bool {
    if let Ok(cpuinfo) = std::fs::read_to_string("/proc/cpuinfo") {
        cpuinfo.contains("aes")
    } else {
        false
    }
}

/// gRPC Controller 客户端
#[derive(Clone)]
pub struct GrpcClient {
    channel: Channel,
}

impl GrpcClient {
    pub async fn new_with_options(
        controller_url: String,
        ca_cert_path: String,
        client_cert_path: String,
        client_key_path: String,
        tls_server_name: Option<String>,
    ) -> Result<Self> {
        let uses_tls = controller_url.starts_with("https://");
        let mut endpoint = Endpoint::from_shared(controller_url.clone())?
            .connect_timeout(Duration::from_secs(10))
            .tcp_keepalive(Some(Duration::from_secs(60)));

        if uses_tls {
            let mut tls_config = ClientTlsConfig::new();

            if let Some(ca_cert) = read_optional_file(&ca_cert_path)? {
                tls_config = tls_config.ca_certificate(tonic::transport::Certificate::from_pem(ca_cert));
            }

            let client_cert = read_optional_file(&client_cert_path)?;
            let client_key = read_optional_file(&client_key_path)?;
            match (client_cert, client_key) {
                (Some(cert), Some(key)) => {
                    tls_config = tls_config.identity(tonic::transport::Identity::from_pem(cert, key));
                }
                (None, None) => {}
                _ => {
                    return Err(anyhow::anyhow!(
                        "client_cert and client_key must be provided together for mTLS"
                    ));
                }
            }

            let domain_name = tls_server_name
                .or_else(|| infer_tls_server_name(&controller_url))
                .unwrap_or_else(|| "localhost".to_string());
            tracing::info!(
                "Connecting to Controller at {} with TLS (domain: {})",
                controller_url,
                domain_name
            );
            tls_config = tls_config.domain_name(domain_name);
            endpoint = endpoint.tls_config(tls_config)?;
        } else {
            tracing::info!("Connecting to Controller at {} without TLS", controller_url);
        }

        let channel = endpoint.connect().await?;
        Ok(Self { channel })
    }

    /// 创建新的 gRPC 客户端（mTLS：双向认证）
    #[allow(dead_code)]
    pub async fn new(
        controller_url: String,
        ca_cert_path: String,
        client_cert_path: String,
        client_key_path: String,
    ) -> Result<Self> {
        Self::new_with_options(
            controller_url,
            ca_cert_path,
            client_cert_path,
            client_key_path,
            None,
        )
        .await
    }
    
    /// 注册到 Controller
    #[allow(dead_code)]
    pub async fn register(
        &self,
        public_key: String,
        endpoint: String,
        public_ip: String,
        hostname: String,
        token: String,
        region: String,
    ) -> Result<RegisterResult> {
        self.register_with_details(
            public_key,
            endpoint,
            public_ip,
            hostname,
            token,
            region,
            String::new(),
            Vec::new(),
            None,
        )
        .await
    }

    pub async fn register_with_details(
        &self,
        public_key: String,
        endpoint: String,
        public_ip: String,
        hostname: String,
        token: String,
        region: String,
        machine_id: String,
        advertised_routes: Vec<String>,
        csr_pem: Option<String>,
    ) -> Result<RegisterResult> {
        let kernel_version = get_kernel_version();
        let has_aesni = has_aesni_support();
        
        let mut request = tonic::Request::new(aria::RegisterRequest {
            public_key,
            endpoint,
            private_ip: String::new(),
            public_ip,
            hostname,
            registered_at: std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_secs() as i64,
            token,
            advertised_routes,
            region,
            customer_id: String::new(),
            runtime_mode: "ebpf".to_string(),
            kernel_version,
            has_aesni,
            machine_id,
            csr_pem: csr_pem.unwrap_or_default(),
        });
        apply_unary_timeout(&mut request);

        let mut client = ControllerServiceClient::new(self.channel.clone());
        let response = client.register(request).await?;
        let resp = response.into_inner();

        let runtime_token = if !resp.runtime_token.trim().is_empty() {
            Some(resp.runtime_token)
        } else {
            None
        };
        let runtime_token_expires_at = if resp.runtime_token_expires_at > 0 {
            Some(resp.runtime_token_expires_at)
        } else {
            None
        };
        let certificate_pem = if !resp.certificate_pem.trim().is_empty() {
            Some(resp.certificate_pem)
        } else {
            None
        };
        let certificate_ca = if !resp.certificate_ca.trim().is_empty() {
            Some(resp.certificate_ca)
        } else {
            None
        };
        let certificate_not_after = if resp.certificate_not_after > 0 {
            Some(resp.certificate_not_after)
        } else {
            None
        };
        let certificate_renew_before = if resp.certificate_renew_before > 0 {
            Some(resp.certificate_renew_before)
        } else {
            None
        };

        Ok(RegisterResult {
            assigned_ip: resp.assigned_ip,
            node_id: (!resp.node_id.trim().is_empty()).then_some(resp.node_id),
            runtime_token,
            runtime_token_expires_at,
            certificate_pem,
            certificate_ca,
            certificate_not_after,
            certificate_renew_before,
        })
    }
    
    /// 从 Controller 同步配置（不带状态上报，向后兼容）
    #[allow(dead_code)]
    pub async fn sync(&self, node_id: Option<String>, public_key: String, runtime_token: Option<String>) -> Result<SyncResult> {
        self.sync_with_state(node_id, public_key, None, None, None, None, None, runtime_token).await
    }

    pub async fn sync_with_state(
        &self,
        node_id: Option<String>,
        public_key: String,
        applied_state_version: Option<String>,
        observed_state: Option<String>,
        observed_message: Option<String>,
        public_ip: Option<String>,
        endpoint: Option<String>,
        runtime_token: Option<String>,
    ) -> Result<SyncResult> {
        let mut request = tonic::Request::new(aria::SyncRequest {
            public_key,
            node_id: node_id.unwrap_or_default(),
            applied_state_version: applied_state_version.unwrap_or_default(),
            observed_state: observed_state.unwrap_or_default(),
            observed_message: observed_message.unwrap_or_default(),
            public_ip: public_ip.unwrap_or_default(),
            endpoint: endpoint.unwrap_or_default(),
        });

        if let Some(token) = &runtime_token {
            let metadata = request.metadata_mut();
            metadata.insert("authorization", authorization_metadata_value(token)?);
        }
        apply_unary_timeout(&mut request);

        let mut client = ControllerServiceClient::new(self.channel.clone());
        let response = client.sync(request).await?;
        Ok(sync_response_to_result(response.into_inner()))
    }

    pub async fn report_metrics(
        &self,
        node_id: Option<String>,
        public_key: String,
        custom_metrics: HashMap<String, f64>,
        runtime_token: Option<String>,
    ) -> Result<()> {
        let timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs() as i64;
        let mut request = tonic::Request::new(aria::MetricsReportRequest {
            agent_id: public_key.clone(),
            timestamp,
            cpu_usage: 0.0,
            memory_usage: 0.0,
            memory_total: 0,
            memory_used: 0,
            disk_usage: 0.0,
            disk_total: 0,
            disk_used: 0,
            network_tx_bytes: 0,
            network_rx_bytes: 0,
            network_tx_packets: 0,
            network_rx_packets: 0,
            network_tx_errors: 0,
            network_rx_errors: 0,
            wg_tx_bytes: 0,
            wg_rx_bytes: 0,
            wg_peer_count: 0,
            wg_active_peers: 0,
            custom_metrics,
            node_id: node_id.unwrap_or_default(),
            public_key,
        });

        if let Some(token) = &runtime_token {
            request
                .metadata_mut()
                .insert("authorization", authorization_metadata_value(token)?);
        }
        apply_unary_timeout(&mut request);

        let mut client = ControllerServiceClient::new(self.channel.clone());
        let response = client.report_metrics(request).await?.into_inner();
        if response.success {
            Ok(())
        } else {
            Err(anyhow::anyhow!("metrics report rejected: {}", response.message))
        }
    }

    pub async fn connect_command_stream(
        &self,
        node_id: Option<String>,
        public_key: String,
        runtime_token: Option<String>,
    ) -> Result<(mpsc::Sender<GrpcCommandResponse>, Streaming<GrpcCommandRequest>)> {
        let (tx, rx) = mpsc::channel(16);

        let mut init_result = HashMap::new();
        if let Some(node_id) = node_id.clone().filter(|value| !value.trim().is_empty()) {
            init_result.insert("agent_id".to_string(), node_id.clone());
            init_result.insert("node_id".to_string(), node_id);
        } else {
            init_result.insert("agent_id".to_string(), public_key.clone());
        }
        init_result.insert("public_key".to_string(), public_key.clone());

        tx.send(GrpcCommandResponse {
            command_id: "init".to_string(),
            status: "ready".to_string(),
            message: "agent connected".to_string(),
            result: init_result,
            completed_at: 0,
            node_id: node_id.unwrap_or_default(),
            public_key,
        })
        .await
        .context("failed to enqueue init message")?;

        let request_stream = ReceiverStream::new(rx);
        let mut client = ControllerServiceClient::new(self.channel.clone());

        let mut request = tonic::Request::new(request_stream);
        if let Some(timeout) = command_stream_rpc_timeout() {
            request.set_timeout(timeout);
        }
        if let Some(token) = &runtime_token {
            let metadata = request.metadata_mut();
            metadata.insert("authorization", authorization_metadata_value(token)?);
        }

        let response = client.command_stream(request).await?;

        Ok((tx, response.into_inner()))
    }
}

fn sync_response_to_result(resp: aria::SyncResponse) -> SyncResult {
    let aria::SyncResponse {
        peers,
        assigned_ip,
        acl_rules,
        qos_rules,
        blacklist_rules,
        desired_state_version,
        runtime_token,
        runtime_token_expires_at,
        snapshot_complete,
        domain_versions,
        ip_groups,
        ..
    } = resp;

    let new_runtime_token = if !runtime_token.trim().is_empty() {
        Some(runtime_token)
    } else {
        None
    };
    let new_runtime_token_expires_at = if runtime_token_expires_at > 0 {
        Some(runtime_token_expires_at)
    } else {
        None
    };

    let mut converted_peers = Vec::with_capacity(peers.len());
    for p in peers {
        converted_peers.push(PeerInfo {
            public_key: p.public_key,
            endpoint: p.endpoint,
            private_ip: p.private_ip,
            public_ip: p.public_ip,
            region: p.region,
            vpc_id: p.vpc_id,
            hostname: p.hostname,
            assigned_ip: p.assigned_ip,
            role: p.role,
            advertised_routes: p.advertised_routes,
        });
    }

    let mut converted_ip_groups = Vec::with_capacity(ip_groups.len());
    for g in ip_groups {
        converted_ip_groups.push(IPGroup {
            id: g.id,
            name: g.name,
            cidrs: g.cidrs,
            kind: g.kind,
        });
    }

    let mut converted_acl_rules = Vec::with_capacity(acl_rules.len());
    for r in acl_rules {
        converted_acl_rules.push(AclRule {
            id: r.id,
            src_net: r.src_net,
            dst_net: r.dst_net,
            src_group_id: r.src_group_id,
            dst_group_id: r.dst_group_id,
            protocol: r.protocol,
            min_port: r.min_port,
            max_port: r.max_port,
            action: r.action,
            direction: r.direction,
            ports: r.ports,
            priority: r.priority,
        });
    }

    let mut converted_qos_rules = Vec::with_capacity(qos_rules.len());
    for r in qos_rules {
        converted_qos_rules.push(QoSRule {
            id: r.id,
            src_ip: r.src_ip,
            dst_ip: r.dst_ip,
            group_id: r.group_id,
            src_port: r.src_port,
            dst_port: r.dst_port,
            protocol: r.protocol,
            bandwidth_mbps: r.bandwidth_mbps,
            direction: r.direction,
            rate_bps: r.rate_bps,
            burst_bytes: r.burst_bytes,
            priority: r.priority,
            mode: r.mode,
        });
    }

    let mut converted_blacklist_rules = Vec::with_capacity(blacklist_rules.len());
    for r in blacklist_rules {
        converted_blacklist_rules.push(BlacklistRule {
            scope: r.scope,
            cidr: r.cidr,
            port: r.port,
        });
    }

    SyncResult {
        peers: converted_peers,
        assigned_ip,
        desired_state_version,
        ip_groups: converted_ip_groups,
        acl_rules: converted_acl_rules,
        qos_rules: converted_qos_rules,
        blacklist_rules: converted_blacklist_rules,
        runtime_token: new_runtime_token,
        runtime_token_expires_at: new_runtime_token_expires_at,
        snapshot_complete,
        domain_versions,
    }
}

fn read_optional_file(path: &str) -> Result<Option<String>> {
    if path.trim().is_empty() {
        return Ok(None);
    }

    let file_path = Path::new(path);
    if !file_path.exists() {
        return Err(anyhow::anyhow!("certificate file not found: {}", path));
    }

    fs::read_to_string(file_path)
        .map(Some)
        .with_context(|| format!("failed to read certificate file: {}", path))
}

fn infer_tls_server_name(controller_url: &str) -> Option<String> {
    let host_port = controller_url
        .split("://")
        .nth(1)
        .unwrap_or(controller_url)
        .split('/')
        .next()
        .unwrap_or(controller_url);

    if host_port.is_empty() {
        return None;
    }

    if host_port.starts_with('[') {
        return host_port
            .trim_start_matches('[')
            .split(']')
            .next()
            .map(|host| host.to_string());
    }

    Some(host_port.split(':').next().unwrap_or(host_port).to_string())
}

#[allow(dead_code)]
/// 同步结果
pub struct SyncResult {
    pub peers: Vec<PeerInfo>,
    pub assigned_ip: String,
    pub desired_state_version: String,
    pub ip_groups: Vec<IPGroup>,
    pub acl_rules: Vec<AclRule>,
    pub qos_rules: Vec<QoSRule>,
    pub blacklist_rules: Vec<BlacklistRule>,
    pub runtime_token: Option<String>,
    pub runtime_token_expires_at: Option<i64>,
    pub snapshot_complete: bool,
    pub domain_versions: HashMap<String, String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IPGroup {
    pub id: String,
    pub name: String,
    pub cidrs: Vec<String>,
    pub kind: String,
}

#[allow(dead_code)]
/// Peer 信息
#[derive(Debug, Clone)]
pub struct PeerInfo {
    pub public_key: String,
    pub endpoint: String,
    pub private_ip: String,
    pub public_ip: String,
    pub region: String,
    pub vpc_id: String,
    pub hostname: String,
    pub assigned_ip: String,
    pub role: String,
    pub advertised_routes: Vec<String>,
}

/// ACL 规则
#[derive(Debug, Clone)]
pub struct AclRule {
    pub id: String,
    pub src_net: String,
    pub dst_net: String,
    pub src_group_id: String,
    pub dst_group_id: String,
    pub protocol: u32,
    pub min_port: u32,
    pub max_port: u32,
    pub action: String,
    pub direction: String,
    pub ports: String,
    pub priority: u32,
}

#[allow(dead_code)]
/// QoS 规则
#[derive(Debug, Clone)]
pub struct QoSRule {
    pub id: String,
    pub src_ip: String,
    pub dst_ip: String,
    pub group_id: String,
    pub src_port: u32,
    pub dst_port: u32,
    pub protocol: u32,
    pub bandwidth_mbps: u64,
    pub direction: String,
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u32,
    pub mode: String,
}

#[derive(Debug, Clone)]
pub struct BlacklistRule {
    pub scope: String,
    pub cidr: String,
    pub port: u32,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AgentAclPolicy {
    pub id: String,
    pub src_group: String,
    pub dst_group: String,
    pub src_group_id: String,
    pub dst_group_id: String,
    pub proto: u8,
    pub action: u8,
    pub direction: u8,
    pub ports: Option<String>,
    pub priority: u16,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AgentQosPolicy {
    pub id: String,
    pub group: String,
    pub group_id: String,
    pub direction: u8,
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    pub mode: u8,
}

pub fn acl_policy_from_sync_rule(rule: &AclRule) -> Result<AgentAclPolicy> {
    let proto = protocol_to_u8(rule.protocol)?;
    let action = acl_action_from_string(&rule.action)?;
    let direction = direction_from_string_or_default(&rule.direction, 0)?;
    let ports = acl_ports_from_rule(rule)?;
    validate_acl_ports(proto, ports.as_deref())?;
    let priority = u16::try_from(rule.priority).context("ACL priority must fit in u16")?;

    Ok(AgentAclPolicy {
        id: rule.id.clone(),
        src_group: cidr_fallback_or_any(&rule.src_net, &rule.src_group_id),
        dst_group: cidr_fallback_or_any(&rule.dst_net, &rule.dst_group_id),
        src_group_id: rule.src_group_id.trim().to_string(),
        dst_group_id: rule.dst_group_id.trim().to_string(),
        proto,
        action,
        direction,
        ports,
        priority,
    })
}

pub fn qos_policy_from_sync_rule(rule: &QoSRule) -> Result<AgentQosPolicy> {
    let direction = direction_from_string_or_default(&rule.direction, inferred_qos_direction(rule))?;
    let group = qos_group_for_direction(rule, direction);
    let rate_bps = if rule.rate_bps > 0 {
        rule.rate_bps
    } else {
        mbps_to_bps(rule.bandwidth_mbps)
    };
    let burst_bytes = if rule.burst_bytes > 0 {
        rule.burst_bytes
    } else {
        default_qos_burst(rate_bps)
    };
    let priority = u8::try_from(rule.priority).context("qos priority must fit in u8")?;
    let mode = qos_mode_from_string(&rule.mode)?;

    Ok(AgentQosPolicy {
        id: rule.id.clone(),
        group,
        group_id: rule.group_id.trim().to_string(),
        direction,
        rate_bps,
        burst_bytes,
        priority,
        mode,
    })
}

fn protocol_to_u8(protocol: u32) -> Result<u8> {
    u8::try_from(protocol).context("protocol must fit in u8")
}

fn cidr_or_any(value: &str) -> String {
    let trimmed = value.trim();
    if trimmed.is_empty() || trimmed == "0" || trimmed == "0.0.0.0/0" || trimmed == "::/0" {
        "any".to_string()
    } else {
        trimmed.to_string()
    }
}

fn cidr_fallback_or_any(value: &str, group_id: &str) -> String {
    if group_id.trim().is_empty() {
        cidr_or_any(value)
    } else {
        value.trim().to_string()
    }
}

fn acl_action_from_string(action: &str) -> Result<u8> {
    match action.trim().to_ascii_lowercase().as_str() {
        "" | "accept" | "pass" | "allow" => Ok(0),
        "drop" | "deny" => Ok(1),
        other => Err(anyhow::anyhow!("invalid ACL action '{}'", other)),
    }
}

fn direction_from_string_or_default(direction: &str, default: u8) -> Result<u8> {
    match direction.trim().to_ascii_lowercase().as_str() {
        "" => Ok(default),
        "ingress" | "in" => Ok(0),
        "egress" | "out" => Ok(1),
        "both" | "all" => Ok(2),
        other => Err(anyhow::anyhow!(
            "invalid direction '{}': must be ingress, egress, or both",
            other
        )),
    }
}

fn acl_ports_from_rule(rule: &AclRule) -> Result<Option<String>> {
    let explicit = rule.ports.trim();
    if !explicit.is_empty() {
        if explicit.eq_ignore_ascii_case("all") {
            return Ok(None);
        }
        return Ok(Some(explicit.to_string()));
    }

    let min = rule.min_port;
    let max = rule.max_port;
    if min == 0 && (max == 0 || max == 65535) {
        return Ok(None);
    }
    if min > 65535 || max > 65535 {
        return Err(anyhow::anyhow!("ACL ports must be in 0..65535"));
    }
    if min == max {
        return Ok(Some(min.to_string()));
    }
    if min > max {
        return Err(anyhow::anyhow!(
            "invalid ACL port range: {}-{}",
            min,
            max
        ));
    }
    Ok(Some(format!("{}-{}", min, max)))
}

fn validate_acl_ports(proto: u8, ports: Option<&str>) -> Result<()> {
    let Some(ports) = ports else {
        return Ok(());
    };
    if ports.trim().is_empty() {
        return Ok(());
    }

    match proto {
        6 | 17 => Ok(()),
        0 => Err(anyhow::anyhow!(
            "port filters require a concrete protocol; use tcp or udp instead of any"
        )),
        other => Err(anyhow::anyhow!(
            "protocol {} does not support ACL port filters",
            other
        )),
    }
}

fn inferred_qos_direction(rule: &QoSRule) -> u8 {
    if !rule.src_ip.trim().is_empty() && rule.dst_ip.trim().is_empty() {
        0
    } else {
        1
    }
}

fn qos_group_for_direction(rule: &QoSRule, direction: u8) -> String {
    if !rule.group_id.trim().is_empty() {
        return qos_group_fallback_for_direction(rule, direction);
    }
    match direction {
        0 => cidr_or_any(&rule.src_ip),
        1 => {
            let dst = cidr_or_any(&rule.dst_ip);
            if dst == "any" {
                cidr_or_any(&rule.src_ip)
            } else {
                dst
            }
        }
        _ => {
            let dst = cidr_or_any(&rule.dst_ip);
            if dst == "any" {
                cidr_or_any(&rule.src_ip)
            } else {
                dst
            }
        }
    }
}

fn qos_group_fallback_for_direction(rule: &QoSRule, direction: u8) -> String {
    match direction {
        0 => rule.src_ip.trim().to_string(),
        _ => {
            let dst = rule.dst_ip.trim();
            if dst.is_empty() {
                rule.src_ip.trim().to_string()
            } else {
                dst.to_string()
            }
        }
    }
}

fn mbps_to_bps(mbps: u64) -> u64 {
    mbps.saturating_mul(1_000_000)
}

fn default_qos_burst(rate_bps: u64) -> u64 {
    (rate_bps / 8 / 10).max(1500)
}

fn qos_mode_from_string(mode: &str) -> Result<u8> {
    match mode.trim().to_ascii_lowercase().as_str() {
        "" | "auto" => Ok(crate::acl_qos_state::QOS_MODE_AUTO),
        "policing" => Ok(crate::acl_qos_state::QOS_MODE_POLICING),
        "shaping" => Ok(crate::acl_qos_state::QOS_MODE_SHAPING),
        other => Err(anyhow::anyhow!(
            "invalid QoS mode '{}': must be auto, policing, or shaping",
            other
        )),
    }
}

#[cfg(test)]
mod tests {
    use super::{
        aria,
        acl_policy_from_sync_rule,
        authorization_metadata_value,
        qos_policy_from_sync_rule,
        sync_response_to_result,
        AclRule,
        QoSRule,
    };

    #[test]
    fn authorization_metadata_rejects_invalid_token_without_panic() {
        let value = authorization_metadata_value("bad\nruntime-token");

        assert!(value.is_err());
    }

    #[test]
    fn sync_response_conversion_preserves_runtime_metadata() {
        let result = sync_response_to_result(aria::SyncResponse {
            assigned_ip: "100.64.0.2".to_string(),
            desired_state_version: "dsv-1".to_string(),
            runtime_token: "rt.new-token".to_string(),
            runtime_token_expires_at: 1_700_000_000,
            snapshot_complete: true,
            ..Default::default()
        });

        assert_eq!(result.assigned_ip, "100.64.0.2");
        assert_eq!(result.desired_state_version, "dsv-1");
        assert_eq!(result.runtime_token.as_deref(), Some("rt.new-token"));
        assert_eq!(result.runtime_token_expires_at, Some(1_700_000_000));
        assert!(result.snapshot_complete);
    }

    #[test]
    fn sync_response_conversion_treats_blank_runtime_token_as_absent() {
        let result = sync_response_to_result(aria::SyncResponse {
            runtime_token: "   ".to_string(),
            runtime_token_expires_at: 0,
            ..Default::default()
        });

        assert_eq!(result.runtime_token, None);
        assert_eq!(result.runtime_token_expires_at, None);
    }

    #[test]
    fn acl_sync_rule_keeps_port_range_as_policy_ports() {
        let policy = acl_policy_from_sync_rule(&AclRule {
            id: "acl-1".to_string(),
            src_net: "10.0.0.0/24".to_string(),
            dst_net: "192.0.2.0/24".to_string(),
            src_group_id: String::new(),
            dst_group_id: String::new(),
            protocol: 6,
            min_port: 80,
            max_port: 82,
            action: "deny".to_string(),
            direction: "egress".to_string(),
            ports: String::new(),
            priority: 100,
        })
        .expect("valid ACL sync rule");

        assert_eq!(policy.src_group, "10.0.0.0/24");
        assert_eq!(policy.dst_group, "192.0.2.0/24");
        assert_eq!(policy.proto, 6);
        assert_eq!(policy.action, 1);
        assert_eq!(policy.direction, 1);
        assert_eq!(policy.ports.as_deref(), Some("80-82"));
        assert_eq!(policy.priority, 100);
    }

    #[test]
    fn acl_sync_rule_rejects_port_filter_with_any_protocol() {
        let result = acl_policy_from_sync_rule(&AclRule {
            id: "acl-2".to_string(),
            src_net: "10.0.0.0/24".to_string(),
            dst_net: "192.0.2.0/24".to_string(),
            src_group_id: String::new(),
            dst_group_id: String::new(),
            protocol: 0,
            min_port: 443,
            max_port: 443,
            action: "allow".to_string(),
            direction: "ingress".to_string(),
            ports: String::new(),
            priority: 100,
        });

        assert!(result.is_err());
    }

    #[test]
    fn acl_sync_rule_preserves_product_group_ids() {
        let policy = acl_policy_from_sync_rule(&AclRule {
            id: "acl-group".to_string(),
            src_net: String::new(),
            dst_net: String::new(),
            src_group_id: "src-group".to_string(),
            dst_group_id: "dst-group".to_string(),
            protocol: 1,
            min_port: 0,
            max_port: 0,
            action: "deny".to_string(),
            direction: "egress".to_string(),
            ports: String::new(),
            priority: 100,
        })
        .expect("valid ACL group rule");

        assert_eq!(policy.src_group_id, "src-group");
        assert_eq!(policy.dst_group_id, "dst-group");
        assert_eq!(policy.src_group, "");
        assert_eq!(policy.dst_group, "");
    }

    #[test]
    fn qos_sync_rule_prefers_explicit_runtime_fields() {
        let policy = qos_policy_from_sync_rule(&QoSRule {
            id: "qos-1".to_string(),
            src_ip: "10.0.0.0/24".to_string(),
            dst_ip: "192.0.2.0/24".to_string(),
            group_id: String::new(),
            src_port: 0,
            dst_port: 0,
            protocol: 0,
            bandwidth_mbps: 100,
            direction: "ingress".to_string(),
            rate_bps: 250_000_000,
            burst_bytes: 4_000_000,
            priority: 7,
            mode: "shaping".to_string(),
        })
        .expect("valid QoS sync rule");

        assert_eq!(policy.group, "10.0.0.0/24");
        assert_eq!(policy.direction, 0);
        assert_eq!(policy.rate_bps, 250_000_000);
        assert_eq!(policy.burst_bytes, 4_000_000);
        assert_eq!(policy.priority, 7);
        assert_eq!(policy.mode, 1);
    }

    #[test]
    fn qos_sync_rule_preserves_product_group_id() {
        let policy = qos_policy_from_sync_rule(&QoSRule {
            id: "qos-group".to_string(),
            src_ip: String::new(),
            dst_ip: String::new(),
            group_id: "office-group".to_string(),
            src_port: 0,
            dst_port: 0,
            protocol: 0,
            bandwidth_mbps: 10,
            direction: "egress".to_string(),
            rate_bps: 10_000_000,
            burst_bytes: 1500,
            priority: 7,
            mode: "policing".to_string(),
        })
        .expect("valid QoS group rule");

        assert_eq!(policy.group_id, "office-group");
        assert_eq!(policy.group, "");
    }

    #[test]
    fn qos_auto_mode_is_preserved_for_runtime_capability_detection() {
        let policy = qos_policy_from_sync_rule(&QoSRule {
            id: "qos-auto-egress".to_string(),
            src_ip: String::new(),
            dst_ip: "100.64.0.2/32".to_string(),
            group_id: String::new(),
            src_port: 0,
            dst_port: 0,
            protocol: 0,
            bandwidth_mbps: 10,
            direction: "egress".to_string(),
            rate_bps: 10_000_000,
            burst_bytes: 1500,
            priority: 7,
            mode: "auto".to_string(),
        })
        .expect("auto egress QoS rule");

        assert_eq!(policy.mode, crate::acl_qos_state::QOS_MODE_AUTO);
    }

    #[test]
    fn qos_empty_mode_uses_auto_default() {
        let policy = qos_policy_from_sync_rule(&QoSRule {
            id: "qos-default-mode".to_string(),
            src_ip: String::new(),
            dst_ip: "100.64.0.2/32".to_string(),
            group_id: String::new(),
            src_port: 0,
            dst_port: 0,
            protocol: 0,
            bandwidth_mbps: 10,
            direction: "egress".to_string(),
            rate_bps: 10_000_000,
            burst_bytes: 1500,
            priority: 7,
            mode: String::new(),
        })
        .expect("default mode QoS rule");

        assert_eq!(policy.mode, crate::acl_qos_state::QOS_MODE_AUTO);
    }
}
