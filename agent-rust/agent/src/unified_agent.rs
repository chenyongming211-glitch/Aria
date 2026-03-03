use anyhow::{Context, Result};
use std::sync::{Arc, Mutex as StdMutex};
use std::time::Duration;
use std::sync::atomic::{AtomicU8, Ordering};
use tokio::signal;
use tokio::signal::unix::{signal, SignalKind};
use tokio::sync::{Mutex, broadcast};
use tokio_util::sync::CancellationToken;
use aya::{
    include_bytes_aligned,
    programs::{tc, SchedClassifier, TcAttachType, Xdp, XdpFlags},
    EbpfLoader,
};
use serde::{Deserialize, Serialize};
use tracing_subscriber::{EnvFilter, Registry, reload};

use crate::grpc_client::{GrpcClient, PeerInfo as GrpcPeerInfo, AclRule};
use crate::wireguard::{WireGuardManager, PeerConfig, InterfaceConfig};
use crate::routing::RoutingManager;
use crate::acl::AclManager;
use crate::qos::QoSManager;
use crate::identity::IdentityManager;
use crate::metrics;
use crate::config::AgentConfig;

const BPF_FS_PATH: &str = "/sys/fs/bpf/aria";

#[derive(Debug, Clone, Serialize, Deserialize)]
struct UnixRequest {
    cmd: String,
    args: serde_json::Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct UnixResponse {
    success: bool,
    message: Option<String>,
    data: Option<serde_json::Value>,
}

pub struct UnifiedAgent {
    config: AgentConfig,
    
    acl_mgr: Arc<Mutex<AclManager>>,
    qos_mgr: Arc<Mutex<QoSManager>>,
    identity_mgr: Arc<StdMutex<IdentityManager>>,
    
    grpc_client: GrpcClient,
    wg_manager: Arc<Mutex<WireGuardManager>>,
    routing_manager: RoutingManager,
    
    unix_socket_path: String,
    config_update_tx: broadcast::Sender<()>,
    
    last_sync_peers: Vec<GrpcPeerInfo>,
    
    cancel_token: CancellationToken,
    log_handle: Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>,
    current_log_level: Arc<StdMutex<String>>,
}

impl UnifiedAgent {
    pub async fn new(
        config: AgentConfig,
        interface: &str,
        log_handle: Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>,
    ) -> Result<Self> {
        tracing::info!("Creating UnifiedAgent...");
        
        let (acl_mgr, qos_mgr, identity_mgr) = Self::load_ebpf_programs(interface)?;
        tracing::info!("✅ eBPF programs loaded");
        
        let grpc_client = GrpcClient::new(
            config.controller_url.clone(),
            config.ca_cert.clone(),
            config.client_cert.clone(),
            config.client_key.clone(),
        ).await.context("Failed to connect to Controller")?;
        tracing::info!("✅ gRPC client connected");
        
        let wg_manager = Arc::new(Mutex::new(WireGuardManager::new(&config.interface_name)));
        tracing::info!("✅ WireGuard manager created");
        
        let routing_manager = RoutingManager::new(&config.interface_name);
        tracing::info!("✅ Routing manager created");
        
        let (config_update_tx, _) = broadcast::channel(16);
        let cancel_token = CancellationToken::new();
        let current_log_level = Arc::new(StdMutex::new("info".to_string()));
        
        Ok(Self {
            config,
            acl_mgr,
            qos_mgr,
            identity_mgr,
            grpc_client,
            wg_manager,
            routing_manager,
            unix_socket_path: "/run/aria-agent.sock".to_string(),
            config_update_tx,
            last_sync_peers: Vec::new(),
            cancel_token,
            log_handle,
            current_log_level,
        })
    }
    
    fn load_ebpf_programs(interface: &str) -> Result<(
        Arc<Mutex<AclManager>>,
        Arc<Mutex<QoSManager>>,
        Arc<StdMutex<IdentityManager>>,
    )> {
        if !std::path::Path::new(BPF_FS_PATH).exists() {
            std::fs::create_dir_all(BPF_FS_PATH)
                .context("Failed to create bpffs directory")?;
        }
        
        let acl_bytes = include_bytes_aligned!(concat!(env!("OUT_DIR"), "/acl"));
        let qos_bytes = include_bytes_aligned!(concat!(env!("OUT_DIR"), "/qos"));
        
        let mut acl_ebpf = EbpfLoader::new()
            .map_pin_path(BPF_FS_PATH)
            .load(acl_bytes)?;
        
        let program: &mut Xdp = acl_ebpf.program_mut("xdp_ingress_acl")
            .context("XDP program not found")?
            .try_into()?;
        program.load()?;
        program.attach(interface, XdpFlags::default())?;
        
        let identity_mgr = IdentityManager::new(&mut acl_ebpf)?;
        let identity_mgr = Arc::new(StdMutex::new(identity_mgr));
        
        let acl_mgr = AclManager::new(&mut acl_ebpf, identity_mgr.clone())?;
        let acl_mgr = Arc::new(Mutex::new(acl_mgr));
        
        let mut qos_ebpf = EbpfLoader::new()
            .map_pin_path(BPF_FS_PATH)
            .load(qos_bytes)?;
        
        let program: &mut SchedClassifier = qos_ebpf.program_mut("tc_egress_qos")
            .context("TC program not found")?
            .try_into()?;
        program.load()?;
        tc::qdisc_add_clsact(interface)?;
        program.attach(interface, TcAttachType::Egress)?;
        
        let qos_mgr = QoSManager::new(&mut qos_ebpf, identity_mgr.clone())?;
        let qos_mgr = Arc::new(Mutex::new(qos_mgr));
        
        Ok((acl_mgr, qos_mgr, identity_mgr))
    }
    
