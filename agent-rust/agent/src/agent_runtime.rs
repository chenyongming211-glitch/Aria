use anyhow::{Context, Result};
use aya::{
    include_bytes_aligned,
    programs::{
        tc::{self, NlOptions, TcAttachOptions},
        SchedClassifier, TcAttachType, Xdp, XdpFlags,
    },
    EbpfLoader,
};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::{Arc, Mutex as StdMutex};
use std::time::Duration;
use tokio::signal;
use tokio::signal::unix::{signal, SignalKind};
use tokio::sync::{mpsc, oneshot, Mutex, Notify};
use tokio_util::sync::CancellationToken;
use tracing_subscriber::{reload, EnvFilter, Registry};

use crate::acl_qos_manager::{AclQosManager, AclQosSnapshot, AclRuleSpec, IPGroupSpec, QosRuleSpec};
use crate::acl_qos_state::{
    requested_directions, ACTION_DROP, DIRECTION_EGRESS, DIRECTION_INGRESS,
};
use crate::certificate_client;
use crate::config::{AgentConfig, ConfigManager};
use crate::grpc_client::{acl_policy_from_sync_rule, qos_policy_from_sync_rule};
use crate::grpc_client::{
    AclRule, BlacklistRule as GrpcBlacklistRule, GrpcClient, GrpcCommandRequest,
    GrpcCommandResponse, IPGroup as GrpcIPGroup, PeerInfo as GrpcPeerInfo,
    QoSRule as GrpcQoSRule,
};
use crate::identity::IdentityManager;
use crate::metrics;
use crate::routing::RoutingManager;
use crate::runtime_credential::RuntimeCredentialStore;
use crate::wireguard::{PeerConfig, WireGuardManager};

const BPF_FS_PATH: &str = "/sys/fs/bpf/aria";
const BPF_PIN_FALLBACK_PATH: &str = "/sys/fs/bpf";
const CERTIFICATE_RENEW_CHECK_INTERVAL: Duration = Duration::from_secs(30 * 60);
const TC_PRIORITY_ACL_EGRESS: u16 = 100;
const TC_PRIORITY_QOS: u16 = 200;
const BLACKLIST_ACL_PRIORITY: u16 = 0;

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

pub struct AgentRuntime {
    config: AgentConfig,
    config_path: String,

    acl_qos_mgr: Arc<Mutex<AclQosManager>>,
    grpc_client: GrpcClient,
    wg_manager: Arc<Mutex<WireGuardManager>>,
    wg_managers: HashMap<String, Arc<Mutex<WireGuardManager>>>,
    routing_manager: RoutingManager,

    unix_socket_path: String,

    last_sync_peers: Arc<Mutex<Vec<GrpcPeerInfo>>>,

    cancel_token: CancellationToken,
    sync_now: Arc<Notify>,
    runtime_credential: RuntimeCredentialStore,
    log_handle: Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>,
    current_log_level: Arc<StdMutex<String>>,
}

impl AgentRuntime {
    pub async fn new(
        config: AgentConfig,
        config_path: String,
        _interface: &str,
        log_handle: Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>,
    ) -> Result<Self> {
        tracing::info!("Creating AgentRuntime...");

        // Create ALL WireGuard interfaces BEFORE loading eBPF
        // (eBPF XDP/TC attaches to the interfaces, so they must exist first)
        let interfaces = if config.multi_tunnel {
            let base = config
                .interface_name
                .trim_end_matches(|c: char| c.is_numeric());
            vec![
                config.interface_name.clone(),
                format!("{}1", base),
                format!("{}2", base),
                format!("{}3", base),
            ]
        } else {
            vec![config.interface_name.clone()]
        };

        let mut wg_managers = HashMap::new();
        for (i, iface_name) in interfaces.iter().enumerate() {
            let port = config.listen_port + i as u16;
            let mut wg = WireGuardManager::new(iface_name);
            wg.ensure_interface(
                config.private_key.clone(),
                config.address.clone(),
                port,
                config.mtu,
            )
            .context(format!(
                "Failed to create WireGuard interface {}",
                iface_name
            ))?;
            tracing::info!(
                "✅ WireGuard interface {} created on port {}",
                iface_name,
                port
            );
            wg_managers.insert(iface_name.clone(), Arc::new(Mutex::new(wg)));
        }

        let wg_manager = wg_managers
            .get(&config.interface_name)
            .expect("main interface must be in wg_managers")
            .clone();

        // Load eBPF programs and attach to ALL interfaces
        let (acl_qos_mgr, _identity_mgr) = Self::load_ebpf_programs_multi(&interfaces)?;
        tracing::info!(
            "✅ eBPF programs loaded and attached to {} interfaces",
            interfaces.len()
        );

        let grpc_client = GrpcClient::new_with_options(
            config.controller_url.clone(),
            config.ca_cert.clone(),
            config.client_cert.clone(),
            config.client_key.clone(),
            config.tls_server_name.clone(),
        )
        .await
        .context("Failed to connect to Controller")?;
        tracing::info!("✅ gRPC client connected");

        let routing_manager = RoutingManager::new(&config.interface_name);
        tracing::info!("✅ Routing manager created");

        let cancel_token = CancellationToken::new();
        let sync_now = Arc::new(Notify::new());
        let current_log_level = Arc::new(StdMutex::new("info".to_string()));
        let last_sync_peers = Arc::new(Mutex::new(Vec::new()));
        let runtime_credential = RuntimeCredentialStore::new(config.current_credential.clone());

        Ok(Self {
            config,
            config_path,
            acl_qos_mgr,
            grpc_client,
            wg_manager,
            wg_managers,
            routing_manager,
            unix_socket_path: "/run/aria-agent.sock".to_string(),
            last_sync_peers,
            cancel_token,
            sync_now,
            runtime_credential,
            log_handle,
            current_log_level,
        })
    }
    fn set_sync_observation(&mut self, status: &str, message: String) {
        self.config.last_sync_status = Some(status.to_string());
        self.config.last_sync_message = Some(message);
        self.config.last_sync_at = Some(current_unix_timestamp());
    }

    fn get_active_interfaces(&self) -> Vec<String> {
        if self.config.multi_tunnel {
            let base = self
                .config
                .interface_name
                .trim_end_matches(|c: char| c.is_numeric());
            vec![
                self.config.interface_name.clone(),
                format!("{}1", base),
                format!("{}2", base),
                format!("{}3", base),
            ]
        } else {
            vec![self.config.interface_name.clone()]
        }
    }

    fn persist_runtime_state(&self) -> Result<()> {
        let config_manager = ConfigManager::new(&self.config_path);
        config_manager.save_state(&self.config.to_state())?;
        Ok(())
    }

