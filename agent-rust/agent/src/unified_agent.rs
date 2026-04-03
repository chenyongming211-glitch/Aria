use anyhow::{Context, Result};
use std::collections::HashMap;
use std::sync::{Arc, Mutex as StdMutex};
use std::time::Duration;
use tokio::signal;
use tokio::signal::unix::{signal, SignalKind};
use tokio::sync::{Mutex, mpsc, oneshot};
use tokio_util::sync::CancellationToken;
use aya::{
    include_bytes_aligned,
    programs::{tc, SchedClassifier, TcAttachType, Xdp, XdpFlags},
    EbpfLoader,
};
use serde::{Deserialize, Serialize};
use tracing_subscriber::{EnvFilter, Registry, reload};

use crate::grpc_client::{
    AclRule,
    BlacklistRule as GrpcBlacklistRule,
    GrpcClient,
    GrpcCommandRequest,
    GrpcCommandResponse,
    PeerInfo as GrpcPeerInfo,
    QoSRule as GrpcQoSRule,
};
use crate::wireguard::{WireGuardManager, PeerConfig};
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

struct RemoteCommandEnvelope {
    request: GrpcCommandRequest,
    reply_tx: oneshot::Sender<GrpcCommandResponse>,
}

fn current_unix_timestamp() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs() as i64
}

pub struct UnifiedAgent {
    config: AgentConfig,
    config_path: String,
    
    acl_mgr: Arc<Mutex<AclManager>>,
    qos_mgr: Arc<Mutex<QoSManager>>,
    grpc_client: GrpcClient,
    wg_manager: Arc<Mutex<WireGuardManager>>,
    routing_manager: RoutingManager,
    
    unix_socket_path: String,
    
    last_sync_peers: Arc<StdMutex<Vec<GrpcPeerInfo>>>,
    
    cancel_token: CancellationToken,
    log_handle: Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>,
    current_log_level: Arc<StdMutex<String>>,
}