    pub async fn start(&mut self) -> Result<()> {
        tracing::info!("Starting UnifiedAgent...");
        
        self.ensure_interface().await?;
        
        self.routing_manager.init()
            .context("Failed to initialize routing manager")?;
        
        self.sync().await?;
        tracing::info!("✅ Initial sync completed");
        
        self.start_unix_socket_server()?;
        
        self.run_main_loop().await
    }
    
    async fn ensure_interface(&self) -> Result<()> {
        let mut wg = self.wg_manager.lock().await;
        
        match wg.list_peers() {
            Ok(_) => {
                tracing::info!("WireGuard interface {} already exists, adopting it", 
                    self.config.interface_name);
                Ok(())
            }
            Err(_) => {
                tracing::info!("Creating WireGuard interface {}", self.config.interface_name);
                
                let iface_config = InterfaceConfig {
                    name: self.config.interface_name.clone(),
                    private_key: self.config.private_key.clone(),
                    listen_port: self.config.listen_port,
                    mtu: self.config.mtu,
                    address: self.config.address.clone(),
                };
                
                wg.create_interface(iface_config)
                    .context("Failed to create WireGuard interface")
            }
        }
    }
    
    async fn run_main_loop(&mut self) -> Result<()> {
        tracing::info!("Entering main loop");
        
        let mut sync_interval = tokio::time::interval(self.config.sync_interval);
        let mut metrics_timer = tokio::time::interval(Duration::from_secs(30));
        let mut sighup = signal(SignalKind::hangup())?;
        
        loop {
            tokio::select! {
                _ = sync_interval.tick() => {
                    if let Err(e) = self.sync().await {
                        tracing::error!("Sync failed: {:?}", e);
                        metrics::record_sync_failure();
                    } else {
                        metrics::record_sync_success(self.last_sync_peers.len());
                    }
                }
                
                _ = metrics_timer.tick() => {
                    if let Err(e) = self.collect_and_report_metrics().await {
                        tracing::error!("Metrics collection failed: {:?}", e);
                    }
                }
                
                _ = sighup.recv() => {
                    tracing::info!("SIGHUP received, reloading config...");
                    if let Err(e) = self.reload_config().await {
                        tracing::error!("Config reload failed: {:?}", e);
                    } else {
                        metrics::record_config_reload();
                    }
                }
                
                _ = signal::ctrl_c() => {
                    tracing::info!("Shutting down...");
                    break;
                }
            }
        }
        
        self.cleanup().await?;
        Ok(())
    }
    
    fn start_unix_socket_server(&self) -> Result<()> {
        use tokio::net::UnixListener;
        use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
        
        let acl_mgr = self.acl_mgr.clone();
        let qos_mgr = self.qos_mgr.clone();
        let wg_manager = self.wg_manager.clone();
        let socket_path = self.unix_socket_path.clone();
        let cancel_token = self.cancel_token.clone();
        let log_handle = self.log_handle.clone();
        let current_log_level = self.current_log_level.clone();
        
        if std::path::Path::new(&socket_path).exists() {
            let _ = std::fs::remove_file(&socket_path);
        }
        
        tokio::spawn(async move {
            let listener = match UnixListener::bind(&socket_path) {
                Ok(l) => l,
                Err(e) => {
                    tracing::error!("Failed to bind Unix socket: {}", e);
                    return;
                }
            };
            
            tracing::info!("Unix socket server listening on {}", socket_path);
            
            loop {
                tokio::select! {
                    accept_result = listener.accept() => {
                        match accept_result {
                            Ok((stream, _)) => {
                                let acl_mgr = acl_mgr.clone();
                                let qos_mgr = qos_mgr.clone();
                                let wg_manager = wg_manager.clone();
                                let log_handle = log_handle.clone();
                                let current_log_level = current_log_level.clone();
                                
                                tokio::spawn(async move {
                                    let (reader, mut writer) = stream.into_split();
                                    let mut reader = BufReader::new(reader).lines();
                                    
                                    while let Some(line) = reader.next_line().await.transpose() {
                                        let line = match line {
                                            Ok(l) => l,
                                            Err(_) => break,
                                        };
                                        
                                        if line.is_empty() {
                                            continue;
                                        }
                                        
                                        let response = match serde_json::from_str::<UnixRequest>(&line) {
                                            Ok(req) => {
                                                Self::handle_unix_command(
                                                    req,
                                                    &acl_mgr,
                                                    &qos_mgr,
                                                    &wg_manager,
                                                    &log_handle,
                                                    &current_log_level,
                                                ).await
                                            }
                                            Err(e) => {
                                                format!("{{\"success\":false,\"message\":\"Invalid request: {}\"}}\n", e)
                                            }
                                        };
                                        
                                        if let Err(e) = writer.write_all(response.as_bytes()).await {
                                            tracing::debug!("Failed to write response: {}", e);
                                            break;
                                        }
                                    }
                                });
                            }
                            Err(e) => {
                                tracing::error!("Error accepting connection: {}", e);
                            }
                        }
                    }
                    
                    _ = cancel_token.cancelled() => {
                        tracing::info!("Unix socket server shutting down gracefully");
                        break;
                    }
                }
            }
            
            if let Err(e) = std::fs::remove_file(&socket_path) {
                tracing::warn!("Failed to remove socket file: {}", e);
            }
            
            tracing::info!("Unix socket server stopped");
        });
        
        tracing::info!("Unix socket server started");
        Ok(())
    }
    