    #[allow(dead_code)]
    fn load_ebpf_programs(
        interface: &str,
    ) -> Result<(Arc<Mutex<AclQosManager>>, Arc<StdMutex<IdentityManager>>)> {
        Self::cleanup_pinned_acl_qos_maps();
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
        let program_ref = acl_ebpf
            .program_mut("xdp_ingress_acl")
            .context("XDP program not found in eBPF object")?;
        tracing::info!("Step 4.2: Program reference obtained");

        tracing::info!("Step 4.3: Converting to XDP type...");
        let program: &mut Xdp = program_ref
            .try_into()
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

        tracing::info!("Step 17: Creating QoS EbpfLoader...");
        let mut qos_ebpf = EbpfLoader::new().load(qos_bytes)?;
        tracing::info!("Step 18: QoS eBPF loaded into memory");

        tracing::info!("Step 19: Loading TC ACL/QoS programs...");
        {
            let program: &mut SchedClassifier = acl_ebpf
                .program_mut("tc_egress_acl")
                .context("TC egress ACL program not found")?
                .try_into()?;
            program.load()?;
        }
        {
            let program: &mut SchedClassifier = qos_ebpf
                .program_mut("tc_ingress_qos")
                .context("TC ingress program not found")?
                .try_into()?;
            program.load()?;
        }
        {
            let program: &mut SchedClassifier = qos_ebpf
                .program_mut("tc_egress_qos")
                .context("TC egress program not found")?
                .try_into()?;
            program.load()?;
        }
        tracing::info!("Step 22: TC programs loaded");

        tracing::info!("Step 22.5: Creating IdentityManager...");
        let identity_mgr = IdentityManager::new(&mut acl_ebpf, Some(&mut qos_ebpf))?;
        tracing::info!("Step 22.6: IdentityManager created");

        let identity_mgr = Arc::new(StdMutex::new(identity_mgr));
        tracing::info!("Step 22.7: IdentityManager wrapped in Arc");

        tracing::info!("Step 23: Preparing clsact qdisc for {}...", interface);
        // 先删除旧的 clsact qdisc（忽略错误）
        let _ = std::process::Command::new("tc")
            .args(&["qdisc", "del", "dev", interface, "clsact"])
            .output();
        tracing::info!("Step 23.5: Old clsact removed (if existed)");

        // 添加新的 clsact qdisc
        tc::qdisc_add_clsact(interface)?;
        tracing::info!("Step 24: clsact qdisc added");

        tracing::info!(
            "Step 25: Attaching TC programs to {} ingress/egress...",
            interface
        );
        {
            let program: &mut SchedClassifier = acl_ebpf
                .program_mut("tc_egress_acl")
                .context("TC egress ACL program not found")?
                .try_into()?;
            Self::attach_tc_program(
                program,
                interface,
                TcAttachType::Egress,
                TC_PRIORITY_ACL_EGRESS,
            )?;
        }
        {
            let program: &mut SchedClassifier = qos_ebpf
                .program_mut("tc_ingress_qos")
                .context("TC ingress program not found")?
                .try_into()?;
            Self::attach_tc_program(program, interface, TcAttachType::Ingress, TC_PRIORITY_QOS)?;
        }
        {
            let program: &mut SchedClassifier = qos_ebpf
                .program_mut("tc_egress_qos")
                .context("TC egress program not found")?
                .try_into()?;
            Self::attach_tc_program(program, interface, TcAttachType::Egress, TC_PRIORITY_QOS)?;
        }
        tracing::info!("Step 26: TC programs attached");

        let acl_qos_mgr = AclQosManager::new(
            acl_ebpf,
            qos_ebpf,
            identity_mgr.clone(),
            vec![interface.to_string()],
        )?;
        Self::verify_ebpf_attachments(&[interface.to_string()])?;
        let acl_qos_mgr = Arc::new(Mutex::new(acl_qos_mgr));

        Ok((acl_qos_mgr, identity_mgr))
    }

    /// Load eBPF programs and attach XDP/TC to multiple interfaces (multi-tunnel mode)
    fn load_ebpf_programs_multi(
        interfaces: &[String],
    ) -> Result<(Arc<Mutex<AclQosManager>>, Arc<StdMutex<IdentityManager>>)> {
        Self::cleanup_pinned_acl_qos_maps();
        tracing::info!(
            "Loading eBPF programs for {} interfaces: {:?}",
            interfaces.len(),
            interfaces
        );

        // Load ACL eBPF
        let acl_bytes = include_bytes_aligned!(concat!(env!("OUT_DIR"), "/acl"));
        let mut acl_ebpf = EbpfLoader::new()
            .load(acl_bytes)
            .context("Failed to load ACL eBPF bytecode")?;

        let program_ref = acl_ebpf
            .program_mut("xdp_ingress_acl")
            .context("XDP program not found in eBPF object")?;
        let xdp_program: &mut Xdp = program_ref.try_into()?;
        xdp_program.load()?;

        // Attach XDP to ALL interfaces
        for iface in interfaces {
            tracing::info!("Attaching XDP program to {}...", iface);
            xdp_program
                .attach(iface, XdpFlags::default())
                .context(format!("Failed to attach XDP to {}", iface))?;
            tracing::info!("✅ XDP attached to {}", iface);
        }

        // Load QoS eBPF
        let qos_bytes = include_bytes_aligned!(concat!(env!("OUT_DIR"), "/qos"));
        let mut qos_ebpf = EbpfLoader::new().load(qos_bytes)?;
        {
            let tc_program: &mut SchedClassifier = acl_ebpf
                .program_mut("tc_egress_acl")
                .context("TC egress ACL program not found")?
                .try_into()?;
            tc_program.load()?;
        }
        {
            let tc_program: &mut SchedClassifier = qos_ebpf
                .program_mut("tc_ingress_qos")
                .context("TC ingress program not found")?
                .try_into()?;
            tc_program.load()?;
        }
        {
            let tc_program: &mut SchedClassifier = qos_ebpf
                .program_mut("tc_egress_qos")
                .context("TC egress program not found")?
                .try_into()?;
            tc_program.load()?;
        }

        let identity_mgr = IdentityManager::new(&mut acl_ebpf, Some(&mut qos_ebpf))?;
        let identity_mgr = Arc::new(StdMutex::new(identity_mgr));

        // Attach TC to ALL interfaces
        for iface in interfaces {
            let _ = std::process::Command::new("tc")
                .args(&["qdisc", "del", "dev", iface, "clsact"])
                .output();
            tc::qdisc_add_clsact(iface)?;
            {
                let tc_program: &mut SchedClassifier = acl_ebpf
                    .program_mut("tc_egress_acl")
                    .context("TC egress ACL program not found")?
                    .try_into()?;
                Self::attach_tc_program(
                    tc_program,
                    iface,
                    TcAttachType::Egress,
                    TC_PRIORITY_ACL_EGRESS,
                )?;
            }
            {
                let tc_program: &mut SchedClassifier = qos_ebpf
                    .program_mut("tc_ingress_qos")
                    .context("TC ingress program not found")?
                    .try_into()?;
                Self::attach_tc_program(tc_program, iface, TcAttachType::Ingress, TC_PRIORITY_QOS)?;
            }
            {
                let tc_program: &mut SchedClassifier = qos_ebpf
                    .program_mut("tc_egress_qos")
                    .context("TC egress program not found")?
                    .try_into()?;
                Self::attach_tc_program(tc_program, iface, TcAttachType::Egress, TC_PRIORITY_QOS)?;
            }
            tracing::info!("✅ TC ingress/egress attached to {}", iface);
        }

        let acl_qos_mgr = AclQosManager::new(
            acl_ebpf,
            qos_ebpf,
            identity_mgr.clone(),
            interfaces.to_vec(),
        )?;
        Self::verify_ebpf_attachments(interfaces)?;
        let acl_qos_mgr = Arc::new(Mutex::new(acl_qos_mgr));

        Ok((acl_qos_mgr, identity_mgr))
    }