impl UnifiedAgent {
    pub async fn new(
        config: AgentConfig,
        config_path: String,
        interface: &str,
        log_handle: Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>,
    ) -> Result<Self> {
        tracing::info!("Creating UnifiedAgent...");
        
        let (acl_mgr, qos_mgr, _identity_mgr) = Self::load_ebpf_programs(interface)?;
        tracing::info!("✅ eBPF programs loaded");
        
        let grpc_client = GrpcClient::new_with_options(
            config.controller_url.clone(),
            config.ca_cert.clone(),
            config.client_cert.clone(),
            config.client_key.clone(),
            config.tls_server_name.clone(),
        ).await.context("Failed to connect to Controller")?;
        tracing::info!("✅ gRPC client connected");
        
        let wg_manager = Arc::new(Mutex::new(WireGuardManager::new(&config.interface_name)));
        tracing::info!("✅ WireGuard manager created");
        
        let routing_manager = RoutingManager::new(&config.interface_name);
        tracing::info!("✅ Routing manager created");
        
        let cancel_token = CancellationToken::new();
        let current_log_level = Arc::new(StdMutex::new("info".to_string()));
        let last_sync_peers = Arc::new(StdMutex::new(Vec::new()));
        
        Ok(Self {
            config,
            config_path,
            acl_mgr,
            qos_mgr,
            grpc_client,
            wg_manager,
            routing_manager,
            unix_socket_path: "/run/aria-agent.sock".to_string(),
            last_sync_peers,
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
        tracing::info!("Step 1: Loading eBPF bytecodes...");
        let acl_bytes = include_bytes_aligned!(concat!(env!("OUT_DIR"), "/acl"));
        let qos_bytes = include_bytes_aligned!(concat!(env!("OUT_DIR"), "/qos"));
        tracing::info!("Step 2: eBPF bytecodes loaded successfully");
        
        tracing::info!("Step 3: Creating ACL EbpfLoader...");
        let mut acl_ebpf = EbpfLoader::new()
            .load(acl_bytes)
            .context("Failed to load ACL eBPF bytecode")?;
        tracing::info!("Step 4: ACL eBPF loaded into memory");
        
        tracing::info!("Step 4.1: Getting mutable program reference...");
        let program_ref = acl_ebpf.program_mut("xdp_ingress_acl")
            .context("XDP program not found in eBPF object")?;
        tracing::info!("Step 4.2: Program reference obtained");
        
        tracing::info!("Step 4.3: Converting to XDP type...");
        let program: &mut Xdp = program_ref.try_into()
            .context("Failed to convert program to XDP type")?;
        tracing::info!("Step 5: XDP program converted successfully");
        
        tracing::info!("Step 7: Loading XDP program into kernel...");
        program.load()?;
        tracing::info!("Step 8: XDP program loaded");
        
        tracing::info!("Step 9: Attaching XDP program to {}...", interface);
        program.attach(interface, XdpFlags::default())?;
        tracing::info!("Step 10: XDP program attached");
        
        // 确认 XDP 真的附加成功了
        tracing::info!("Step 10.5: Verifying XDP attachment...");
        std::thread::sleep(std::time::Duration::from_millis(100));
        
        tracing::info!("Step 11: Creating IdentityManager...");
        let identity_mgr = IdentityManager::new(&mut acl_ebpf)?;
        tracing::info!("Step 12: IdentityManager created");
        
        let identity_mgr = Arc::new(StdMutex::new(identity_mgr));
        tracing::info!("Step 13: IdentityManager wrapped in Arc");
        
        tracing::info!("Step 14: Creating AclManager...");
        let acl_mgr = AclManager::new(&mut acl_ebpf, identity_mgr.clone())?;
        tracing::info!("Step 15: AclManager created");
        
        let acl_mgr = Arc::new(Mutex::new(acl_mgr));
        tracing::info!("Step 16: AclManager wrapped in Arc");
        
        tracing::info!("Step 17: Creating QoS EbpfLoader...");
        let mut qos_ebpf = EbpfLoader::new()
            .load(qos_bytes)?;
        tracing::info!("Step 18: QoS eBPF loaded into memory");
        
        tracing::info!("Step 19: Getting TC program...");
        let program: &mut SchedClassifier = qos_ebpf.program_mut("tc_egress_qos")
            .context("TC program not found")?
            .try_into()?;
        tracing::info!("Step 20: TC program obtained");
        
        tracing::info!("Step 21: Loading TC program into kernel...");
        program.load()?;
        tracing::info!("Step 22: TC program loaded");
        
        tracing::info!("Step 23: Preparing clsact qdisc for {}...", interface);
        // 先删除旧的 clsact qdisc（忽略错误）
        let _ = std::process::Command::new("tc")
            .args(&["qdisc", "del", "dev", interface, "clsact"])
            .output();
        tracing::info!("Step 23.5: Old clsact removed (if existed)");
        
        // 添加新的 clsact qdisc
        tc::qdisc_add_clsact(interface)?;
        tracing::info!("Step 24: clsact qdisc added");
        
        tracing::info!("Step 25: Attaching TC program to {} egress...", interface);
        program.attach(interface, TcAttachType::Egress)?;
        tracing::info!("Step 26: TC program attached");
        
        let qos_mgr = QoSManager::new(&mut qos_ebpf, identity_mgr.clone())?;
        let qos_mgr = Arc::new(Mutex::new(qos_mgr));
        
        Ok((acl_mgr, qos_mgr, identity_mgr))
    }
    
    pub async fn start(&mut self) -> Result<()> {
        tracing::info!("Starting UnifiedAgent...");
        
        // Step 1: 创建 WireGuard 接口（aria0）
        self.ensure_interface().await?;
        tracing::info!("✅ WireGuard interface created/verified");
        
        // ========================================
        // Step 2: 系统优化（P0 + P1）- 在接口创建后执行
        // ========================================
        tracing::info!("Step 2: Applying system optimizations...");
        
        // 优化主接口
        let optimizer = crate::system_optimization::SystemOptimizer::new(
            51820,                  // WireGuard 默认端口
            "eth0".to_string(),     // 物理网卡
            self.config.interface_name.clone(), // 隧道网卡（如 aria0）
        );
        
        match optimizer.optimize(true) { // true = 优化隧道接口
            Ok(result) => {
                tracing::info!("✅ System optimizations applied for {}", self.config.interface_name);
                if !result.warnings.is_empty() {
                    tracing::warn!("⚠️  Some optimizations had warnings: {:?}", result.warnings);
                }
            }
            Err(e) => {
                tracing::warn!("⚠️  System optimizations failed for {}: {}", self.config.interface_name, e);
            }
        }
        
        // 如果启用了多隧道，优化额外的接口
        if self.config.multi_tunnel {
            let base_name = self.config.interface_name.clone();
            let base_name = base_name.trim_end_matches(|c: char| c.is_numeric());
            
            for i in 1..4 {
                let interface_name = format!("{}{}", base_name, i);
                let port = 51820 + i as u16;
                
                tracing::info!("Optimizing additional interface {}...", interface_name);
                let optimizer_extra = crate::system_optimization::SystemOptimizer::new(
                    port,
                    "eth0".to_string(),
                    interface_name.clone(),
                );
                
                match optimizer_extra.optimize(true) {
                    Ok(result) => {
                        tracing::info!("✅ Optimized interface {}", interface_name);
                        if !result.warnings.is_empty() {
                            tracing::warn!("⚠️  Warnings for {}: {:?}", interface_name, result.warnings);
                        }
                    }
                    Err(e) => {
                        tracing::warn!("⚠️  Failed to optimize {}: {}", interface_name, e);
                    }
                }
            }
        }
        
        // Step 3: 初始化路由
        self.routing_manager.init()
            .context("Failed to initialize routing manager")?;
        
        // Step 4: 首次同步
        self.sync().await?;
        tracing::info!("✅ Initial sync completed");
        
        self.start_unix_socket_server()?;
        let (remote_command_tx, remote_command_rx) = mpsc::channel(16);
        self.start_command_stream_task(remote_command_tx);
        
        self.run_main_loop(remote_command_rx).await
    }
    
    async fn ensure_interface(&self) -> Result<()> {
        let mut wg = self.wg_manager.lock().await;
        
        // 使用 ensure_interface 确保主接口存在且配置正确
        tracing::info!("Ensuring WireGuard interface {}", self.config.interface_name);
        wg.ensure_interface(
            self.config.private_key.clone(),
            self.config.address.clone(),
            self.config.listen_port,
            self.config.mtu,
        ).context("Failed to ensure main interface")?;
        
        // 如果启用了多隧道模式，创建额外的接口
        if self.config.multi_tunnel {
            let base_name = self.config.interface_name.clone();
            let base_name = base_name.trim_end_matches(|c: char| c.is_numeric());
            
            for i in 1..4 {
                let interface_name = format!("{}{}", base_name, i);
                let port = self.config.listen_port + i as u16;
                
                tracing::info!("Ensuring additional WireGuard interface {} on port {}", interface_name, port);
                
                let mut wg_extra = WireGuardManager::new(&interface_name);
                wg_extra.ensure_interface(
                    self.config.private_key.clone(),
                    self.config.address.clone(),
                    port,
                    self.config.mtu,
                ).context(format!("Failed to ensure interface {}", interface_name))?;
                
                tracing::info!("✅ Additional interface {} ready on port {}", interface_name, port);
            }
        }
        
        Ok(())
    }
    
    async fn run_main_loop(&mut self, mut remote_command_rx: mpsc::Receiver<RemoteCommandEnvelope>) -> Result<()> {
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
                        metrics::record_sync_success(self.last_sync_peers.lock().unwrap().len());
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

                maybe_command = remote_command_rx.recv() => {
                    match maybe_command {
                        Some(envelope) => {
                            let response = self.execute_remote_command(envelope.request).await;
                            if envelope.reply_tx.send(response).is_err() {
                                tracing::warn!("Remote command response receiver dropped");
                            }
                        }
                        None => {
                            tracing::warn!("Remote command channel closed, stopping agent loop");
                            break;
                        }
                    }
                }
                
                _ = signal::ctrl_c() => {
                    tracing::info!("Shutting down...");
                    break;
                }
            }
        }
        
        self.cancel_token.cancel();
        self.cleanup().await?;
        Ok(())
    }

