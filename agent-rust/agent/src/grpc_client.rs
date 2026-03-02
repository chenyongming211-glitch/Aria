use anyhow::Result;
use tonic::transport::{Certificate, Channel, ClientTlsConfig, Identity, Endpoint};
use std::fs;

// 引入生成的 protobuf 代码
pub mod aria {
    tonic::include_proto!("aria.agent");
}

use aria::controller_service_client::ControllerServiceClient;

/// gRPC Controller 客户端
pub struct GrpcClient {
    client: ControllerServiceClient<Channel>,
}

impl GrpcClient {
    /// 创建新的 gRPC 客户端（带 mTLS）
    pub async fn new(
        controller_url: String,
        ca_cert_path: String,
        client_cert_path: String,
        client_key_path: String,
    ) -> Result<Self> {
        // 加载证书
        let ca_cert = fs::read_to_string(&ca_cert_path)?;
        let client_cert = fs::read_to_string(&client_cert_path)?;
        let client_key = fs::read_to_string(&client_key_path)?;
        
        // 创建 TLS 配置
        let ca = Certificate::from_pem(ca_cert);
        let identity = Identity::from_pem(client_cert, client_key);
        
        let tls_config = ClientTlsConfig::new()
            .ca_certificate(ca)
            .identity(identity)
            .domain_name("aria-controller");
        
        // 创建 Endpoint 并配置 TLS
        let endpoint = Endpoint::from_shared(controller_url)?
            .tls_config(tls_config)?;
        
        // 连接服务器
        let channel = endpoint.connect().await?;
        
        // 创建客户端
        let client = ControllerServiceClient::new(channel);
        
        Ok(Self { client })
    }
    
    /// 注册到 Controller
    pub async fn register(
        &mut self,
        public_key: String,
        endpoint: String,
        public_ip: String,
        hostname: String,
        token: String,
        region: String,
    ) -> Result<String> {
        let request = tonic::Request::new(aria::RegisterRequest {
            public_key,
            endpoint,
            private_ip: String::new(),
            public_ip,
            hostname,
            registered_at: std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)?
                .as_secs() as i64,
            token,
            advertised_routes: vec![],
            region,
            customer_id: String::new(),
            runtime_mode: "ebpf".to_string(),
            kernel_version: "5.15.0".to_string(),
            has_aesni: true,
        });
        
        let response = self.client.register(request).await?;
        let resp = response.into_inner();
        
        Ok(resp.assigned_ip)
    }
    
    /// 从 Controller 同步配置
    pub async fn sync(&mut self, public_key: String) -> Result<SyncResult> {
        let request = tonic::Request::new(aria::SyncRequest {
            public_key,
        });
        
        let response = self.client.sync(request).await?;
        let resp = response.into_inner();
        
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
            acl_rules: resp.acl_rules.into_iter().map(|r| AclRule {
                src_net: r.src_net,
                dst_net: r.dst_net,
                protocol: r.protocol,
                min_port: r.min_port,
                max_port: r.max_port,
            }).collect(),
        })
    }
}

/// 同步结果
pub struct SyncResult {
    pub peers: Vec<PeerInfo>,
    pub assigned_ip: String,
    pub acl_rules: Vec<AclRule>,
}

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
    pub max_port: u32,
}