    fn cleanup_pinned_acl_qos_maps() {
        let map_names = [
            "SRC_IPV4_ID_MAP",
            "DST_IPV4_ID_MAP",
            "SRC_IPV6_ID_MAP",
            "DST_IPV6_ID_MAP",
            "POLICY_TABLE",
            "PORT_BITMAP_POOL",
            "RULE_STATS",
            "QOS_CONFIG",
            "QOS_TOKEN_BUCKET",
            "QOS_STATS",
            "FIREWALL_CONFIG",
            "TAP_CONFIG_MAP",
        ];

        for base_path in [BPF_FS_PATH, BPF_PIN_FALLBACK_PATH] {
            for map_name in map_names {
                let pin_path = format!("{}/{}", base_path, map_name);
                if std::path::Path::new(&pin_path).exists() {
                    if let Err(error) = std::fs::remove_file(&pin_path) {
                        tracing::debug!(
                            "Failed to remove old pinned eBPF map {}: {}",
                            pin_path,
                            error
                        );
                    }
                }
            }
        }
    }

    fn attach_tc_program(
        program: &mut SchedClassifier,
        interface: &str,
        attach_type: TcAttachType,
        priority: u16,
    ) -> Result<()> {
        // Aya 0.13 uses TCX by default on Linux >= 6.6. That path is valid, but it is
        // not visible through `tc filter show`. Use netlink clsact attach so runtime
        // health checks and operator verification can prove the datapath is active.
        program.attach_with_options(
            interface,
            attach_type,
            TcAttachOptions::Netlink(NlOptions {
                priority,
                handle: 0,
            }),
        )?;
        Ok(())
    }

    fn verify_ebpf_attachments(interfaces: &[String]) -> Result<()> {
        for iface in interfaces {
            Self::verify_xdp_attachment(iface)
                .with_context(|| format!("XDP ACL attachment verification failed for {}", iface))?;
            Self::verify_tc_attachment(iface, "egress", "tc_egress_acl").with_context(|| {
                format!("TC egress ACL attachment verification failed for {}", iface)
            })?;
            Self::verify_tc_attachment(iface, "ingress", "tc_ingress_qos").with_context(|| {
                format!(
                    "TC ingress QoS attachment verification failed for {}",
                    iface
                )
            })?;
            Self::verify_tc_attachment(iface, "egress", "tc_egress_qos").with_context(|| {
                format!("TC egress QoS attachment verification failed for {}", iface)
            })?;
        }
        Ok(())
    }

    fn verify_xdp_attachment(interface: &str) -> Result<()> {
        let output = std::process::Command::new("bpftool")
            .arg("net")
            .output()
            .context("failed to execute bpftool net")?;
        if !output.status.success() {
            return Err(anyhow::anyhow!(
                "bpftool net failed: {}",
                String::from_utf8_lossy(&output.stderr)
            ));
        }
        let stdout = String::from_utf8_lossy(&output.stdout);
        if !stdout.contains(interface) {
            return Err(anyhow::anyhow!(
                "no XDP program is attached to {}; bpftool net output: {}",
                interface,
                stdout
            ));
        }

        let prog_output = std::process::Command::new("bpftool")
            .args(["prog", "show"])
            .output()
            .context("failed to execute bpftool prog show")?;
        if !prog_output.status.success() {
            return Err(anyhow::anyhow!(
                "bpftool prog show failed: {}",
                String::from_utf8_lossy(&prog_output.stderr)
            ));
        }
        let prog_stdout = String::from_utf8_lossy(&prog_output.stdout);
        if !prog_stdout.contains("xdp_ingress_acl") {
            return Err(anyhow::anyhow!(
                "xdp_ingress_acl is not loaded; bpftool prog show output: {}",
                prog_stdout
            ));
        }
        Ok(())
    }

    fn verify_tc_attachment(interface: &str, direction: &str, program_name: &str) -> Result<()> {
        let output = std::process::Command::new("tc")
            .args(["filter", "show", "dev", interface, direction])
            .output()
            .with_context(|| {
                format!(
                    "failed to execute tc filter show dev {} {}",
                    interface, direction
                )
            })?;
        if !output.status.success() {
            return Err(anyhow::anyhow!(
                "tc filter show dev {} {} failed: {}",
                interface,
                direction,
                String::from_utf8_lossy(&output.stderr)
            ));
        }
        let stdout = String::from_utf8_lossy(&output.stdout);
        if !stdout.contains(program_name) {
            return Err(anyhow::anyhow!(
                "{} is not attached to {} {}; tc output: {}",
                program_name,
                interface,
                direction,
                stdout
            ));
        }
        Ok(())
    }

    pub async fn start(&mut self) -> Result<()> {
        tracing::info!("Starting AgentRuntime...");

        // ========================================
        // 系统优化（P0 + P1）- 接口已由 new() 创建
        // ========================================
        tracing::info!("Applying system optimizations...");

        // 确定所有接口列表
        let all_interfaces = if self.config.multi_tunnel {
            let base = self
                .config
                .interface_name
                .trim_end_matches(|c: char| c.is_numeric())
                .to_string();
            vec![
                (self.config.interface_name.clone(), self.config.listen_port),
                (format!("{}1", base), self.config.listen_port + 1),
                (format!("{}2", base), self.config.listen_port + 2),
                (format!("{}3", base), self.config.listen_port + 3),
            ]
        } else {
            vec![(self.config.interface_name.clone(), self.config.listen_port)]
        };

        for (iface, port) in &all_interfaces {
            // 自动探测物理网卡名（排除回环和虚拟网卡）
            let mut phys_iface = "eth0".to_string();
            if let Ok(interfaces) = std::panic::catch_unwind(|| pnet::datalink::interfaces()) {
                let filtered: Vec<_> = interfaces
                    .into_iter()
                    .filter(|i| {
                        !i.is_loopback() && !i.name.starts_with("aria") && !i.name.starts_with("lo")
                    })
                    .collect();
                if !filtered.is_empty() {
                    phys_iface = filtered[0].name.clone();
                }
            }

            let optimizer = crate::system_optimization::SystemOptimizer::new(
                *port,
                phys_iface.clone(),
                iface.clone(),
            );

            tracing::debug!(
                "Applying system optimization to physical interface: {}",
                phys_iface
            );

            match optimizer.optimize(true) {
                Ok(result) => {
                    tracing::info!("✅ System optimizations applied for {}", iface);
                    if !result.warnings.is_empty() {
                        tracing::warn!("⚠️  Warnings for {}: {:?}", iface, result.warnings);
                    }
                }
                Err(e) => tracing::warn!("⚠️  Failed to optimize {}: {}", iface, e),
            }
        }

        // Step 3: 初始化路由
        self.routing_manager
            .init()
            .context("Failed to initialize routing manager")?;

        // Step 4: 首次同步
        self.sync().await?;
        tracing::info!("✅ Initial sync completed");

        self.start_unix_socket_server()?;
        let (remote_command_tx, remote_command_rx) = mpsc::channel(16);
        self.start_command_stream_task(remote_command_tx);

        self.run_main_loop(remote_command_rx).await
    }

    #[allow(dead_code)]
    async fn ensure_interface(&self) -> Result<()> {
        let mut wg = self.wg_manager.lock().await;

        // 使用 ensure_interface 确保主接口存在且配置正确
        tracing::info!(
            "Ensuring WireGuard interface {}",
            self.config.interface_name
        );
        wg.ensure_interface(
            self.config.private_key.clone(),
            self.config.address.clone(),
            self.config.listen_port,
            self.config.mtu,
        )
        .context("Failed to ensure main interface")?;

        // 如果启用了多隧道模式，创建额外的接口
        if self.config.multi_tunnel {
            let base_name = self.config.interface_name.clone();
            let base_name = base_name.trim_end_matches(|c: char| c.is_numeric());

            for i in 1..4 {
                let interface_name = format!("{}{}", base_name, i);
                let port = self.config.listen_port + i as u16;

                tracing::info!(
                    "Ensuring additional WireGuard interface {} on port {}",
                    interface_name,
                    port
                );

                let mut wg_extra = WireGuardManager::new(&interface_name);
                wg_extra
                    .ensure_interface(
                        self.config.private_key.clone(),
                        self.config.address.clone(),
                        port,
                        self.config.mtu,
                    )
                    .context(format!("Failed to ensure interface {}", interface_name))?;

                tracing::info!(
                    "✅ Additional interface {} ready on port {}",
                    interface_name,
                    port
                );
            }
        }

        Ok(())
    }