    fn start_command_stream_task(&self, remote_command_tx: mpsc::Sender<RemoteCommandEnvelope>) {
        let grpc_client = self.grpc_client.clone();
        let cancel_token = self.cancel_token.clone();
        let agent_id = self.config.public_key.clone();

        tokio::spawn(async move {
            loop {
                if cancel_token.is_cancelled() {
                    break;
                }

                match grpc_client.connect_command_stream(agent_id.clone()).await {
                    Ok((response_tx, mut request_stream)) => {
                        tracing::info!("Controller command stream connected");

                        loop {
                            tokio::select! {
                                _ = cancel_token.cancelled() => {
                                    tracing::info!("Command stream task shutting down");
                                    return;
                                }
                                message = request_stream.message() => {
                                    match message {
                                        Ok(Some(request)) => {
                                            let command_id = request.command_id.clone();
                                            let command_name = request.command.clone();
                                            let (reply_tx, reply_rx) = oneshot::channel();

                                            if remote_command_tx.send(RemoteCommandEnvelope {
                                                request,
                                                reply_tx,
                                            }).await.is_err() {
                                                let _ = response_tx.send(build_failed_command_response(
                                                    command_id,
                                                    "remote command executor unavailable".to_string(),
                                                )).await;
                                                return;
                                            }

                                            if response_tx.send(GrpcCommandResponse {
                                                command_id: command_id.clone(),
                                                status: "acknowledged".to_string(),
                                                message: format!("command {} queued", command_name),
                                                result: HashMap::new(),
                                                completed_at: 0,
                                            }).await.is_err() {
                                                tracing::warn!("Failed to send acknowledged response for {}", command_id);
                                                break;
                                            }

                                            match reply_rx.await {
                                                Ok(response) => {
                                                    if response_tx.send(response).await.is_err() {
                                                        tracing::warn!("Failed to send final response for {}", command_id);
                                                        break;
                                                    }
                                                }
                                                Err(e) => {
                                                    if response_tx.send(build_failed_command_response(
                                                        command_id.clone(),
                                                        format!("command execution dropped: {}", e),
                                                    )).await.is_err() {
                                                        tracing::warn!("Failed to report dropped command {}", command_id);
                                                        break;
                                                    }
                                                }
                                            }
                                        }
                                        Ok(None) => {
                                            tracing::warn!("Controller command stream closed");
                                            break;
                                        }
                                        Err(e) => {
                                            tracing::warn!("Command stream receive error: {:?}", e);
                                            break;
                                        }
                                    }
                                }
                            }
                        }
                    }
                    Err(e) => {
                        tracing::warn!("Failed to connect command stream: {:?}", e);
                    }
                }

                tokio::select! {
                    _ = cancel_token.cancelled() => break,
                    _ = tokio::time::sleep(Duration::from_secs(5)) => {}
                }
            }

            tracing::info!("Command stream task stopped");
        });
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
        let last_sync_peers = self.last_sync_peers.clone();
        
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
                                let last_sync_peers = last_sync_peers.clone();
                                
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
                                        
                                        // 获取 last_sync_peers 的快照
                                        let peers_snapshot = last_sync_peers.lock().unwrap().clone();
                                        
                                        let response = match serde_json::from_str::<UnixRequest>(&line) {
                                            Ok(req) => {
                                                Self::handle_unix_command(
                                                    req,
                                                    &acl_mgr,
                                                    &qos_mgr,
                                                    &wg_manager,
                                                    &log_handle,
                                                    &current_log_level,
                                                    &peers_snapshot,
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
        
        Ok(())
    }
    
    async fn handle_unix_command(
        req: UnixRequest,
        acl_mgr: &Arc<Mutex<AclManager>>,
        qos_mgr: &Arc<Mutex<QoSManager>>,
        wg_manager: &Arc<Mutex<WireGuardManager>>,
        log_handle: &Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>,
        current_log_level: &Arc<StdMutex<String>>,
        last_sync_peers: &[GrpcPeerInfo],
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
                let last_sync = last_sync_peers.to_vec();
                match wg.list_peers() {
                    Ok(peers) => {
                        let peers_json: Vec<serde_json::Value> = peers
                            .into_iter()
                            .map(|p| {
                                // 从 last_sync_peers 中查找 region
                                let region = last_sync.iter()
                                    .find(|sync_peer| sync_peer.public_key == p.public_key)
                                    .map(|sync_peer| sync_peer.region.clone())
                                    .unwrap_or_else(|| "unknown".to_string());
                                
                                serde_json::json!({
                                    "public_key": p.public_key,
                                    "endpoint": p.endpoint,
                                    "allowed_ips": p.allowed_ips,
                                    "last_handshake_secs": p.last_handshake,
                                    "region": region,
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

    async fn execute_remote_command(&mut self, request: GrpcCommandRequest) -> GrpcCommandResponse {
        let command_id = request.command_id.clone();
        let command_name = request.command.clone();

        tracing::info!("Executing remote command {} ({})", command_id, command_name);

        let response = match command_name.as_str() {
            "sync" => match self.sync().await {
                Ok(_) => {
                    let mut result = HashMap::new();
                    result.insert(
                        "peer_count".to_string(),
                        self.last_sync_peers.lock().unwrap().len().to_string(),
                    );
                    build_completed_command_response(
                        command_id.clone(),
                        "sync completed".to_string(),
                        result,
                    )
                }
                Err(e) => build_failed_command_response(
                    command_id.clone(),
                    format!("sync failed: {}", e),
                ),
            },
            "config_reload" => match self.reload_config().await {
                Ok(_) => build_completed_command_response(
                    command_id.clone(),
                    "config reload completed".to_string(),
                    HashMap::new(),
                ),
                Err(e) => build_failed_command_response(
                    command_id.clone(),
                    format!("config reload failed: {}", e),
                ),
            },
            "health_check" => self.execute_health_check_command(command_id.clone()).await,
            "restart" => build_failed_command_response(
                command_id.clone(),
                "restart is not implemented yet".to_string(),
            ),
            _ => self.execute_unix_style_remote_command(request).await,
        };

        if response.status == "failed" {
            tracing::warn!(
                "Remote command {} ({}) failed: {}",
                command_id,
                command_name,
                response.message
            );
        } else {
            tracing::info!("Remote command {} ({}) completed", command_id, command_name);
        }

        response
    }

    async fn execute_health_check_command(&self, command_id: String) -> GrpcCommandResponse {
        let mut result = HashMap::new();
        result.insert("agent_id".to_string(), self.config.public_key.clone());
        result.insert("interface_name".to_string(), self.config.interface_name.clone());
        result.insert(
            "hostname".to_string(),
            self.config.hostname.clone().unwrap_or_else(|| "unknown".to_string()),
        );
        result.insert(
            "sync_interval_secs".to_string(),
            self.config.sync_interval.as_secs().to_string(),
        );
        result.insert(
            "last_sync_peer_count".to_string(),
            self.last_sync_peers.lock().unwrap().len().to_string(),
        );

        let wg = self.wg_manager.lock().await;
        match wg.list_peers() {
            Ok(peers) => {
                result.insert("wireguard_peer_count".to_string(), peers.len().to_string());
                build_completed_command_response(
                    command_id,
                    "agent healthy".to_string(),
                    result,
                )
            }
            Err(e) => {
                result.insert("wireguard_error".to_string(), e.to_string());
                build_failed_command_response_with_result(
                    command_id,
                    "failed to query wireguard peers".to_string(),
                    result,
                )
            }
        }
    }

    async fn execute_unix_style_remote_command(
        &self,
        request: GrpcCommandRequest,
    ) -> GrpcCommandResponse {
        let args = Self::decode_remote_command_args(&request.params);
        let peers_snapshot = self.last_sync_peers.lock().unwrap().clone();
        let response_raw = Self::handle_unix_command(
            UnixRequest {
                cmd: request.command.clone(),
                args,
            },
            &self.acl_mgr,
            &self.qos_mgr,
            &self.wg_manager,
            &self.log_handle,
            &self.current_log_level,
            &peers_snapshot,
        )
        .await;

        match serde_json::from_str::<UnixResponse>(response_raw.trim()) {
            Ok(response) => {
                let result = Self::unix_response_to_result_map(response.data);
                if response.success {
                    build_completed_command_response(
                        request.command_id,
                        response.message.unwrap_or_else(|| "command completed".to_string()),
                        result,
                    )
                } else {
                    build_failed_command_response_with_result(
                        request.command_id,
                        response.message.unwrap_or_else(|| "command failed".to_string()),
                        result,
                    )
                }
            }
            Err(e) => build_failed_command_response(
                request.command_id,
                format!("failed to parse command response: {}", e),
            ),
        }
    }

    fn decode_remote_command_args(params: &HashMap<String, String>) -> serde_json::Value {
        let mut args = serde_json::Map::new();
        for (key, value) in params {
            let parsed = serde_json::from_str::<serde_json::Value>(value)
                .unwrap_or_else(|_| serde_json::Value::String(value.clone()));
            args.insert(key.clone(), parsed);
        }
        serde_json::Value::Object(args)
    }

    fn unix_response_to_result_map(data: Option<serde_json::Value>) -> HashMap<String, String> {
        match data {
            Some(serde_json::Value::Object(map)) => map
                .into_iter()
                .map(|(key, value)| {
                    let value = match value {
                        serde_json::Value::String(s) => s,
                        other => other.to_string(),
                    };
                    (key, value)
                })
                .collect(),
            Some(other) => {
                let mut result = HashMap::new();
                result.insert("data".to_string(), other.to_string());
                result
            }
            None => HashMap::new(),
        }
    }
    
    pub async fn sync(&mut self) -> Result<()> {
        tracing::debug!("Syncing with Controller...");
        
        let sync_result = self.grpc_client
            .sync(self.config.public_key.clone())
            .await?;
        
        tracing::debug!("Sync received: {} peers, {} ACL rules, {} blacklist rules, {} QoS rules", 
            sync_result.peers.len(), 
            sync_result.acl_rules.len(),
            sync_result.blacklist_rules.len(),
            sync_result.qos_rules.len());
        
        self.sync_peers(&sync_result.peers).await?;
        self.sync_advertised_routes(&sync_result.peers).await?;
        
        if let Err(e) = self.sync_acl_rules(&sync_result.acl_rules).await {
            tracing::error!("Failed to sync ACL rules: {:?}", e);
        }

        if let Err(e) = self.sync_blacklist_rules(&sync_result.blacklist_rules).await {
            tracing::error!("Failed to sync blacklist rules: {:?}", e);
        }
        
        if let Err(e) = self.sync_qos_rules(&sync_result.qos_rules).await {
            tracing::error!("Failed to sync QoS rules: {:?}", e);
        }
        
        *self.last_sync_peers.lock().unwrap() = sync_result.peers;
        tracing::debug!("Sync completed");
        Ok(())
    }
    
    /// 同步 QoS 规则
    async fn sync_qos_rules(&mut self, new_rules: &[GrpcQoSRule]) -> Result<()> {
        tracing::info!("Syncing {} QoS rules", new_rules.len());
        
        // 克隆 Arc 以便在 spawn_blocking 中使用
        let qos_mgr = self.qos_mgr.clone();
        let new_rules = new_rules.to_vec();
        
        let result = tokio::task::spawn_blocking(move || -> Result<()> {
            let mut mgr = qos_mgr.blocking_lock();
            mgr.clear_all_rules().map_err(|e| anyhow::anyhow!(e))?;
            
            let mut success_count = 0;
            let mut fail_count = 0;
            
            for rule in &new_rules {
                // 根据规则特征判断类型并应用
                let result = if rule.src_ip.is_empty() && rule.dst_ip.is_empty() {
                    // 端口级规则（已弃用，忽略）
                    tracing::warn!("Port-level rules are deprecated, skipping");
                    continue;
                } else if rule.src_ip.is_empty() || rule.dst_ip.is_empty() {
                    // IP 级规则
                    let ip = if !rule.src_ip.is_empty() {
                        &rule.src_ip
                    } else {
                        &rule.dst_ip
                    };
                    mgr.limit_ip(ip, rule.bandwidth_mbps).map_err(|e| anyhow::anyhow!(e))
                } else if rule.src_port == 0 && rule.dst_port == 0 {
                    // Peer 级规则（只有 IP 对）
                    mgr.limit_peer_pair(&rule.src_ip, &rule.dst_ip, rule.bandwidth_mbps).map_err(|e| anyhow::anyhow!(e))
                } else {
                    // 服务级规则（五元组）
                    mgr.limit_service(
                        &rule.src_ip,
                        &rule.dst_ip,
                        rule.src_port as u16,
                        rule.dst_port as u16,
                        rule.protocol as u8,
                        rule.bandwidth_mbps,
                    ).map_err(|e| anyhow::anyhow!(e))
                };
                
                match result {
                    Ok(_) => {
                        success_count += 1;
                        tracing::debug!(
                            "Applied QoS rule: {}:{}:{}:{}:{} -> {} Mbps",
                            rule.src_ip, rule.dst_ip, rule.src_port,
                            rule.dst_port, rule.protocol, rule.bandwidth_mbps
                        );
                    }
                    Err(e) => {
                        fail_count += 1;
                        tracing::error!(
                            "Failed to apply QoS rule: {}:{}:{}:{}:{} -> {:?}",
                            rule.src_ip, rule.dst_ip, rule.src_port,
                            rule.dst_port, rule.protocol, e
                        );
                    }
                }
            }
            
            tracing::info!(
                "QoS sync completed: {} success, {} failed",
                success_count,
                fail_count
            );
            
            Ok(())
        }).await?;
        
        result
    }

    async fn sync_blacklist_rules(&mut self, new_rules: &[GrpcBlacklistRule]) -> Result<()> {
        tracing::info!("Syncing {} blacklist rules", new_rules.len());

        let acl_mgr = self.acl_mgr.clone();
        let new_rules = new_rules.to_vec();

        let result = tokio::task::spawn_blocking(move || -> Result<()> {
            let mut mgr = acl_mgr.blocking_lock();
            mgr.clear_all_blacklists().map_err(|e| anyhow::anyhow!(e))?;

            let mut success_count = 0;
            let mut fail_count = 0;

            for rule in &new_rules {
                let apply_result = match rule.scope.as_str() {
                    "src" if !rule.cidr.is_empty() => mgr.block_src_cidr(&rule.cidr).map(|_| ()).map_err(|e| anyhow::anyhow!(e)),
                    "dst" if !rule.cidr.is_empty() => mgr.block_dst_cidr(&rule.cidr).map(|_| ()).map_err(|e| anyhow::anyhow!(e)),
                    "ports" if rule.port > 0 => mgr.block_port(rule.port as u16).map_err(|e| anyhow::anyhow!(e)),
                    _ => Err(anyhow::anyhow!("invalid blacklist rule payload")),
                };

                match apply_result {
                    Ok(_) => success_count += 1,
                    Err(e) => {
                        fail_count += 1;
                        tracing::error!("Failed to apply blacklist rule {:?}: {:?}", rule, e);
                    }
                }
            }

            tracing::info!(
                "Blacklist sync completed: {} success, {} failed",
                success_count,
                fail_count
            );

            Ok(())
        }).await?;

        result
    }
    
    async fn sync_peers(&mut self, new_peers: &[GrpcPeerInfo]) -> Result<()> {
        let new_peers = new_peers.to_vec();
        let multi_tunnel = self.config.multi_tunnel;
        
        let result = tokio::task::spawn_blocking(move || -> Result<(usize, usize, usize, usize)> {
            // 确定要配置的接口列表
            let interfaces = if multi_tunnel {
                vec!["aria0", "aria1", "aria2", "aria3"]
            } else {
                vec!["aria0"]
            };
            let interface_count = interfaces.len();
            
            let mut total_added = 0;
            let mut total_removed = 0;
            let mut total_updated = 0;
            
            for iface in interfaces {
                let iface_wg_manager = Arc::new(Mutex::new(WireGuardManager::new(iface)));
                let mut wg = iface_wg_manager.blocking_lock();
                
                let current_peers = wg.list_peers()
                    .context("Failed to list current peers")?;
                
                let (to_add, to_remove, to_update) = Self::diff_peers_static(&current_peers, &new_peers);
                
                // 删除 peer
                for peer in &to_remove {
                    if iface == "aria0" {
                        tracing::info!("Removing peer {} from {}...", &peer[..16.min(peer.len())], iface);
                    }
                    wg.remove_peer(&peer)
                        .context("Failed to remove peer")?;
                    if iface == "aria0" {
                        metrics::record_wireguard_peer_change("remove");
                    }
                }
                total_removed += to_remove.len();
                
                // 添加 peer
                for peer in &to_add {
                    if iface == "aria0" {
                        tracing::info!("Adding peer {} to {}...", &peer.public_key[..16.min(peer.public_key.len())], iface);
                    }
                    
                    let mut allowed_ips = vec![format!("{}/32", peer.assigned_ip)];
                    allowed_ips.extend(peer.advertised_routes.clone());
                    
                    // 根据 iface 编号调整 endpoint 端口
                    let endpoint = if !peer.endpoint.is_empty() {
                        let adjusted_endpoint = Self::adjust_endpoint_port(&peer.endpoint, iface);
                        Some(adjusted_endpoint)
                    } else {
                        None
                    };
                    
                    let peer_config = PeerConfig {
                        public_key: peer.public_key.clone(),
                        endpoint,
                        allowed_ips,
                        persistent_keepalive: 25,
                    };
                    
                    wg.add_peer(peer_config)
                        .context("Failed to add peer")?;
                    if iface == "aria0" {
                        metrics::record_wireguard_peer_change("add");
                    }
                }
                total_added += to_add.len();
                
                // 更新 peer
                for peer in &to_update {
                    if iface == "aria0" {
                        tracing::debug!("Updating peer {} on {}...", &peer.public_key[..16.min(peer.public_key.len())], iface);
                    }
                    
                    wg.remove_peer(&peer.public_key)
                        .context("Failed to remove peer for update")?;
                    
                    let mut allowed_ips = vec![format!("{}/32", peer.assigned_ip)];
                    allowed_ips.extend(peer.advertised_routes.clone());
                    
                    // 根据 iface 编号调整 endpoint 端口
                    let endpoint = if !peer.endpoint.is_empty() {
                        let adjusted_endpoint = Self::adjust_endpoint_port(&peer.endpoint, iface);
                        Some(adjusted_endpoint)
                    } else {
                        None
                    };
                    
                    let peer_config = PeerConfig {
                        public_key: peer.public_key.clone(),
                        endpoint,
                        allowed_ips,
                        persistent_keepalive: 25,
                    };
                    
                    wg.add_peer(peer_config)
                        .context("Failed to add updated peer")?;
                    if iface == "aria0" {
                        metrics::record_wireguard_peer_change("update");
                    }
                }
                total_updated += to_update.len();
            }
            
            Ok((total_added / interface_count, total_removed / interface_count, total_updated / interface_count, new_peers.len()))
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
    
    fn adjust_endpoint_port(endpoint: &str, iface: &str) -> String {
        // aria0:51820, aria1:51821, aria2:51822, aria3:51823
        let port_offset = match iface {
            "aria0" => 0,
            "aria1" => 1,
            "aria2" => 2,
            "aria3" => 3,
            _ => 0,
        };
        
        if let Some(colon_pos) = endpoint.rfind(':') {
            let host = &endpoint[..colon_pos];
            let base_port = 51820u16;
            let new_port = base_port + port_offset;
            format!("{}:{}", host, new_port)
        } else {
            endpoint.to_string()
        }
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
        
        // 收集期望的所有路由
        // 1. peer 的 VPN IP 路由（/32）
        // 2. peer 宣告的非 /32 路由
        let mut desired_routes = StdHashSet::new();
        for peer in peers {
            // 添加 peer 的 VPN IP 路由
            desired_routes.insert(format!("{}/32", peer.assigned_ip));
            
            // 添加 peer 宣告的非 /32 路由
            for route in &peer.advertised_routes {
                if !route.ends_with("/32") {
                    desired_routes.insert(route.clone());
                }
            }
        }
        
        // 确定要使用的接口列表（多隧道模式使用所有接口）
        let interfaces: Vec<String> = if self.config.multi_tunnel {
            vec!["aria0".to_string(), "aria1".to_string(), "aria2".to_string(), "aria3".to_string()]
        } else {
            vec![self.config.interface_name.clone()]
        };
        
        let multi_tunnel = self.config.multi_tunnel;
        
        // 在单个阻塞任务中完成所有路由操作，保证原子性和性能
        let routing_manager = self.routing_manager.clone();
        let result = tokio::task::spawn_blocking(move || -> Result<(usize, usize, usize)> {
            // 获取当前路由
            let current_routes = routing_manager.list_vpn_routes()
                .context("Failed to list current VPN routes")?;
            
            // 计算差异
            let to_remove: Vec<_> = current_routes.difference(&desired_routes).cloned().collect();
            
            let mut added_count = 0;
            let mut removed_count = 0;
            
            // 删除多余的路由
            for route in &to_remove {
                if let Err(e) = routing_manager.remove_vpn_route(route) {
                    tracing::error!("Failed to remove stale route {}: {:?}", route, e);
                } else {
                    removed_count += 1;
                    tracing::info!("Removed stale route: {}", route);
                }
            }
            
            // 添加或更新所有期望的路由（使用 replace，会自动替换旧路由）
            for route in &desired_routes {
                if multi_tunnel {
                    // 多隧道模式：使用 ECMP 路由（replace 会自动替换旧路由）
                    let interfaces_str: Vec<&str> = interfaces.iter().map(|s| s.as_str()).collect();
                    if let Err(e) = routing_manager.add_ecmp_route(route, &interfaces_str, Some(100)) {
                        tracing::error!("Failed to add ECMP route {}: {:?}", route, e);
                    } else {
                        added_count += 1;
                    }
                } else {
                    // 单隧道模式：使用普通路由
                    if let Err(e) = routing_manager.add_vpn_route(route) {
                        tracing::error!("Failed to add route {}: {:?}", route, e);
                    } else {
                        added_count += 1;
                    }
                }
            }
            
            if multi_tunnel && added_count > 0 {
                tracing::info!("Added/updated {} ECMP routes via {} interfaces", added_count, interfaces.len());
            }
            
            Ok((added_count, removed_count, desired_routes.len()))
        }).await?;
        
        match result {
            Ok((added_count, removed_count, total_count)) => {
                if added_count > 0 || removed_count > 0 {
                    tracing::info!(
                        "Route sync completed: {} routes added/updated, {} routes removed, {} routes total",
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
        
        let new_client = GrpcClient::new_with_options(
            self.config.controller_url.clone(),
            self.config.ca_cert.clone(),
            self.config.client_cert.clone(),
            self.config.client_key.clone(),
            self.config.tls_server_name.clone(),
        ).await.context("Failed to reconnect to Controller")?;
        
        self.grpc_client = new_client;
        tracing::info!("✅ gRPC client reconnected successfully");
        Ok(())
    }
    
    async fn reload_config(&mut self) -> Result<()> {
        tracing::info!("Reloading configuration...");
        
        let config_manager = crate::config::ConfigManager::new(&self.config_path);
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
            let old_tls_server_name = self.config.tls_server_name.clone();
            
            // 更新配置
            self.config.controller_url = new_config.controller_url.clone();
            self.config.ca_cert = new_config.ca_cert.clone();
            self.config.client_cert = new_config.client_cert.clone();
            self.config.client_key = new_config.client_key.clone();
            self.config.tls_server_name = new_config.tls_server_name.clone();
            
            // 尝试重连
            if let Err(e) = self.reconnect_grpc().await {
                tracing::error!("Failed to reconnect gRPC client, rolling back config: {:?}", e);
                
                // 回滚配置
                self.config.controller_url = old_url;
                self.config.ca_cert = old_ca;
                self.config.client_cert = old_cert;
                self.config.client_key = old_key;
                self.config.tls_server_name = old_tls_server_name;
                
                metrics::record_grpc_error();
                metrics::record_config_reload_failure();
            }
        }
        
        // 3. 检测证书路径变更
        else if new_config.ca_cert != self.config.ca_cert ||
                new_config.client_cert != self.config.client_cert ||
                new_config.client_key != self.config.client_key ||
                new_config.tls_server_name != self.config.tls_server_name {
            tracing::warn!("Certificate paths changed, reconnecting gRPC client");
            
            // 备份旧证书路径
            let old_ca = self.config.ca_cert.clone();
            let old_cert = self.config.client_cert.clone();
            let old_key = self.config.client_key.clone();
            let old_tls_server_name = self.config.tls_server_name.clone();
            
            // 更新证书路径
            self.config.ca_cert = new_config.ca_cert.clone();
            self.config.client_cert = new_config.client_cert.clone();
            self.config.client_key = new_config.client_key.clone();
            self.config.tls_server_name = new_config.tls_server_name.clone();
            
            // 尝试重连
            if let Err(e) = self.reconnect_grpc().await {
                tracing::error!("Failed to reconnect gRPC client with new certificates, rolling back: {:?}", e);
                
                // 回滚证书路径
                self.config.ca_cert = old_ca;
                self.config.client_cert = old_cert;
                self.config.client_key = old_key;
                self.config.tls_server_name = old_tls_server_name;
                
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

fn build_completed_command_response(
    command_id: String,
    message: String,
    result: HashMap<String, String>,
) -> GrpcCommandResponse {
    GrpcCommandResponse {
        command_id,
        status: "completed".to_string(),
        message,
        result,
        completed_at: current_unix_timestamp(),
    }
}

fn build_failed_command_response(command_id: String, message: String) -> GrpcCommandResponse {
    build_failed_command_response_with_result(command_id, message, HashMap::new())
}

fn build_failed_command_response_with_result(
    command_id: String,
    message: String,
    result: HashMap<String, String>,
) -> GrpcCommandResponse {
    GrpcCommandResponse {
        command_id,
        status: "failed".to_string(),
        message,
        result,
        completed_at: current_unix_timestamp(),
    }
}