    async fn handle_unix_command(
        req: UnixRequest,
        acl_mgr: &Arc<Mutex<AclManager>>,
        qos_mgr: &Arc<Mutex<QoSManager>>,
        wg_manager: &Arc<Mutex<WireGuardManager>>,
        log_handle: &Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>,
        current_log_level: &Arc<StdMutex<String>>,
    ) -> String {
        let response = match req.cmd.as_str() {
            // ===== 状态查询 =====
            "status" | "get_status" => UnixResponse {
                success: true,
                message: Some("Agent running".to_string()),
                data: None,
            },
            "peers" | "get_peers" => {
                let wg = wg_manager.lock().await;
                match wg.list_peers() {
                    Ok(peers) => {
                        let peers_json: Vec<serde_json::Value> = peers
                            .into_iter()
                            .map(|p| {
                                serde_json::json!({
                                    "public_key": p.public_key,
                                    "endpoint": p.endpoint,
                                    "allowed_ips": p.allowed_ips,
                                    "last_handshake_secs": p.last_handshake,
                                })
                            })
                            .collect();
                        UnixResponse {
                            success: true,
                            message: None,
                            data: Some(serde_json::json!({"peers": peers_json, "total": peers_json.len()})),
                        }
                    }
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(format!("Failed to list peers: {}", e)),
                        data: None,
                    },
                }
            }
            "routes" | "get_routes" => {
                let output = tokio::task::spawn_blocking(|| {
                    std::process::Command::new("ip")
                        .args(&["route", "show", "table", "100"])
                        .output()
                }).await;
                
                match output {
                    Ok(Ok(output)) if output.status.success() => {
                        let routes_str = String::from_utf8_lossy(&output.stdout);
                        let routes: Vec<&str> = routes_str.lines().collect();
                        UnixResponse {
                            success: true,
                            message: None,
                            data: Some(serde_json::json!({
                                "routes": routes,
                                "total": routes.len(),
                                "table": 100
                            })),
                        }
                    }
                    Ok(Ok(output)) => UnixResponse {
                        success: false,
                        message: Some(format!("Failed to list routes: {}", 
                            String::from_utf8_lossy(&output.stderr))),
                        data: None,
                    },
                    Ok(Err(e)) => UnixResponse {
                        success: false,
                        message: Some(format!("Failed to execute ip route: {}", e)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(format!("spawn_blocking failed: {}", e)),
                        data: None,
                    },
                }
            }
            "ping" => UnixResponse {
                success: true,
                message: Some("pong".to_string()),
                data: None,
            },
            
            // ===== QoS 管理 =====
            "qos_limit_ip" => {
                let ip = req.args["ip"].as_str().unwrap_or("").to_string();
                let mbps = req.args["mbps"].as_u64().unwrap_or(0);
                
                let mut mgr = qos_mgr.lock().await;
                match mgr.limit_ip(&ip, mbps) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some(format!("Limited {} to {} Mbps", ip, mbps)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_limit_peer" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let mbps = req.args["mbps"].as_u64().unwrap_or(0);
                
                let mut mgr = qos_mgr.lock().await;
                match mgr.limit_peer_pair(&src_ip, &dst_ip, mbps) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some(format!("Limited {} -> {} to {} Mbps", src_ip, dst_ip, mbps)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_limit_port" => {
                let port = req.args["port"].as_u64().unwrap_or(0) as u16;
                let mbps = req.args["mbps"].as_u64().unwrap_or(0);
                let protocol = req.args["protocol"].as_u64().unwrap_or(0) as u8;
                
                let mut mgr = qos_mgr.lock().await;
                match mgr.limit_port(port, mbps, protocol) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some(format!("Limited port {} to {} Mbps", port, mbps)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_limit_service" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let src_port = req.args["src_port"].as_u64().unwrap_or(0) as u16;
                let dst_port = req.args["dst_port"].as_u64().unwrap_or(0) as u16;
                let protocol = req.args["protocol"].as_u64().unwrap_or(6) as u8;
                let mbps = req.args["mbps"].as_u64().unwrap_or(0);
                
                let mut mgr = qos_mgr.lock().await;
                match mgr.limit_service(&src_ip, &dst_ip, src_port, dst_port, protocol, mbps) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some(format!("Limited service to {} Mbps", mbps)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_remove_ip" => {
                let ip = req.args["ip"].as_str().unwrap_or("").to_string();
                let mut mgr = qos_mgr.lock().await;
                match mgr.remove_ip_limit(&ip) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some(format!("Removed limit for {}", ip)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_remove_peer" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let mut mgr = qos_mgr.lock().await;
                match mgr.remove_peer_limit(&src_ip, &dst_ip) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some(format!("Removed peer limit {} -> {}", src_ip, dst_ip)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_remove_service" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let src_port = req.args["src_port"].as_u64().unwrap_or(0) as u16;
                let dst_port = req.args["dst_port"].as_u64().unwrap_or(0) as u16;
                let protocol = req.args["protocol"].as_u64().unwrap_or(6) as u8;
                let mut mgr = qos_mgr.lock().await;
                match mgr.remove_service_limit(&src_ip, &dst_ip, src_port, dst_port, protocol) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some("Removed service limit".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_stats_ip" => {
                let ip = req.args["ip"].as_str().unwrap_or("").to_string();
                let mgr = qos_mgr.lock().await;
                match mgr.get_ip_stats(&ip) {
                    Ok(Some(stats)) => UnixResponse {
                        success: true,
                        message: None,
                        data: Some(serde_json::to_value(stats).unwrap()),
                    },
                    Ok(None) => UnixResponse {
                        success: true,
                        message: Some("No stats found".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_stats_peer" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let mgr = qos_mgr.lock().await;
                match mgr.get_peer_stats(&src_ip, &dst_ip) {
                    Ok(Some(stats)) => UnixResponse {
                        success: true,
                        message: None,
                        data: Some(serde_json::to_value(stats).unwrap()),
                    },
                    Ok(None) => UnixResponse {
                        success: true,
                        message: Some("No peer stats found".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "qos_stats_service" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let src_port = req.args["src_port"].as_u64().unwrap_or(0) as u16;
                let dst_port = req.args["dst_port"].as_u64().unwrap_or(0) as u16;
                let protocol = req.args["protocol"].as_u64().unwrap_or(6) as u8;
                
                let mgr = qos_mgr.lock().await;
                match mgr.get_service_stats(&src_ip, &dst_ip, src_port, dst_port, protocol) {
                    Ok(Some(stats)) => UnixResponse {
                        success: true,
                        message: None,
                        data: Some(serde_json::to_value(stats).unwrap()),
                    },
                    Ok(None) => UnixResponse {
                        success: true,
                        message: Some("No service stats found".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            
            // ===== ACL 管理 =====
            "acl_allow" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let dst_port = req.args["dst_port"].as_u64().unwrap_or(0) as u16;
                let protocol = req.args["protocol"].as_u64().unwrap_or(0) as u8;
                
                let mut mgr = acl_mgr.lock().await;
                match mgr.allow(&src_ip, &dst_ip, dst_port, protocol) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some("ACL allow rule added".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_deny" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let dst_port = req.args["dst_port"].as_u64().unwrap_or(0) as u16;
                let protocol = req.args["protocol"].as_u64().unwrap_or(0) as u8;
                
                let mut mgr = acl_mgr.lock().await;
                match mgr.deny(&src_ip, &dst_ip, dst_port, protocol) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some("ACL deny rule added".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_block_src_ip" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let mut mgr = acl_mgr.lock().await;
                match mgr.block_src_ip(&src_ip) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some("Blocked source IP".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_block_dst_ip" => {
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let mut mgr = acl_mgr.lock().await;
                match mgr.block_dst_ip(&dst_ip) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some("Blocked destination IP".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_block_port" => {
                let port = req.args["port"].as_u64().unwrap_or(0) as u16;
                let mut mgr = acl_mgr.lock().await;
                match mgr.block_port(port) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some(format!("Blocked port {}", port)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_unblock_src_ip" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let mut mgr = acl_mgr.lock().await;
                match mgr.unblock_src_ip(&src_ip) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some("Unblocked source IP".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_unblock_dst_ip" => {
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let mut mgr = acl_mgr.lock().await;
                match mgr.unblock_dst_ip(&dst_ip) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some("Unblocked destination IP".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_unblock_port" => {
                let port = req.args["port"].as_u64().unwrap_or(0) as u16;
                let mut mgr = acl_mgr.lock().await;
                match mgr.unblock_port(port) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some(format!("Unblocked port {}", port)),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_remove_rule" => {
                let src_ip = req.args["src_ip"].as_str().unwrap_or("").to_string();
                let dst_ip = req.args["dst_ip"].as_str().unwrap_or("").to_string();
                let dst_port = req.args["dst_port"].as_u64().unwrap_or(0) as u16;
                let protocol = req.args["protocol"].as_u64().unwrap_or(0) as u8;
                
                let mut mgr = acl_mgr.lock().await;
                match mgr.remove_rule(&src_ip, &dst_ip, dst_port, protocol) {
                    Ok(_) => UnixResponse {
                        success: true,
                        message: Some("ACL rule removed".to_string()),
                        data: None,
                    },
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(e.to_string()),
                        data: None,
                    },
                }
            }
            "acl_stats" => {
                let mgr = acl_mgr.lock().await;
                match mgr.get_all_rule_stats() {
                    Ok(stats) => {
                        let stats_json: Vec<serde_json::Value> = stats
                            .into_iter()
                            .map(|(key, value)| {
                                serde_json::json!({
                                    "src_id": key.src_id,
                                    "dst_id": key.dst_id,
                                    "protocol": key.protocol,
                                    "dst_port": key.dst_port,
                                    "action": value.action,
                                    "packets": value.packets,
                                    "bytes": value.bytes,
                                })
                            })
                            .collect();
                        UnixResponse {
                            success: true,
                            message: None,
                            data: Some(serde_json::json!({"rules": stats_json, "total": stats_json.len()})),
                        }
                    }
                    Err(e) => UnixResponse {
                        success: false,
                        message: Some(format!("Failed to get ACL stats: {}", e)),
                        data: None,
                    },
                }
            }
            
            // ===== 日志管理 =====
            "set_log_level" => {
                let level = req.args["level"].as_str().unwrap_or("info").to_string();
                
                let handle = log_handle.lock().unwrap();
                if let Some(handle) = handle.as_ref() {
                    let new_filter = match level.as_str() {
                        "trace" => EnvFilter::new("trace"),
                        "debug" => EnvFilter::new("debug"),
                        "info" => EnvFilter::new("info"),
                        "warn" => EnvFilter::new("warn"),
                        "error" => EnvFilter::new("error"),
                        _ => EnvFilter::new("info"),
                    };
                    
                    match handle.reload(new_filter) {
                        Ok(_) => {
                            let mut current = current_log_level.lock().unwrap();
                            *current = level.clone();
                            drop(current);
                            drop(handle);
                            
                            tracing::info!("Log level updated to {}", level);
                            UnixResponse {
                                success: true,
                                message: Some(format!("Log level updated to {}", level)),
                                data: None,
                            }
                        }
                        Err(e) => {
                            UnixResponse {
                                success: false,
                                message: Some(format!("Failed to update log level: {}", e)),
                                data: None,
                            }
                        }
                    }
                } else {
                    UnixResponse {
                        success: false,
                        message: Some("Log handle not available".to_string()),
                        data: None,
                    }
                }
            }
            "get_log_level" => {
                let current = current_log_level.lock().unwrap();
                let level = current.clone();
                drop(current);
                
                UnixResponse {
                    success: true,
                    message: None,
                    data: Some(serde_json::json!({"level": level})),
                }
            }
            
            // ===== 未知命令 =====
            _ => UnixResponse {
                success: false,
                message: Some(format!("Unknown command: {}", req.cmd)),
                data: None,
            },
        };
        
        serde_json::to_string(&response).unwrap_or_else(|_| {
            "{\"success\":false,\"message\":\"Failed to serialize response\"}".to_string()
        }) + "\n"
    }
    
    pub async fn sync(&mut self) -> Result<()> {
        tracing::debug!("Syncing with Controller...");
        
        let sync_result = self.grpc_client
            .sync(self.config.public_key.clone())
            .await?;
        
        tracing::debug!("Sync received: {} peers, {} ACL rules", 
            sync_result.peers.len(), sync_result.acl_rules.len());
        
        self.sync_peers(&sync_result.peers).await?;
        self.sync_advertised_routes(&sync_result.peers).await?;
        
        if !sync_result.acl_rules.is_empty() {
            if let Err(e) = self.sync_acl_rules(&sync_result.acl_rules).await {
                tracing::error!("Failed to sync ACL rules: {:?}", e);
            }
        }
        
        self.last_sync_peers = sync_result.peers;
        tracing::debug!("Sync completed");
        Ok(())
    }
    
    async fn sync_peers(&mut self, new_peers: &[GrpcPeerInfo]) -> Result<()> {
        let wg_manager = self.wg_manager.clone();
        let new_peers = new_peers.to_vec();
        let interface_name = self.config.interface_name.clone();
        
        let result = tokio::task::spawn_blocking(move || -> Result<(usize, usize, usize, usize)> {
            let mut wg = wg_manager.blocking_lock();
            
            let current_peers = wg.list_peers()
                .context("Failed to list current peers")?;
            
            // 计算 peer 差异
            let (to_add, to_remove, to_update) = Self::diff_peers_static(&current_peers, &new_peers);
            
            // 删除 peer
            for peer in &to_remove {
                tracing::info!("Removing peer: {}...", &peer[..16.min(peer.len())]);
                wg.remove_peer(&peer)
                    .context("Failed to remove peer")?;
                metrics::record_wireguard_peer_change("remove");
            }
            
            // 添加 peer
            for peer in &to_add {
                tracing::info!("Adding peer: {}...", &peer.public_key[..16.min(peer.public_key.len())]);
                
                let peer_config = PeerConfig {
                    public_key: peer.public_key.clone(),
                    endpoint: if peer.endpoint.is_empty() { None } else { Some(peer.endpoint.clone()) },
                    allowed_ips: vec![format!("{}/32", peer.assigned_ip)],
                    persistent_keepalive: 25,
                };
                
                wg.add_peer(peer_config)
                    .context("Failed to add peer")?;
                metrics::record_wireguard_peer_change("add");
            }
            
            // 更新 peer
            for peer in &to_update {
                tracing::debug!("Updating peer: {}...", &peer.public_key[..16.min(peer.public_key.len())]);
                
                wg.remove_peer(&peer.public_key)
                    .context("Failed to remove peer for update")?;
                
                let peer_config = PeerConfig {
                    public_key: peer.public_key.clone(),
                    endpoint: if peer.endpoint.is_empty() { None } else { Some(peer.endpoint.clone()) },
                    allowed_ips: vec![format!("{}/32", peer.assigned_ip)],
                    persistent_keepalive: 25,
                };
                
                wg.add_peer(peer_config)
                    .context("Failed to add updated peer")?;
                metrics::record_wireguard_peer_change("update");
            }
            
            Ok((to_add.len(), to_remove.len(), to_update.len(), new_peers.len()))
        }).await?;
        
        match result {
            Ok((added, removed, updated, total)) => {
                let total_changes = added + removed + updated;
                if total_changes > 0 {
                    tracing::info!(
                        "Peer sync: +{} -{} ~{} (total: {})",
                        added, removed, updated, total
                    );
                }
            }
            Err(e) => {
                tracing::error!("Peer sync failed: {:?}", e);
                return Err(e);
            }
        }
        
        Ok(())
    }
    
    fn diff_peers_static(
        current: &[crate::wireguard::PeerInfo],
        desired: &[GrpcPeerInfo],
    ) -> (Vec<GrpcPeerInfo>, Vec<String>, Vec<GrpcPeerInfo>) {
        let mut to_add = Vec::new();
        let mut to_remove = Vec::new();
        let mut to_update = Vec::new();
        
        for desired_peer in desired {
            let current_peer = current.iter().find(|p| p.public_key == desired_peer.public_key);
            
            if let Some(current) = current_peer {
                let desired_endpoint = if desired_peer.endpoint.is_empty() {
                    None
                } else {
                    Some(desired_peer.endpoint.as_str())
                };
                
                if current.endpoint.as_deref() != desired_endpoint ||
                   current.allowed_ips.get(0).map(|s| s.as_str()) != Some(&desired_peer.assigned_ip) {
                    to_update.push(desired_peer.clone());
                }
            } else {
                to_add.push(desired_peer.clone());
            }
        }
        
        for current_peer in current {
            if !desired.iter().any(|p| p.public_key == current_peer.public_key) {
                to_remove.push(current_peer.public_key.clone());
            }
        }
        
        (to_add, to_remove, to_update)
    }
    
    async fn sync_advertised_routes(&mut self, peers: &[GrpcPeerInfo]) -> Result<()> {
        use std::collections::HashSet as StdHashSet;
        
        // 收集期望的所有路由（所有 peer 宣告的非 /32 路由）
        let mut desired_routes = StdHashSet::new();
        for peer in peers {
            for route in &peer.advertised_routes {
                if !route.ends_with("/32") {
                    desired_routes.insert(route.clone());
                }
            }
        }
        
        // 在单个阻塞任务中完成所有路由操作，保证原子性和性能
        let routing_manager = self.routing_manager.clone();
        let result = tokio::task::spawn_blocking(move || -> Result<(usize, usize, usize)> {
            // 获取当前路由
            let current_routes = routing_manager.list_vpn_routes()
                .context("Failed to list current VPN routes")?;
            
            // 计算差异
            let to_add: Vec<_> = desired_routes.difference(&current_routes).cloned().collect();
            let to_remove: Vec<_> = current_routes.difference(&desired_routes).cloned().collect();
            
            let mut added_count = 0;
            let mut removed_count = 0;
            
            // 添加缺失的路由
            for route in to_add {
                if let Err(e) = routing_manager.add_vpn_route(&route) {
                    tracing::error!("Failed to add advertised route {}: {:?}", route, e);
                } else {
                    added_count += 1;
                    tracing::info!("Added advertised route: {}", route);
                }
            }
            
            // 删除多余的路由
            for route in to_remove {
                if let Err(e) = routing_manager.remove_vpn_route(&route) {
                    tracing::error!("Failed to remove stale route {}: {:?}", route, e);
                } else {
                    removed_count += 1;
                    tracing::info!("Removed stale route: {}", route);
                }
            }
            
            Ok((added_count, removed_count, desired_routes.len()))
        }).await?;
        
        match result {
            Ok((added_count, removed_count, total_count)) => {
                if added_count > 0 || removed_count > 0 {
                    tracing::info!(
                        "Route sync completed: {} routes added, {} routes removed, {} routes total",
                        added_count, removed_count, total_count
                    );
                }
            }
            Err(e) => {
                tracing::error!("Route sync failed: {:?}", e);
                return Err(e);
            }
        }
        
        Ok(())
    }
    
    async fn sync_acl_rules(&mut self, rules: &[AclRule]) -> Result<()> {
        let mut acl = self.acl_mgr.lock().await;
        
        acl.clear_all_rules()?;
        
        for rule in rules {
            acl.allow(
                &rule.src_net,
                &rule.dst_net,
                rule.min_port as u16,
                rule.protocol as u8,
            )?;
        }
        
        metrics::record_acl_rule_count(rules.len());
        tracing::info!("Synced {} ACL rules", rules.len());
        Ok(())
    }
    
    async fn collect_and_report_metrics(&self) -> Result<()> {
        tracing::trace!("Collecting metrics...");
        
        let wg_manager = self.wg_manager.clone();
        let acl_mgr = self.acl_mgr.clone();
        let qos_mgr = self.qos_mgr.clone();
        
        let result = tokio::task::spawn_blocking(move || -> Result<(
            Vec<(String, Option<String>, u64, u64, Option<u64>)>, // peers
            (usize, usize, u64, u64), // wg totals
            Vec<(u32, &str, u64, u64)>, // acl stats
            Vec<(&str, u32, u64, u64)> // qos stats
        )> {
            // 收集 WireGuard 统计
            let wg = wg_manager.blocking_lock();
            let stats = wg.get_stats()?;
            
            let total_rx: u64 = stats.peers.iter().map(|p| p.rx_bytes).sum();
            let total_tx: u64 = stats.peers.iter().map(|p| p.tx_bytes).sum();
            let active_peers = stats.peers.iter().filter(|p| {
                p.last_handshake.map(|hs| hs > 0 && hs < 180).unwrap_or(false)
            }).count();
            
            let wg_totals = (stats.peers.len(), active_peers, total_rx, total_tx);
            
            let peer_stats: Vec<_> = stats.peers.iter().map(|p| {
                (p.public_key.clone(), p.endpoint.clone(), p.rx_bytes, p.tx_bytes, p.last_handshake)
            }).collect();
            
            drop(wg);
            
            // 收集 ACL 统计
            let acl = acl_mgr.blocking_lock();
            let acl_stats: Vec<_> = acl.get_all_rule_stats()
                .unwrap_or_default()
                .into_iter()
                .map(|(_, value)| {
                    let action = if value.action == 1 { "pass" } else { "drop" };
                    (value.rule_id, action, value.packets, value.bytes)
                })
                .collect();
            drop(acl);
            
            // 收集 QoS 统计
            let qos = qos_mgr.blocking_lock();
            let qos_stats: Vec<_> = qos.get_all_qos_stats()
                .unwrap_or_default()
                .into_iter()
                .map(|(rule_type, rule_id, passed, dropped)| {
                    // 将 String 转换为 &'static str 需要特殊处理
                    match rule_type.as_str() {
                        "ip" => ("ip", rule_id, passed, dropped),
                        "peer" => ("peer", rule_id, passed, dropped),
                        "service" => ("service", rule_id, passed, dropped),
                        _ => ("unknown", rule_id, passed, dropped),
                    }
                })
                .collect();
            drop(qos);
            
            Ok((peer_stats, wg_totals, acl_stats, qos_stats))
        }).await?;
        
        // 在异步上下文中记录 metrics
        match result {
            Ok((peer_stats, wg_totals, acl_stats, qos_stats)) => {
                // WireGuard metrics
                metrics::record_wireguard_totals(wg_totals.0, wg_totals.1, wg_totals.2, wg_totals.3);
                
                for (public_key, endpoint, rx_bytes, tx_bytes, last_handshake) in peer_stats {
                    metrics::record_wireguard_peer_stats(
                        &public_key,
                        endpoint.as_deref(),
                        rx_bytes,
                        tx_bytes,
                        last_handshake,
                    );
                }
                
                // ACL metrics
                for (rule_id, action, packets, bytes) in acl_stats {
                    metrics::record_acl_rule_stats(rule_id, action, packets, bytes);
                }
                
                // QoS metrics
                for (rule_type, rule_id, passed, dropped) in qos_stats {
                    metrics::record_qos_rule_stats(rule_type, rule_id, passed, dropped);
                }
            }
            Err(e) => {
                tracing::error!("Failed to collect metrics: {:?}", e);
            }
        }
        
        let uptime = metrics::record_agent_uptime();
        tracing::trace!("Metrics collected (uptime: {}s)", uptime);
        
        Ok(())
    }
    
    async fn reconnect_grpc(&mut self) -> Result<()> {
        tracing::info!("Reconnecting to Controller at {}...", self.config.controller_url);
        
        let new_client = GrpcClient::new(
            self.config.controller_url.clone(),
            self.config.ca_cert.clone(),
            self.config.client_cert.clone(),
            self.config.client_key.clone(),
        ).await.context("Failed to reconnect to Controller")?;
        
        self.grpc_client = new_client;
        tracing::info!("✅ gRPC client reconnected successfully");
        Ok(())
    }
    
    async fn reload_config(&mut self) -> Result<()> {
        tracing::info!("Reloading configuration...");
        
        let config_manager = crate::config::ConfigManager::new("/etc/aria/agent.yaml");
        let new_config = config_manager.load()?;
        
        // 1. 检测 sync_interval 变更
        if new_config.sync_interval != self.config.sync_interval {
            tracing::info!("Sync interval changed: {:?} -> {:?}", 
                self.config.sync_interval, new_config.sync_interval);
            self.config.sync_interval = new_config.sync_interval;
        }
        
        // 2. 检测 Controller URL 变更（需要重连）
        if new_config.controller_url != self.config.controller_url {
            tracing::warn!("Controller URL changed: {} -> {}", 
                self.config.controller_url, new_config.controller_url);
            
            // 备份旧配置以便回滚
            let old_url = self.config.controller_url.clone();
            let old_ca = self.config.ca_cert.clone();
            let old_cert = self.config.client_cert.clone();
            let old_key = self.config.client_key.clone();
            
            // 更新配置
            self.config.controller_url = new_config.controller_url.clone();
            self.config.ca_cert = new_config.ca_cert.clone();
            self.config.client_cert = new_config.client_cert.clone();
            self.config.client_key = new_config.client_key.clone();
            
            // 尝试重连
            if let Err(e) = self.reconnect_grpc().await {
                tracing::error!("Failed to reconnect gRPC client, rolling back config: {:?}", e);
                
                // 回滚配置
                self.config.controller_url = old_url;
                self.config.ca_cert = old_ca;
                self.config.client_cert = old_cert;
                self.config.client_key = old_key;
                
                metrics::record_grpc_error();
                metrics::record_config_reload_failure();
            }
        }
        
        // 3. 检测证书路径变更
        else if new_config.ca_cert != self.config.ca_cert ||
                new_config.client_cert != self.config.client_cert ||
                new_config.client_key != self.config.client_key {
            tracing::warn!("Certificate paths changed, reconnecting gRPC client");
            
            // 备份旧证书路径
            let old_ca = self.config.ca_cert.clone();
            let old_cert = self.config.client_cert.clone();
            let old_key = self.config.client_key.clone();
            
            // 更新证书路径
            self.config.ca_cert = new_config.ca_cert.clone();
            self.config.client_cert = new_config.client_cert.clone();
            self.config.client_key = new_config.client_key.clone();
            
            // 尝试重连
            if let Err(e) = self.reconnect_grpc().await {
                tracing::error!("Failed to reconnect gRPC client with new certificates, rolling back: {:?}", e);
                
                // 回滚证书路径
                self.config.ca_cert = old_ca;
                self.config.client_cert = old_cert;
                self.config.client_key = old_key;
                
                metrics::record_grpc_error();
                metrics::record_config_reload_failure();
            }
        }
        
        // 4. 检测 WireGuard 配置变更
        if new_config.listen_port != self.config.listen_port {
            tracing::warn!("Listen port changed: {} -> {}", 
                self.config.listen_port, new_config.listen_port);
        }
        
        if new_config.mtu != self.config.mtu {
            tracing::warn!("MTU changed: {} -> {}", 
                self.config.mtu, new_config.mtu);
        }
        
        // 5. 检测不应动态变更的配置
        if new_config.public_key != self.config.public_key ||
           new_config.private_key != self.config.private_key {
            tracing::error!("Public/private key cannot be changed dynamically, ignoring");
        }
        
        if new_config.interface_name != self.config.interface_name {
            tracing::error!("Interface name cannot be changed dynamically, ignoring");
        }
        
        // 更新其他可安全变更的配置
        self.config.region = new_config.region;
        self.config.advertised_routes = new_config.advertised_routes;
        self.config.hostname = new_config.hostname;
        
        tracing::info!("Configuration reloaded successfully");
        Ok(())
    }
    
    async fn cleanup(&self) -> Result<()> {
        tracing::info!("Cleaning up...");
        
        let map_names = [
            "SRC_IPV4_ID_MAP", "DST_IPV4_ID_MAP",
            "SRC_IPV6_ID_MAP", "DST_IPV6_ID_MAP",
        ];
        
        for map_name in map_names {
            let pin_path = format!("{}/{}", BPF_FS_PATH, map_name);
            if std::path::Path::new(&pin_path).exists() {
                let _ = std::fs::remove_file(&pin_path);
            }
        }
        
        if std::path::Path::new(&self.unix_socket_path).exists() {
            let _ = std::fs::remove_file(&self.unix_socket_path);
        }
        
        tracing::info!("Cleanup completed");
        Ok(())
    }
}