    async fn run_main_loop(
        &mut self,
        mut remote_command_rx: mpsc::Receiver<RemoteCommandEnvelope>,
    ) -> Result<()> {
        tracing::info!("Entering main loop");

        let mut sync_interval = tokio::time::interval(self.config.sync_interval);
        let mut metrics_timer = tokio::time::interval(Duration::from_secs(30));
        let mut certificate_renew_timer = tokio::time::interval(CERTIFICATE_RENEW_CHECK_INTERVAL);
        let mut sighup = signal(SignalKind::hangup())?;

        loop {
            tokio::select! {
                _ = sync_interval.tick() => {
                    if let Err(e) = self.sync().await {
                        tracing::error!("Sync failed: {:?}", e);
                        self.set_sync_observation("error", e.to_string());
                        if let Err(save_err) = self.persist_runtime_state() {
                            tracing::warn!("Failed to persist runtime state after sync error: {:?}", save_err);
                        }
                        metrics::record_sync_failure();
                    } else {
                        metrics::record_sync_success(self.last_sync_peers.lock().await.len());
                    }
                }

                _ = self.sync_now.notified() => {
                    tracing::info!("External sync notification received, syncing now...");
                    if let Err(e) = self.sync().await {
                        tracing::error!("Immediate sync failed: {:?}", e);
                        self.set_sync_observation("error", e.to_string());
                    } else {
                        tracing::info!("✅ Immediate sync completed");
                        metrics::record_sync_success(self.last_sync_peers.lock().await.len());
                        // 重置定时器，避免刚同步完又立即触发定时同步
                        sync_interval.reset();
                    }
                }

                _ = metrics_timer.tick() => {

                    if let Err(e) = self.collect_and_report_metrics().await {
                        tracing::error!("Metrics collection failed: {:?}", e);
                    }
                }

                _ = certificate_renew_timer.tick() => {
                    if let Err(e) = self.maybe_renew_certificate().await {
                        tracing::warn!("Certificate renewal check failed: {:?}", e);
                    }
                }

                _ = sighup.recv() => {
                    tracing::info!("SIGHUP received, reloading config...");
                    let old_interval = self.config.sync_interval;
                    if let Err(e) = self.reload_config().await {
                        tracing::error!("Config reload failed: {:?}", e);
                    } else {
                        if self.config.sync_interval != old_interval {
                            sync_interval = tokio::time::interval(self.config.sync_interval);
                            sync_interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
                            tracing::info!("Applied new sync interval: {:?}", self.config.sync_interval);
                        }
                        metrics::record_config_reload();
                    }
                }

                maybe_command = remote_command_rx.recv() => {
                    match maybe_command {
                        Some(envelope) => {
                            let old_interval = self.config.sync_interval;
                            let response = self.execute_remote_command(envelope.request).await;

                            // 检查 sync_interval 是否变更（可能通过 config_reload 命令）
                            if self.config.sync_interval != old_interval {
                                sync_interval = tokio::time::interval(self.config.sync_interval);
                                sync_interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
                                tracing::info!("Applied new sync interval after remote command: {:?}", self.config.sync_interval);
                            }

                            if let Err(_) = envelope.reply_tx.send(response) {

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
        let sync_now = self.sync_now.clone();
        let node_id = self.config.node_id.clone();
        let public_key = self.config.public_key.clone();
        let runtime_credential = self.runtime_credential.clone();

        tokio::spawn(async move {
            loop {
                if cancel_token.is_cancelled() {
                    break;
                }

                let current_credential = runtime_credential.snapshot().await;
                match grpc_client
                    .connect_command_stream(
                        node_id.clone(),
                        public_key.clone(),
                        current_credential.clone(),
                    )
                    .await
                {
                    Ok((response_tx, mut request_stream)) => {
                        tracing::info!("Controller command stream connected");

                        // 连接成功，通知主循环立即执行一次 Sync
                        sync_now.notify_one();

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
                                                node_id: node_id.clone().unwrap_or_default(),
                                                public_key: public_key.clone(),
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
        use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
        use tokio::net::UnixListener;

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
                                        let peers_snapshot = last_sync_peers.lock().await.clone();

                                        let response = match serde_json::from_str::<UnixRequest>(&line) {
                                            Ok(req) => {
                                                Self::handle_unix_command(
                                                    req,
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
                                let region = last_sync
                                    .iter()
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
                            data: Some(
                                serde_json::json!({"peers": peers_json, "total": peers_json.len()}),
                            ),
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
                })
                .await;

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
                        message: Some(format!(
                            "Failed to list routes: {}",
                            String::from_utf8_lossy(&output.stderr)
                        )),
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
                        Err(e) => UnixResponse {
                            success: false,
                            message: Some(format!("Failed to update log level: {}", e)),
                            data: None,
                        },
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
                        self.last_sync_peers.lock().await.len().to_string(),
                    );
                    if let Some(value) = self
                        .config
                        .last_desired_version
                        .clone()
                        .filter(|value| !value.trim().is_empty())
                    {
                        result.insert("desired_state_version".to_string(), value);
                    }
                    if let Some(value) = self
                        .config
                        .last_applied_version
                        .clone()
                        .filter(|value| !value.trim().is_empty())
                    {
                        result.insert("applied_state_version".to_string(), value);
                    }
                    if let Some(value) = self
                        .config
                        .last_sync_status
                        .clone()
                        .filter(|value| !value.trim().is_empty())
                    {
                        result.insert("observed_state".to_string(), value);
                    }
                    if let Some(value) = self
                        .config
                        .last_sync_message
                        .clone()
                        .filter(|value| !value.trim().is_empty())
                    {
                        result.insert("observed_message".to_string(), value);
                    }
                    if let Some(value) = self.config.last_sync_at {
                        result.insert("observed_at".to_string(), value.to_string());
                    }
                    build_completed_command_response(
                        command_id.clone(),
                        "sync completed".to_string(),
                        result,
                    )
                }
                Err(e) => {
                    build_failed_command_response(command_id.clone(), format!("sync failed: {}", e))
                }
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
        let reported_agent_id = self
            .config
            .node_id
            .clone()
            .filter(|value| !value.trim().is_empty())
            .unwrap_or_else(|| self.config.public_key.clone());
        result.insert("agent_id".to_string(), reported_agent_id);
        result.insert("public_key".to_string(), self.config.public_key.clone());
        if let Some(node_id) = self
            .config
            .node_id
            .clone()
            .filter(|value| !value.trim().is_empty())
        {
            result.insert("node_id".to_string(), node_id);
        }
        result.insert(
            "interface_name".to_string(),
            self.config.interface_name.clone(),
        );
        result.insert(
            "hostname".to_string(),
            self.config
                .hostname
                .clone()
                .unwrap_or_else(|| "unknown".to_string()),
        );
        result.insert(
            "sync_interval_secs".to_string(),
            self.config.sync_interval.as_secs().to_string(),
        );
        result.insert(
            "last_sync_peer_count".to_string(),
            self.last_sync_peers.lock().await.len().to_string(),
        );

        let wg = self.wg_manager.lock().await;
        match wg.list_peers() {
            Ok(peers) => {
                result.insert("wireguard_peer_count".to_string(), peers.len().to_string());
                build_completed_command_response(command_id, "agent healthy".to_string(), result)
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
        let peers_snapshot = self.last_sync_peers.lock().await.clone();
        let response_raw = Self::handle_unix_command(
            UnixRequest {
                cmd: request.command.clone(),
                args,
            },
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
                        response
                            .message
                            .unwrap_or_else(|| "command completed".to_string()),
                        result,
                    )
                } else {
                    build_failed_command_response_with_result(
                        request.command_id,
                        response
                            .message
                            .unwrap_or_else(|| "command failed".to_string()),
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

        let sync_result = self
            .grpc_client
            .sync_with_state(
                self.config.node_id.clone(),
                self.config.public_key.clone(),
                self.config.last_applied_version.clone(),
                self.config.last_sync_status.clone(),
                self.config.last_sync_message.clone(),
                self.config.public_ip.clone(),
                self.config.public_endpoint.clone(),
                self.config.current_credential.clone(),
            )
            .await?;

        apply_runtime_token_from_sync(&mut self.config, &self.runtime_credential, &sync_result)
            .await;

        tracing::debug!(
            "Sync received: {} peers, {} ACL rules, {} blacklist rules, {} QoS rules",
            sync_result.peers.len(),
            sync_result.acl_rules.len(),
            sync_result.blacklist_rules.len(),
            sync_result.qos_rules.len()
        );

        self.sync_peers(&sync_result.peers).await?;
        self.sync_advertised_routes(&sync_result.peers).await?;
        *self.last_sync_peers.lock().await = sync_result.peers.clone();

        let mut apply_errors = Vec::new();
        if let Err(e) = self
            .sync_acl_qos_snapshot(
                &sync_result.acl_rules,
                &sync_result.blacklist_rules,
                &sync_result.qos_rules,
                &sync_result.ip_groups,
            )
            .await
        {
            tracing::error!("Failed to sync ACL/QoS snapshot: {:?}", e);
            apply_errors.push(format!("acl_qos: {}", e));
        }

        if !apply_errors.is_empty() {
            let message = format!("sync apply failed: {}", apply_errors.join("; "));
            self.set_sync_observation("error", message.clone());
            if let Err(e) = self.persist_runtime_state() {
                tracing::warn!("Failed to persist runtime state after apply error: {:?}", e);
            }
            return Err(anyhow::anyhow!(message));
        }

        let desired_state_version = sync_result.desired_state_version.clone();
        let assigned_ip = sync_result.assigned_ip.clone();

        if !assigned_ip.trim().is_empty() {
            self.config.assigned_ip = Some(assigned_ip.clone());
            self.config.address = Some(format!("{}/32", assigned_ip));
        }
        if !desired_state_version.trim().is_empty() {
            self.config.last_desired_version = Some(desired_state_version.clone());
            self.config.last_applied_version = Some(desired_state_version);
        }
        if sync_result.snapshot_complete && !sync_result.domain_versions.is_empty() {
            self.config.latest_domain_versions = sync_result.domain_versions.clone();
        }
        self.set_sync_observation("applied", "sync applied successfully".to_string());

        if let Err(e) = self.persist_runtime_state() {
            tracing::warn!("Failed to persist runtime state after sync: {:?}", e);
        }
        tracing::debug!("Sync completed");
        Ok(())
    }

    async fn sync_acl_qos_snapshot(
        &mut self,
        acl_rules: &[AclRule],
        blacklist_rules: &[GrpcBlacklistRule],
        qos_rules: &[GrpcQoSRule],
        ip_groups: &[GrpcIPGroup],
    ) -> Result<()> {
        tracing::info!(
            "Syncing ACL/QoS snapshot: {} ACL, {} blacklist, {} QoS rules",
            acl_rules.len(),
            blacklist_rules.len(),
            qos_rules.len()
        );

        let mut snapshot = AclQosSnapshot {
            ip_groups: ip_groups
                .iter()
                .map(|group| IPGroupSpec {
                    id: group.id.clone(),
                    name: group.name.clone(),
                    cidrs: group.cidrs.clone(),
                    kind: group.kind.clone(),
                })
                .collect(),
            acl_rules: Vec::new(),
            qos_rules: Vec::new(),
            acl_enabled: true,
            qos_enabled: true,
        };

        for rule in acl_rules.iter() {
            let policy = acl_policy_from_sync_rule(rule)?;
            for direction in requested_directions(policy.direction) {
                snapshot.acl_rules.push(AclRuleSpec {
                    id: policy.id.clone(),
                    src_group: policy.src_group.clone(),
                    dst_group: policy.dst_group.clone(),
                    src_group_id: policy.src_group_id.clone(),
                    dst_group_id: policy.dst_group_id.clone(),
                    proto: policy.proto,
                    action: policy.action,
                    priority: policy.priority,
                    direction,
                    ports: policy.ports.clone(),
                });
            }
        }

        for rule in blacklist_rules {
            match rule.scope.as_str() {
                "src" if !rule.cidr.is_empty() => snapshot.acl_rules.push(AclRuleSpec {
                    id: String::new(),
                    src_group: rule.cidr.clone(),
                    dst_group: "any".to_string(),
                    src_group_id: String::new(),
                    dst_group_id: String::new(),
                    proto: 0,
                    action: ACTION_DROP,
                    priority: BLACKLIST_ACL_PRIORITY,
                    direction: DIRECTION_INGRESS,
                    ports: None,
                }),
                "dst" if !rule.cidr.is_empty() => snapshot.acl_rules.push(AclRuleSpec {
                    id: String::new(),
                    src_group: "any".to_string(),
                    dst_group: rule.cidr.clone(),
                    src_group_id: String::new(),
                    dst_group_id: String::new(),
                    proto: 0,
                    action: ACTION_DROP,
                    priority: BLACKLIST_ACL_PRIORITY,
                    direction: DIRECTION_INGRESS,
                    ports: None,
                }),
                "ports" if rule.port > 0 => {
                    let ports = rule.port.to_string();
                    for proto in [6u8, 17u8] {
                        snapshot.acl_rules.push(AclRuleSpec {
                            id: String::new(),
                            src_group: "any".to_string(),
                            dst_group: "any".to_string(),
                            src_group_id: String::new(),
                            dst_group_id: String::new(),
                            proto,
                            action: ACTION_DROP,
                            priority: BLACKLIST_ACL_PRIORITY,
                            direction: DIRECTION_INGRESS,
                            ports: Some(ports.clone()),
                        });
                    }
                }
                _ => return Err(anyhow::anyhow!("invalid blacklist rule payload")),
            }
        }

        for rule in qos_rules {
            let policy = qos_policy_from_sync_rule(rule)?;
            if policy.rate_bps == 0 {
                return Err(anyhow::anyhow!("QoS rate_bps must be greater than zero"));
            }
            for direction in requested_directions(policy.direction) {
                snapshot.qos_rules.push(QosRuleSpec {
                    id: policy.id.clone(),
                    group: policy.group.clone(),
                    group_id: policy.group_id.clone(),
                    direction,
                    rate_bps: policy.rate_bps,
                    burst_bytes: policy.burst_bytes,
                    priority: policy.priority,
                    mode: policy.mode,
                });
            }
        }

        let acl_qos_mgr = self.acl_qos_mgr.clone();
        let applied_counts = (snapshot.acl_rules.len(), snapshot.qos_rules.len());
        tokio::task::spawn_blocking(move || -> Result<()> {
            let mut mgr = acl_qos_mgr.blocking_lock();
            mgr.apply_snapshot(snapshot).map_err(|e| anyhow::anyhow!(e))
        })
        .await??;

        metrics::record_acl_rule_count(applied_counts.0);
        metrics::record_qos_rule_count(applied_counts.1);
        tracing::info!(
            "ACL/QoS snapshot applied: {} ACL policies, {} QoS policies",
            applied_counts.0,
            applied_counts.1
        );
        Ok(())
    }

    async fn sync_peers(&mut self, new_peers: &[GrpcPeerInfo]) -> Result<()> {
        let new_peers = new_peers.to_vec();
        let _multi_tunnel = self.config.multi_tunnel;
        let interfaces = self.get_active_interfaces();
        let wg_managers = self.wg_managers.clone();
        let base_iface = self.config.interface_name.clone();
        let listen_port = self.config.listen_port;

        let result =
            tokio::task::spawn_blocking(move || -> Result<(usize, usize, usize, usize)> {
                let interface_count = interfaces.len();

                let mut total_added = 0;
                let mut total_removed = 0;
                let mut total_updated = 0;

                for iface in &interfaces {
                    let mgr = wg_managers
                        .get(iface)
                        .unwrap_or_else(|| panic!("WireGuardManager for {} not found", iface));
                    let mut wg = mgr.blocking_lock();

                    let current_peers = wg.list_peers().context("Failed to list current peers")?;

                    let (to_add, to_remove, to_update) =
                        Self::diff_peers_static(&current_peers, &new_peers);

                    // 删除 peer
                    for peer in &to_remove {
                        if iface == &base_iface {
                            tracing::info!(
                                "Removing peer {} from {}...",
                                &peer[..16.min(peer.len())],
                                iface
                            );
                        }
                        wg.remove_peer(&peer).context("Failed to remove peer")?;
                        if iface == &base_iface {
                            metrics::record_wireguard_peer_change("remove");
                        }
                    }
                    total_removed += to_remove.len();

                    // 添加 peer
                    for peer in &to_add {
                        if iface == &base_iface {
                            tracing::info!(
                                "Adding peer {} to {}...",
                                &peer.public_key[..16.min(peer.public_key.len())],
                                iface
                            );
                        }

                        let mut allowed_ips = vec![format!("{}/32", peer.assigned_ip)];
                        allowed_ips.extend(peer.advertised_routes.clone());

                        // 根据 iface 编号调整 endpoint 端口
                        let endpoint = if !peer.endpoint.is_empty() {
                            let adjusted_endpoint = Self::adjust_endpoint_port_static(
                                &peer.endpoint,
                                iface,
                                &base_iface,
                                listen_port,
                            );
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

                        wg.add_peer(peer_config).context("Failed to add peer")?;
                        if iface == &base_iface {
                            metrics::record_wireguard_peer_change("add");
                        }
                    }
                    total_added += to_add.len();

                    // 更新 peer
                    for peer in &to_update {
                        if iface == &base_iface {
                            tracing::debug!(
                                "Updating peer {} on {}...",
                                &peer.public_key[..16.min(peer.public_key.len())],
                                iface
                            );
                        }

                        wg.remove_peer(&peer.public_key)
                            .context("Failed to remove peer for update")?;

                        let mut allowed_ips = vec![format!("{}/32", peer.assigned_ip)];
                        allowed_ips.extend(peer.advertised_routes.clone());

                        // 根据 iface 编号调整 endpoint 端口
                        let endpoint = if !peer.endpoint.is_empty() {
                            let adjusted_endpoint = Self::adjust_endpoint_port_static(
                                &peer.endpoint,
                                iface,
                                &base_iface,
                                listen_port,
                            );
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
                        if iface == &base_iface {
                            metrics::record_wireguard_peer_change("update");
                        }
                    }
                    total_updated += to_update.len();
                }

                Ok((
                    total_added / interface_count,
                    total_removed / interface_count,
                    total_updated / interface_count,
                    new_peers.len(),
                ))
            })
            .await?;

        match result {
            Ok((added, removed, updated, total)) => {
                let total_changes = added + removed + updated;
                if total_changes > 0 {
                    tracing::info!(
                        "Peer sync: +{} -{} ~{} (total: {})",
                        added,
                        removed,
                        updated,
                        total
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

    fn adjust_endpoint_port_static(
        endpoint: &str,
        iface: &str,
        base_iface: &str,
        base_port: u16,
    ) -> String {
        // Calculate offset based on trailing digit or comparison
        let offset = if iface == base_iface {
            0
        } else if let Some(last_char) = iface.chars().last() {
            if last_char.is_numeric() {
                last_char.to_digit(10).unwrap_or(0) as u16
            } else {
                0
            }
        } else {
            0
        };

        if let Some(colon_pos) = endpoint.rfind(':') {
            let host = &endpoint[..colon_pos];
            let port_str = &endpoint[colon_pos + 1..];

            let port = if let Ok(orig_port) = port_str.parse::<u16>() {
                orig_port + offset
            } else {
                base_port + offset
            };

            format!("{}:{}", host, port)
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
            let current_peer = current
                .iter()
                .find(|p| p.public_key == desired_peer.public_key);

            if let Some(current) = current_peer {
                let desired_endpoint = if desired_peer.endpoint.is_empty() {
                    None
                } else {
                    Some(desired_peer.endpoint.as_str())
                };

                if current.endpoint.as_deref() != desired_endpoint
                    || current.allowed_ips.get(0).map(|s| s.as_str())
                        != Some(&desired_peer.assigned_ip)
                {
                    to_update.push(desired_peer.clone());
                }
            } else {
                to_add.push(desired_peer.clone());
            }
        }

        for current_peer in current {
            if !desired
                .iter()
                .any(|p| p.public_key == current_peer.public_key)
            {
                to_remove.push(current_peer.public_key.clone());
            }
        }

        (to_add, to_remove, to_update)
    }

    async fn sync_advertised_routes(&mut self, peers: &[GrpcPeerInfo]) -> Result<()> {
        use std::collections::HashSet as StdHashSet;

        // 收集期望的所有路由
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

        // 确定要使用的接口列表
        let interfaces = self.get_active_interfaces();
        let multi_tunnel = self.config.multi_tunnel;

        // 在单个阻塞任务中完成所有路由操作，保证原子性和性能
        let routing_manager = self.routing_manager.clone();
        let result = tokio::task::spawn_blocking(move || -> Result<(usize, usize, usize)> {
            // 获取当前路由
            let current_routes = routing_manager
                .list_vpn_routes()
                .context("Failed to list current VPN routes")?;

            // 计算差异
            let to_remove: Vec<_> = current_routes
                .difference(&desired_routes)
                .cloned()
                .collect();

            let mut added_count = 0;
            let mut removed_count = 0;
            let mut failures = Vec::new();

            // 删除多余的路由
            for route in &to_remove {
                if let Err(e) = routing_manager.remove_vpn_route(route) {
                    tracing::error!("Failed to remove stale route {}: {:?}", route, e);
                    failures.push(format!("remove {}: {}", route, e));
                } else {
                    removed_count += 1;
                    tracing::info!("Removed stale route: {}", route);
                }
            }

            // 添加或更新所有期望的路由（使用 replace，会自动替换旧路由）
            for route in &desired_routes {
                if multi_tunnel {
                    // 多隧道模式：使用 ECMP 路由
                    let interfaces_str: Vec<&str> = interfaces.iter().map(|s| s.as_str()).collect();
                    if let Err(e) =
                        routing_manager.add_ecmp_route(route, &interfaces_str, Some(100))
                    {
                        tracing::error!("Failed to add ECMP route {}: {:?}", route, e);
                        failures.push(format!("add ecmp {}: {}", route, e));
                    } else {
                        added_count += 1;
                    }
                } else {
                    // 单接口模式
                    if let Err(e) = routing_manager.add_vpn_route(route) {
                        tracing::error!("Failed to add route {}: {:?}", route, e);
                        failures.push(format!("add {}: {}", route, e));
                    } else {
                        added_count += 1;
                    }
                }
            }

            fail_route_sync_if_needed(&failures)?;
            Ok((added_count, removed_count, desired_routes.len()))
        })
        .await?;

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

    async fn collect_and_report_metrics(&self) -> Result<()> {
        tracing::trace!("Collecting metrics...");

        let wg_manager = self.wg_manager.clone();
        let acl_qos_mgr = self.acl_qos_mgr.clone();

        let result = tokio::task::spawn_blocking(move || -> Result<(
            Vec<(String, Option<String>, u64, u64, Option<u64>)>, // peers
            (usize, usize, u64, u64), // wg totals
            Vec<(u32, &'static str, u64, u64)>, // acl stats
            Vec<(&'static str, u32, u64, u64)> // qos stats
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

            let acl_qos = acl_qos_mgr.blocking_lock();
            let acl_stats = acl_qos.get_all_rule_stats().unwrap_or_default();
            let qos_stats = acl_qos.get_all_qos_stats().unwrap_or_default();

            Ok((peer_stats, wg_totals, acl_stats, qos_stats))
        }).await?;

        // 在异步上下文中记录 metrics
        match result {
            Ok((peer_stats, wg_totals, acl_stats, qos_stats)) => {
                // WireGuard metrics
                metrics::record_wireguard_totals(
                    wg_totals.0,
                    wg_totals.1,
                    wg_totals.2,
                    wg_totals.3,
                );

                for (public_key, endpoint, rx_bytes, tx_bytes, last_handshake) in peer_stats {
                    metrics::record_wireguard_peer_stats(
                        &public_key,
                        endpoint.as_deref(),
                        rx_bytes,
                        tx_bytes,
                        last_handshake,
                    );
                }

                let mut custom_metrics = HashMap::new();
                let mut acl_packets = 0_u64;
                let mut acl_bytes = 0_u64;
                let mut acl_dropped_packets = 0_u64;
                let mut acl_dropped_bytes = 0_u64;

                // ACL metrics
                for (rule_id, action, packets, bytes) in &acl_stats {
                    metrics::record_acl_rule_stats(*rule_id, *action, *packets, *bytes);
                    if *action == "drop" {
                        acl_dropped_packets = acl_dropped_packets.saturating_add(*packets);
                        acl_dropped_bytes = acl_dropped_bytes.saturating_add(*bytes);
                    } else {
                        acl_packets = acl_packets.saturating_add(*packets);
                        acl_bytes = acl_bytes.saturating_add(*bytes);
                    }
                }

                let mut qos_passed_bytes = 0_u64;
                let mut qos_dropped_bytes = 0_u64;
                let mut qos_shaped_bytes = 0_u64;
                // QoS metrics
                for (rule_type, rule_id, passed, dropped) in &qos_stats {
                    metrics::record_qos_rule_stats(rule_type, *rule_id, *passed, *dropped);
                    qos_passed_bytes = qos_passed_bytes.saturating_add(*passed);
                    qos_dropped_bytes = qos_dropped_bytes.saturating_add(*dropped);
                }

                let acl_qos_mgr = self.acl_qos_mgr.clone();
                let rule_stats = tokio::task::spawn_blocking(move || {
                    let mgr = acl_qos_mgr.blocking_lock();
                    let acl = mgr
                        .get_acl_rule_runtime_stats()
                        .map_err(|e| anyhow::anyhow!(e))?;
                    let qos = mgr
                        .get_qos_rule_runtime_stats()
                        .map_err(|e| anyhow::anyhow!(e))?;
                    Ok::<_, anyhow::Error>((acl, qos))
                })
                .await??;
                for stat in rule_stats.0 {
                    custom_metrics
                        .insert(format!("acl_rule.{}.packets", stat.id), stat.packets as f64);
                    custom_metrics.insert(format!("acl_rule.{}.bytes", stat.id), stat.bytes as f64);
                    custom_metrics.insert(
                        format!("acl_rule.{}.dropped_packets", stat.id),
                        stat.dropped_packets as f64,
                    );
                    custom_metrics.insert(
                        format!("acl_rule.{}.dropped_bytes", stat.id),
                        stat.dropped_bytes as f64,
                    );
                }
                for stat in rule_stats.1 {
                    qos_shaped_bytes = qos_shaped_bytes.saturating_add(stat.shaped_bytes);
                    custom_metrics.insert(
                        format!("qos_rule.{}.passed_bytes", stat.id),
                        stat.passed_bytes as f64,
                    );
                    custom_metrics.insert(
                        format!("qos_rule.{}.dropped_bytes", stat.id),
                        stat.dropped_bytes as f64,
                    );
                    custom_metrics.insert(
                        format!("qos_rule.{}.shaped_bytes", stat.id),
                        stat.shaped_bytes as f64,
                    );
                }

                custom_metrics.insert("acl_packets".to_string(), acl_packets as f64);
                custom_metrics.insert("acl_bytes".to_string(), acl_bytes as f64);
                custom_metrics.insert(
                    "acl_dropped_packets".to_string(),
                    acl_dropped_packets as f64,
                );
                custom_metrics.insert("acl_dropped_bytes".to_string(), acl_dropped_bytes as f64);
                custom_metrics.insert("qos_passed_bytes".to_string(), qos_passed_bytes as f64);
                custom_metrics.insert("qos_dropped_bytes".to_string(), qos_dropped_bytes as f64);
                custom_metrics.insert("qos_shaped_bytes".to_string(), qos_shaped_bytes as f64);

                let runtime_token = self.runtime_credential.snapshot().await;
                if let Err(e) = self
                    .grpc_client
                    .report_metrics(
                        self.config.node_id.clone(),
                        self.config.public_key.clone(),
                        custom_metrics,
                        runtime_token,
                    )
                    .await
                {
                    tracing::warn!("Failed to report policy metrics: {:?}", e);
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
        tracing::info!(
            "Reconnecting to Controller at {}...",
            self.config.controller_url
        );

        let new_client = GrpcClient::new_with_options(
            self.config.controller_url.clone(),
            self.config.ca_cert.clone(),
            self.config.client_cert.clone(),
            self.config.client_key.clone(),
            self.config.tls_server_name.clone(),
        )
        .await
        .context("Failed to reconnect to Controller")?;

        self.grpc_client = new_client;
        tracing::info!("✅ gRPC client reconnected successfully");
        Ok(())
    }

    async fn maybe_renew_certificate(&mut self) -> Result<()> {
        if self
            .config
            .current_credential
            .as_deref()
            .map(str::trim)
            .unwrap_or("")
            .is_empty()
        {
            return Ok(());
        }
        if self.config.client_cert.trim().is_empty()
            || self.config.client_key.trim().is_empty()
            || self.config.ca_cert.trim().is_empty()
        {
            tracing::debug!(
                "Skipping certificate renewal check because certificate paths are incomplete"
            );
            return Ok(());
        }

        if !certificate_client::should_renew_certificate(
            &self.config.client_cert,
            self.config.certificate_renew_before,
        )? {
            return Ok(());
        }

        let controller_api_url = certificate_client::resolve_controller_api_url(
            &self.config.controller_url,
            Some(&self.config.controller_api_url),
        )?;
        let common_name = self
            .config
            .node_id
            .clone()
            .or_else(|| self.config.hostname.clone())
            .filter(|value| !value.trim().is_empty())
            .unwrap_or_else(|| "aria-agent".to_string());
        let runtime_token = self
            .config
            .current_credential
            .clone()
            .context("runtime credential missing for certificate renewal")?;

        tracing::info!(
            "Client certificate is nearing expiry, requesting renewal from {}",
            controller_api_url
        );
        let renewed = certificate_client::renew_certificate(
            &controller_api_url,
            &runtime_token,
            &self.config.ca_cert,
            &common_name,
        )
        .await?;

        certificate_client::write_renewed_certificate_files(
            &self.config.ca_cert,
            &self.config.client_cert,
            &self.config.client_key,
            &renewed,
        )?;
        self.reconnect_grpc().await?;
        tracing::info!(
            "Client certificate renewed successfully; new expiry at {:?}",
            renewed.not_after
        );
        Ok(())
    }

    async fn reload_config(&mut self) -> Result<()> {
        tracing::info!("Reloading configuration...");

        let config_manager = crate::config::ConfigManager::new(&self.config_path);
        let new_config = config_manager.load()?;

        // 1. 检测 sync_interval 变更
        if new_config.sync_interval != self.config.sync_interval {
            tracing::info!(
                "Sync interval changed: {:?} -> {:?}",
                self.config.sync_interval,
                new_config.sync_interval
            );
            self.config.sync_interval = new_config.sync_interval;
        }

        if new_config.certificate_renew_before != self.config.certificate_renew_before {
            tracing::info!(
                "Certificate renew window changed: {:?} -> {:?}",
                self.config.certificate_renew_before,
                new_config.certificate_renew_before
            );
            self.config.certificate_renew_before = new_config.certificate_renew_before;
        }

        if new_config.controller_api_url != self.config.controller_api_url {
            tracing::info!(
                "Controller API URL changed: {} -> {}",
                self.config.controller_api_url,
                new_config.controller_api_url
            );
            self.config.controller_api_url = new_config.controller_api_url.clone();
        }

        // 2. 检测 Controller URL 变更（需要重连）
        if new_config.controller_url != self.config.controller_url {
            tracing::warn!(
                "Controller URL changed: {} -> {}",
                self.config.controller_url,
                new_config.controller_url
            );

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
                tracing::error!(
                    "Failed to reconnect gRPC client, rolling back config: {:?}",
                    e
                );

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
        else if new_config.ca_cert != self.config.ca_cert
            || new_config.client_cert != self.config.client_cert
            || new_config.client_key != self.config.client_key
            || new_config.tls_server_name != self.config.tls_server_name
        {
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
                tracing::error!(
                    "Failed to reconnect gRPC client with new certificates, rolling back: {:?}",
                    e
                );

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
            tracing::warn!(
                "Listen port changed: {} -> {}",
                self.config.listen_port,
                new_config.listen_port
            );
        }

        if new_config.mtu != self.config.mtu {
            tracing::warn!("MTU changed: {} -> {}", self.config.mtu, new_config.mtu);
        }

        // 5. 检测不应动态变更的配置
        if new_config.public_key != self.config.public_key
            || new_config.private_key != self.config.private_key
        {
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
            "SRC_IPV4_ID_MAP",
            "DST_IPV4_ID_MAP",
            "SRC_IPV6_ID_MAP",
            "DST_IPV6_ID_MAP",
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

async fn apply_runtime_token_from_sync(
    config: &mut AgentConfig,
    runtime_credential: &RuntimeCredentialStore,
    sync_result: &crate::grpc_client::SyncResult,
) {
    let rotated_token = sync_result.runtime_token.is_some();
    if let Some(new_token) = sync_result.runtime_token.clone() {
        config.current_credential = Some(new_token.clone());
        runtime_credential.update(Some(new_token)).await;
    }
    if let Some(expires_at) = sync_result.runtime_token_expires_at {
        config.current_credential_expires_at = Some(expires_at);
    } else if rotated_token {
        config.current_credential_expires_at = None;
    }
}

fn fail_route_sync_if_needed(failures: &[String]) -> Result<()> {
    if failures.is_empty() {
        return Ok(());
    }

    Err(anyhow::anyhow!(
        "route sync failed: {} route operations failed: {}",
        failures.len(),
        failures.join("; ")
    ))
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
        node_id: String::new(),
        public_key: String::new(),
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
        node_id: String::new(),
        public_key: String::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::{apply_runtime_token_from_sync, fail_route_sync_if_needed};
    use crate::config::AgentConfig;
    use crate::grpc_client::SyncResult;
    use crate::runtime_credential::RuntimeCredentialStore;

    #[tokio::test]
    async fn runtime_token_is_recorded_before_local_apply_errors_are_returned() {
        let mut config = AgentConfig::default();
        let store = RuntimeCredentialStore::new(None);
        let sync_result = SyncResult {
            peers: Vec::new(),
            assigned_ip: String::new(),
            desired_state_version: String::new(),
            ip_groups: Vec::new(),
            acl_rules: Vec::new(),
            qos_rules: Vec::new(),
            blacklist_rules: Vec::new(),
            runtime_token: Some("rt.new-token".to_string()),
            runtime_token_expires_at: Some(1_700_000_000),
            snapshot_complete: false,
            domain_versions: Default::default(),
        };

        apply_runtime_token_from_sync(&mut config, &store, &sync_result).await;

        assert_eq!(config.current_credential.as_deref(), Some("rt.new-token"));
        assert_eq!(config.current_credential_expires_at, Some(1_700_000_000));
        assert_eq!(store.snapshot().await.as_deref(), Some("rt.new-token"));
    }

    #[tokio::test]
    async fn runtime_token_rotation_without_expiry_clears_stale_expiry() {
        let mut config = AgentConfig {
            current_credential: Some("rt.old-token".to_string()),
            current_credential_expires_at: Some(1_600_000_000),
            ..Default::default()
        };
        let store = RuntimeCredentialStore::new(Some("rt.old-token".to_string()));
        let sync_result = SyncResult {
            peers: Vec::new(),
            assigned_ip: String::new(),
            desired_state_version: String::new(),
            ip_groups: Vec::new(),
            acl_rules: Vec::new(),
            qos_rules: Vec::new(),
            blacklist_rules: Vec::new(),
            runtime_token: Some("rt.new-token".to_string()),
            runtime_token_expires_at: None,
            snapshot_complete: false,
            domain_versions: Default::default(),
        };

        apply_runtime_token_from_sync(&mut config, &store, &sync_result).await;

        assert_eq!(config.current_credential.as_deref(), Some("rt.new-token"));
        assert_eq!(config.current_credential_expires_at, None);
        assert_eq!(store.snapshot().await.as_deref(), Some("rt.new-token"));
    }

    #[test]
    fn route_sync_failures_return_error() {
        let failures = vec![
            "add 10.10.0.0/16: netlink failed".to_string(),
            "remove 10.20.0.0/16: not permitted".to_string(),
        ];

        let err = fail_route_sync_if_needed(&failures).expect_err("expected route sync failure");
        let message = err.to_string();

        assert!(message.contains("2 route operations failed"));
        assert!(message.contains("add 10.10.0.0/16"));
        assert!(message.contains("remove 10.20.0.0/16"));
    }
}
