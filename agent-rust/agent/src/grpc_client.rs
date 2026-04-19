use anyhow::{Context, Result};
use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::transport::{Channel, ClientTlsConfig, Endpoint};
use tonic::Streaming;

// 引入生成的 protobuf 代码
pub mod aria {
    tonic::include_proto!("aria.agent");
}

use aria::controller_service_client::ControllerServiceClient;

pub type GrpcCommandRequest = aria::CommandRequest;
pub type GrpcCommandResponse = aria::CommandResponse;

#[derive(Debug, Clone)]
pub struct RegisterResult {
    pub assigned_ip: String,
    pub node_id: Option<String>,
    pub runtime_token: Option<String>,
    pub runtime_token_expires_at: Option<i64>,
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
            .timeout(Duration::from_secs(30))
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
    ) -> Result<RegisterResult> {
        let kernel_version = get_kernel_version();
        let has_aesni = has_aesni_support();
        
        let request = tonic::Request::new(aria::RegisterRequest {
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
        });

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

        Ok(RegisterResult {
            assigned_ip: resp.assigned_ip,
            node_id: (!resp.node_id.trim().is_empty()).then_some(resp.node_id),
            runtime_token,
            runtime_token_expires_at,
        })
    }
    
    /// 从 Controller 同步配置（不带状态上报，向后兼容）
    #[allow(dead_code)]
    pub async fn sync(&self, node_id: Option<String>, public_key: String, runtime_token: Option<String>) -> Result<SyncResult> {
        self.sync_with_state(node_id, public_key, None, None, None, runtime_token).await
    }

    pub async fn sync_with_state(
        &self,
        node_id: Option<String>,
        public_key: String,
        applied_state_version: Option<String>,
        observed_state: Option<String>,
        observed_message: Option<String>,
        runtime_token: Option<String>,
    ) -> Result<SyncResult> {
        let mut request = tonic::Request::new(aria::SyncRequest {
            public_key,
            node_id: node_id.unwrap_or_default(),
            applied_state_version: applied_state_version.unwrap_or_default(),
            observed_state: observed_state.unwrap_or_default(),
            observed_message: observed_message.unwrap_or_default(),
        });

        if let Some(token) = &runtime_token {
            let metadata = request.metadata_mut();
            metadata.insert("authorization", format!("Bearer {}", token).parse().unwrap());
        }

        let mut client = ControllerServiceClient::new(self.channel.clone());
        let response = client.sync(request).await?;
        let resp = response.into_inner();

        let new_runtime_token = if !resp.runtime_token.trim().is_empty() {
            Some(resp.runtime_token)
        } else {
            None
        };
        let new_runtime_token_expires_at = if resp.runtime_token_expires_at > 0 {
            Some(resp.runtime_token_expires_at)
        } else {
            None
        };

        Ok(SyncResult {
            peers: resp.peers.into_iter().map(|p| PeerInfo {
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
            }).collect(),
            assigned_ip: resp.assigned_ip,
            desired_state_version: resp.desired_state_version,
            acl_rules: resp.acl_rules.into_iter().map(|r| AclRule {
                src_net: r.src_net,
                dst_net: r.dst_net,
                protocol: r.protocol,
                min_port: r.min_port,
                max_port: r.max_port,
            }).collect(),
            qos_rules: resp.qos_rules.into_iter().map(|r| QoSRule {
                src_ip: r.src_ip,
                dst_ip: r.dst_ip,
                src_port: r.src_port,
                dst_port: r.dst_port,
                protocol: r.protocol,
                bandwidth_mbps: r.bandwidth_mbps,
            }).collect(),
            blacklist_rules: resp.blacklist_rules.into_iter().map(|r| BlacklistRule {
                scope: r.scope,
                cidr: r.cidr,
                port: r.port,
            }).collect(),
            runtime_token: new_runtime_token,
            runtime_token_expires_at: new_runtime_token_expires_at,
        })
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
        if let Some(token) = &runtime_token {
            let metadata = request.metadata_mut();
            metadata.insert("authorization", format!("Bearer {}", token).parse().unwrap());
        }

        let response = client.command_stream(request).await?;

        Ok((tx, response.into_inner()))
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
    pub acl_rules: Vec<AclRule>,
    pub qos_rules: Vec<QoSRule>,
    pub blacklist_rules: Vec<BlacklistRule>,
    pub runtime_token: Option<String>,
    pub runtime_token_expires_at: Option<i64>,
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
    pub src_net: String,
    pub dst_net: String,
    pub protocol: u32,
    pub min_port: u32,
    #[allow(dead_code)]
    pub max_port: u32,
}

#[allow(dead_code)]
/// QoS 规则
#[derive(Debug, Clone)]
pub struct QoSRule {
    pub src_ip: String,
    pub dst_ip: String,
    pub src_port: u32,
    pub dst_port: u32,
    pub protocol: u32,
    pub bandwidth_mbps: u64,
}

#[derive(Debug, Clone)]
pub struct BlacklistRule {
    pub scope: String,
    pub cidr: String,
    pub port: u32,
}
