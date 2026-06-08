use std::os::unix::net::UnixStream;
use std::io::{BufReader, BufWriter, BufRead, Write};
use std::net::{IpAddr, Ipv4Addr, UdpSocket};
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use serde::{Deserialize, Serialize};
use tracing::info;
use tracing_subscriber::{
    layer::SubscriberExt,
    util::SubscriberInitExt,
    EnvFilter,
    Registry,
    reload,
    fmt,
};

mod identity;
mod config;
mod metrics;
mod unified_agent;
mod grpc_client;
mod certificate_client;
mod wireguard;
mod routing;
mod system_optimization;
mod runtime_credential;
mod acl_qos_state;
mod acl_qos_maps;
mod acl_qos_manager;

const SOCKET_PATH: &str = "/run/aria-agent.sock";
const DEFAULT_CONFIG_PATH: &str = "/etc/aria/agent.yaml";

type LogLevelHandle = Arc<Mutex<Option<reload::Handle<EnvFilter, Registry>>>>;

#[derive(Parser)]
#[command(name = "aria-agent")]
#[command(about = "Aria Agent - eBPF-based QoS and ACL", long_about = None)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    Up {
        #[arg(short, long)]
        interface: String,
        #[arg(short = 'l', long, default_value = "info")]
        log_level: String,
        #[arg(long, default_value = "0.0.0.0:9090")]
        metrics_addr: String,
        #[arg(long)]
        config: Option<String>,
    },
    Init {
        #[arg(long)]
        server: String,
        #[arg(long)]
        token: String,
        #[arg(long)]
        public_ip: Option<String>,
        #[arg(long)]
        public_endpoint: Option<String>,
        #[arg(long)]
        hostname: Option<String>,
        #[arg(long)]
        region: Option<String>,
        #[arg(long)]
        interface: Option<String>,
        #[arg(long)]
        ca_cert: Option<String>,
        #[arg(long)]
        client_cert: Option<String>,
        #[arg(long)]
        client_key: Option<String>,
        #[arg(long)]
        controller_api_url: Option<String>,
        #[arg(long)]
        tls_server_name: Option<String>,
        #[arg(long)]
        config: Option<String>,
        #[arg(long)]
        advertise_routes: Option<String>,
    },
    Status,
    Peers,
    Route,
    Down,
    Qos {
        #[command(subcommand)]
        cmd: QosCommands,
    },
    Acl {
        #[command(subcommand)]
        cmd: AclCommands,
    },
    Log {
        #[arg(short, long)]
        level: String,
    },
}

#[derive(Subcommand)]
enum QosCommands {
    LimitIp {
        #[arg(short, long)]
        ip: String,
        #[arg(short = 'b', long)]
        mbps: u64,
    },
    LimitPeer {
        #[arg(long)]
        src_ip: String,
        #[arg(long)]
        dst_ip: String,
        #[arg(short = 'b', long)]
        mbps: u64,
    },
    LimitPort {
        #[arg(short, long)]
        port: u16,
        #[arg(short = 'b', long)]
        mbps: u64,
        #[arg(long, default_value = "0")]
        protocol: u8,
    },
    LimitService {
        #[arg(long)]
        src_ip: Option<String>,
        #[arg(long)]
        dst_ip: Option<String>,
        #[arg(long)]
        src_port: u16,
        #[arg(long)]
        dst_port: u16,
        #[arg(long, default_value = "6")]
        protocol: u8,
        #[arg(short = 'b', long)]
        mbps: u64,
    },
    RemoveIp {
        #[arg(short, long)]
        ip: String,
    },
    RemovePeer {
        #[arg(long)]
        src_ip: String,
        #[arg(long)]
        dst_ip: String,
    },
    RemoveService {
        #[arg(long)]
        src_ip: Option<String>,
        #[arg(long)]
        dst_ip: Option<String>,
        #[arg(long)]
        src_port: u16,
        #[arg(long)]
        dst_port: u16,
        #[arg(long, default_value = "6")]
        protocol: u8,
    },
    StatsIp {
        #[arg(short, long)]
        ip: String,
    },
    StatsPeer {
        #[arg(long)]
        src_ip: String,
        #[arg(long)]
        dst_ip: String,
    },
    StatsService {
        #[arg(long)]
        src_ip: Option<String>,
        #[arg(long)]
        dst_ip: Option<String>,
        #[arg(long)]
        src_port: u16,
        #[arg(long)]
        dst_port: u16,
        #[arg(long, default_value = "6")]
        protocol: u8,
    },
}

#[derive(Subcommand)]
enum AclCommands {
    BlockSrcIp {
        #[arg(short, long)]
        ip: String,
    },
    UnblockSrcIp {
        #[arg(short, long)]
        ip: String,
    },
    BlockDstIp {
        #[arg(short, long)]
        ip: String,
    },
    UnblockDstIp {
        #[arg(short, long)]
        ip: String,
    },
    BlockPort {
        #[arg(short, long)]
        port: u16,
    },
    UnblockPort {
        #[arg(short, long)]
        port: u16,
    },
    Allow {
        #[arg(long)]
        src_ip: Option<String>,
        #[arg(long)]
        dst_ip: Option<String>,
        #[arg(long, default_value = "0")]
        src_port: u16,
        #[arg(long)]
        dst_port: u16,
        #[arg(long, default_value = "6")]
        protocol: u8,
    },
    Deny {
        #[arg(long)]
        src_ip: Option<String>,
        #[arg(long)]
        dst_ip: Option<String>,
        #[arg(long, default_value = "0")]
        src_port: u16,
        #[arg(long)]
        dst_port: u16,
        #[arg(long, default_value = "6")]
        protocol: u8,
    },
    RemoveRule {
        #[arg(long)]
        src_ip: Option<String>,
        #[arg(long)]
        dst_ip: Option<String>,
        #[arg(long, default_value = "0")]
        src_port: u16,
        #[arg(long)]
        dst_port: u16,
        #[arg(long, default_value = "6")]
        protocol: u8,
    },
    Stats {
        #[arg(long)]
        src_ip: Option<String>,
        #[arg(long)]
        dst_ip: Option<String>,
        #[arg(long, default_value = "0")]
        src_port: u16,
        #[arg(long)]
        dst_port: u16,
        #[arg(long, default_value = "6")]
        protocol: u8,
    },
}

#[derive(Serialize, Deserialize, Debug)]
struct Request {
    cmd: String,
    args: serde_json::Value,
}

#[derive(Serialize, Deserialize, Debug)]
struct Response {
    success: bool,
    message: Option<String>,
    data: Option<serde_json::Value>,
}

fn main() -> Result<()> {
    // 安装 rustls CryptoProvider (必须在任何 TLS 操作之前)
    rustls::crypto::aws_lc_rs::default_provider()
        .install_default()
        .expect("Failed to install rustls aws_lc provider");

    let cli = Cli::parse();

    match cli.command {
        Commands::Up { interface, log_level, metrics_addr, config } => {
            let rt = tokio::runtime::Runtime::new()
                .context("Failed to create tokio runtime")?;
            rt.block_on(run_unified_agent(&interface, &log_level, &metrics_addr, config.as_deref()))?;
        }
        Commands::Init {
            server,
            token,
            public_ip,
            public_endpoint,
            hostname,
            region,
            interface,
            ca_cert,
            client_cert,
            client_key,
            controller_api_url,
            tls_server_name,
            config,
            advertise_routes,
        } => {
            run_init(
                &server,
                &token,
                public_ip.as_deref(),
                public_endpoint.as_deref(),
                hostname.as_deref(),
                region.as_deref(),
                interface.as_deref(),
                ca_cert.as_deref(),
                client_cert.as_deref(),
                client_key.as_deref(),
                controller_api_url.as_deref(),
                tls_server_name.as_deref(),
                config.as_deref(),
                advertise_routes.as_deref(),
            )?
        }
        Commands::Status => send_status_command()?,
        Commands::Peers => send_peers_command()?,
        Commands::Route => send_route_command()?,
        Commands::Down => send_down_command()?,
        Commands::Qos { cmd } => send_qos_command(cmd)?,
        Commands::Acl { cmd } => send_acl_command(cmd)?,
        Commands::Log { level } => send_log_command(&level)?,
    }

    Ok(())
}

fn init_logging(level: &str) -> Result<LogLevelHandle> {
    let filter = EnvFilter::try_new(level)
        .context("Invalid log level")?;
    
    let (filter_layer, reload_handle) = reload::Layer::new(filter);
    let fmt_layer = fmt::layer();

    Registry::default()
        .with(filter_layer)
        .with(fmt_layer)
        .try_init()
        .context("Failed to initialize logging")?;
    
    let handle = Arc::new(Mutex::new(Some(reload_handle)));
    info!("Initialized logging with level: {}", level);
    
    Ok(handle)
}


fn send_qos_command(cmd: QosCommands) -> Result<()> {
    let stream = UnixStream::connect(SOCKET_PATH)?;
    let mut writer = BufWriter::new(&stream);
    let mut reader = BufReader::new(&stream);

    let req = match cmd {
        QosCommands::LimitIp { ip, mbps } => Request {
            cmd: "qos_limit_ip".to_string(),
            args: serde_json::json!({"ip": ip, "mbps": mbps}),
        },
        QosCommands::LimitPeer { src_ip, dst_ip, mbps } => Request {
            cmd: "qos_limit_peer".to_string(),
            args: serde_json::json!({"src_ip": src_ip, "dst_ip": dst_ip, "mbps": mbps}),
        },
        QosCommands::LimitPort { port, mbps, protocol } => Request {
            cmd: "qos_limit_port".to_string(),
            args: serde_json::json!({"port": port, "mbps": mbps, "protocol": protocol}),
        },
        QosCommands::LimitService { src_ip, dst_ip, src_port, dst_port, protocol, mbps } => Request {
            cmd: "qos_limit_service".to_string(),
            args: serde_json::json!({
                "src_ip": src_ip.unwrap_or_default(),
                "dst_ip": dst_ip.unwrap_or_default(),
                "src_port": src_port,
                "dst_port": dst_port,
                "protocol": protocol,
                "mbps": mbps
            }),
        },
        QosCommands::RemoveIp { ip } => Request {
            cmd: "qos_remove_ip".to_string(),
            args: serde_json::json!({"ip": ip}),
        },
        QosCommands::RemovePeer { src_ip, dst_ip } => Request {
            cmd: "qos_remove_peer".to_string(),
            args: serde_json::json!({"src_ip": src_ip, "dst_ip": dst_ip}),
        },
        QosCommands::RemoveService { src_ip, dst_ip, src_port, dst_port, protocol } => Request {
            cmd: "qos_remove_service".to_string(),
            args: serde_json::json!({
                "src_ip": src_ip.unwrap_or_default(),
                "dst_ip": dst_ip.unwrap_or_default(),
                "src_port": src_port,
                "dst_port": dst_port,
                "protocol": protocol
            }),
        },
        QosCommands::StatsIp { ip } => Request {
            cmd: "qos_stats_ip".to_string(),
            args: serde_json::json!({"ip": ip}),
        },
        QosCommands::StatsPeer { src_ip, dst_ip } => Request {
            cmd: "qos_stats_peer".to_string(),
            args: serde_json::json!({"src_ip": src_ip, "dst_ip": dst_ip}),
        },
        QosCommands::StatsService { src_ip, dst_ip, src_port, dst_port, protocol } => Request {
            cmd: "qos_stats_service".to_string(),
            args: serde_json::json!({
                "src_ip": src_ip.unwrap_or_default(),
                "dst_ip": dst_ip.unwrap_or_default(),
                "src_port": src_port,
                "dst_port": dst_port,
                "protocol": protocol
            }),
        },
    };

    let req_json = serde_json::to_string(&req)?;
    writer.write_all(req_json.as_bytes())?;
    writer.write_all(b"\n")?;
    writer.flush()?;

    let mut resp_line = String::new();
    reader.read_line(&mut resp_line)?;
    let resp: Response = serde_json::from_str(&resp_line)?;

    if resp.success {
        if let Some(msg) = resp.message {
            println!("{}", msg);
        }
        if let Some(data) = resp.data {
            println!("{}", serde_json::to_string_pretty(&data)?);
        }
    } else {
        eprintln!("Error: {}", resp.message.unwrap_or_else(|| "Unknown error".to_string()));
        std::process::exit(1);
    }

    Ok(())
}

fn send_acl_command(cmd: AclCommands) -> Result<()> {
    let stream = UnixStream::connect(SOCKET_PATH)?;
    let mut writer = BufWriter::new(&stream);
    let mut reader = BufReader::new(&stream);

    let req = acl_command_request(cmd);

    let req_json = serde_json::to_string(&req)?;
    writer.write_all(req_json.as_bytes())?;
    writer.write_all(b"\n")?;
    writer.flush()?;

    let mut resp_line = String::new();
    reader.read_line(&mut resp_line)?;
    let resp: Response = serde_json::from_str(&resp_line)?;

    if resp.success {
        if let Some(msg) = resp.message {
            println!("{}", msg);
        }
        if let Some(data) = resp.data {
            println!("{}", serde_json::to_string_pretty(&data)?);
        }
    } else {
        eprintln!("Error: {}", resp.message.unwrap_or_else(|| "Unknown error".to_string()));
        std::process::exit(1);
    }

    Ok(())
}

fn acl_command_request(cmd: AclCommands) -> Request {
    match cmd {
        AclCommands::BlockSrcIp { ip } => Request {
            cmd: "acl_block_src_ip".to_string(),
            args: serde_json::json!({"src_ip": ip}),
        },
        AclCommands::UnblockSrcIp { ip } => Request {
            cmd: "acl_unblock_src_ip".to_string(),
            args: serde_json::json!({"src_ip": ip}),
        },
        AclCommands::BlockDstIp { ip } => Request {
            cmd: "acl_block_dst_ip".to_string(),
            args: serde_json::json!({"dst_ip": ip}),
        },
        AclCommands::UnblockDstIp { ip } => Request {
            cmd: "acl_unblock_dst_ip".to_string(),
            args: serde_json::json!({"dst_ip": ip}),
        },
        AclCommands::BlockPort { port } => Request {
            cmd: "acl_block_port".to_string(),
            args: serde_json::json!({"port": port}),
        },
        AclCommands::UnblockPort { port } => Request {
            cmd: "acl_unblock_port".to_string(),
            args: serde_json::json!({"port": port}),
        },
        AclCommands::Allow { src_ip, dst_ip, src_port, dst_port, protocol } => Request {
            cmd: "acl_allow".to_string(),
            args: serde_json::json!({
                "src_ip": src_ip.unwrap_or_default(),
                "dst_ip": dst_ip.unwrap_or_default(),
                "src_port": src_port,
                "dst_port": dst_port,
                "protocol": protocol
            }),
        },
        AclCommands::Deny { src_ip, dst_ip, src_port, dst_port, protocol } => Request {
            cmd: "acl_deny".to_string(),
            args: serde_json::json!({
                "src_ip": src_ip.unwrap_or_default(),
                "dst_ip": dst_ip.unwrap_or_default(),
                "src_port": src_port,
                "dst_port": dst_port,
                "protocol": protocol
            }),
        },
        AclCommands::RemoveRule { src_ip, dst_ip, src_port, dst_port, protocol } => Request {
            cmd: "acl_remove_rule".to_string(),
            args: serde_json::json!({
                "src_ip": src_ip.unwrap_or_default(),
                "dst_ip": dst_ip.unwrap_or_default(),
                "src_port": src_port,
                "dst_port": dst_port,
                "protocol": protocol
            }),
        },
        AclCommands::Stats { src_ip, dst_ip, src_port, dst_port, protocol } => Request {
            cmd: "acl_stats".to_string(),
            args: serde_json::json!({
                "src_ip": src_ip.unwrap_or_default(),
                "dst_ip": dst_ip.unwrap_or_default(),
                "src_port": src_port,
                "dst_port": dst_port,
                "protocol": protocol
            }),
        },
    }
}

fn send_log_command(level: &str) -> Result<()> {
    let stream = UnixStream::connect(SOCKET_PATH)?;
    let mut writer = BufWriter::new(&stream);
    let mut reader = BufReader::new(&stream);

    let req = Request {
        cmd: "set_log_level".to_string(),
        args: serde_json::json!({"level": level}),
    };

    let req_json = serde_json::to_string(&req)?;
    writer.write_all(req_json.as_bytes())?;
    writer.write_all(b"\n")?;
    writer.flush()?;

    let mut resp_line = String::new();
    reader.read_line(&mut resp_line)?;
    let resp: Response = serde_json::from_str(&resp_line)?;
    
    if resp.success {
        if let Some(msg) = resp.message {
            println!("{}", msg);
        }
    } else {
        eprintln!("Error: {}", resp.message.unwrap_or_else(|| "Unknown error".to_string()));
        std::process::exit(1);
    }

    Ok(())
}

fn run_init(
    server: &str,
    token: &str,
    public_ip: Option<&str>,
    public_endpoint: Option<&str>,
    hostname: Option<&str>,
    region: Option<&str>,
    interface: Option<&str>,
    ca_cert: Option<&str>,
    client_cert: Option<&str>,
    client_key: Option<&str>,
    controller_api_url: Option<&str>,
    tls_server_name: Option<&str>,
    config_path: Option<&str>,
    advertise_routes: Option<&str>,
) -> Result<()> {
    println!("Initializing Aria Agent...");
    println!("  Server: {}", server);
    println!("  Token: {}...", &token[..16.min(token.len())]);
    
    let iface = interface.unwrap_or("aria0");
    let hostname = hostname
        .map(|value| value.to_string())
        .or_else(detect_hostname)
        .unwrap_or_else(|| "aria-agent".to_string());
    let device_id = detect_machine_id().unwrap_or_else(|| generate_fallback_device_id(&hostname));
    let (private_key, public_key) = wireguard::WireGuardManager::generate_keypair()
        .context("Failed to generate WireGuard keypair")?;

    let bootstrap = config::BootstrapConfig {
        controller_url: server.to_string(),
        controller_api_url: controller_api_url.unwrap_or_default().to_string(),
        ca_cert: ca_cert.unwrap_or_default().to_string(),
        client_cert: client_cert.unwrap_or_default().to_string(),
        client_key: client_key.unwrap_or_default().to_string(),
        tls_server_name: tls_server_name.map(|value| value.to_string()),
        enrollment_token: Some(token.to_string()),
        public_ip: public_ip.and_then(normalize_public_ipv4),
        public_endpoint: public_endpoint
            .and_then(|value| normalize_public_endpoint(Some(value), public_ip, 51820)),
        interface_name: iface.to_string(),
        listen_port: 51820,
        mtu: 1360,
        region: region.map(|s| s.to_string()),
        customer_id: None,
        advertised_routes: parse_advertised_routes(advertise_routes),
        hostname: Some(hostname),
        sync_interval: Duration::from_secs(5),
        certificate_renew_before: Duration::from_secs(72 * 60 * 60),
        multi_tunnel: true,
    };
    let state = config::AgentState {
        device_id: Some(device_id),
        private_key,
        public_key,
        ..Default::default()
    };

    let config_path = config_path.unwrap_or(DEFAULT_CONFIG_PATH);
    let config_manager = config::ConfigManager::new(config_path);
    config_manager.save_bootstrap(&bootstrap)?;
    config_manager.save_state(&state)?;
    
    println!("Bootstrap config saved to {}", config_manager.bootstrap_path());
    println!("Runtime state saved to {}", config_manager.state_path());
    println!("Run 'aria-agent up' to start the agent.");
    
    Ok(())
}

fn parse_advertised_routes(advertise_routes: Option<&str>) -> Option<Vec<String>> {
    advertise_routes.and_then(|routes| {
        let routes = routes
            .split(',')
            .map(|route| route.trim())
            .filter(|route| !route.is_empty())
            .map(|route| route.to_string())
            .collect::<Vec<_>>();
        if routes.is_empty() {
            None
        } else {
            Some(routes)
        }
    })
}

fn detect_hostname() -> Option<String> {
    if let Ok(hostname) = std::env::var("HOSTNAME") {
        let hostname = hostname.trim();
        if !hostname.is_empty() {
            return Some(hostname.to_string());
        }
    }

    for path in ["/etc/hostname", "/proc/sys/kernel/hostname"] {
        if let Ok(hostname) = std::fs::read_to_string(path) {
            let hostname = hostname.trim();
            if !hostname.is_empty() {
                return Some(hostname.to_string());
            }
        }
    }

    if let Ok(output) = std::process::Command::new("hostname").output() {
        if output.status.success() {
            let hostname = String::from_utf8_lossy(&output.stdout).trim().to_string();
            if !hostname.is_empty() {
                return Some(hostname);
            }
        }
    }

    None
}

fn detect_machine_id() -> Option<String> {
    for path in ["/etc/machine-id", "/var/lib/dbus/machine-id"] {
        if let Ok(machine_id) = std::fs::read_to_string(path) {
            let machine_id = machine_id.trim();
            if !machine_id.is_empty() {
                return Some(machine_id.to_string());
            }
        }
    }

    None
}

fn generate_fallback_device_id(hostname: &str) -> String {
    let ts = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();
    format!("{}-{}", hostname.replace(' ', "-"), ts)
}

fn normalize_public_ipv4(candidate: &str) -> Option<String> {
    let candidate = candidate
        .trim()
        .lines()
        .next()
        .unwrap_or_default()
        .trim()
        .trim_matches(|ch| ch == '"' || ch == char::from(39));
    match candidate.parse::<IpAddr>().ok()? {
        IpAddr::V4(ip) if is_public_ipv4(ip) => Some(ip.to_string()),
        _ => None,
    }
}

fn is_public_ipv4(ip: Ipv4Addr) -> bool {
    let octets = ip.octets();
    if ip.is_unspecified()
        || ip.is_loopback()
        || ip.is_private()
        || ip.is_link_local()
        || ip.is_multicast()
        || ip.is_broadcast()
    {
        return false;
    }

    match octets {
        [100, second, _, _] if (64..=127).contains(&second) => false, // CGNAT
        [0, _, _, _] => false,
        [169, 254, _, _] => false,
        [192, 0, 0, _] => false,
        [192, 0, 2, _] => false,
        [198, second, _, _] if second == 18 || second == 19 => false,
        [198, 51, 100, _] => false,
        [203, 0, 113, _] => false,
        [first, _, _, _] if first >= 224 => false,
        _ => true,
    }
}

fn detect_route_source_public_ipv4() -> Option<String> {
    let socket = UdpSocket::bind("0.0.0.0:0").ok()?;
    socket.connect("8.8.8.8:80").ok()?;
    let local_addr = socket.local_addr().ok()?;
    match local_addr.ip() {
        IpAddr::V4(ip) if is_public_ipv4(ip) => Some(ip.to_string()),
        _ => None,
    }
}

fn normalize_public_endpoint(
    configured_endpoint: Option<&str>,
    public_ip: Option<&str>,
    listen_port: u16,
) -> Option<String> {
    if let Some(endpoint) = configured_endpoint.map(str::trim).filter(|value| !value.is_empty()) {
        let (host, port) = split_endpoint_host_port(endpoint);
        if host.is_empty() {
            return public_ip
                .and_then(normalize_public_ipv4)
                .map(|ip| format!("{}:{}", ip, port.unwrap_or_else(|| listen_port.to_string())));
        }
        if let Ok(IpAddr::V4(ip)) = host.parse::<IpAddr>() {
            if is_public_ipv4(ip) {
                return Some(endpoint.to_string());
            }
            return public_ip
                .and_then(normalize_public_ipv4)
                .map(|ip| format!("{}:{}", ip, port.unwrap_or_else(|| listen_port.to_string())));
        }
        if host.eq_ignore_ascii_case("localhost") {
            return None;
        }
        return Some(endpoint.to_string());
    }

    public_ip
        .and_then(normalize_public_ipv4)
        .map(|ip| format!("{}:{}", ip, listen_port))
}

fn split_endpoint_host_port(endpoint: &str) -> (String, Option<String>) {
    let endpoint = endpoint.trim();
    if let Some(port) = endpoint.strip_prefix(':') {
        return ("".to_string(), Some(port.to_string()));
    }
    if let Some(rest) = endpoint.strip_prefix('[') {
        if let Some((host, tail)) = rest.split_once(']') {
            let port = tail.strip_prefix(':').map(|value| value.to_string());
            return (host.to_string(), port);
        }
    }
    match endpoint.rsplit_once(':') {
        Some((host, port)) => (host.to_string(), Some(port.to_string())),
        None => (endpoint.to_string(), None),
    }
}

async fn detect_public_ipv4(configured: Option<&str>) -> Option<String> {
    if let Some(ip) = configured.and_then(normalize_public_ipv4) {
        return Some(ip);
    }

    let client = match reqwest::Client::builder()
        .timeout(Duration::from_secs(1))
        .build()
    {
        Ok(client) => client,
        Err(_) => return detect_route_source_public_ipv4(),
    };
    let probes: &[(&str, &[(&str, &str)])] = &[
        ("http://metadata.tencentyun.com/latest/meta-data/public-ipv4", &[]),
        ("http://100.100.100.200/latest/meta-data/eipv4", &[]),
        ("http://100.100.100.200/latest/meta-data/public-ipv4", &[]),
        ("http://169.254.169.254/latest/meta-data/public-ipv4", &[]),
        (
            "http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip",
            &[("Metadata-Flavor", "Google")],
        ),
        ("https://checkip.amazonaws.com", &[]),
        ("https://api.ipify.org", &[]),
        ("https://ifconfig.me/ip", &[]),
    ];

    for (url, headers) in probes {
        let mut request = client.get(*url);
        for (key, value) in *headers {
            request = request.header(*key, *value);
        }
        if let Ok(response) = request.send().await {
            if response.status().is_success() {
                if let Ok(body) = response.text().await {
                    if let Some(ip) = normalize_public_ipv4(&body) {
                        return Some(ip);
                    }
                }
            }
        }
    }

    detect_route_source_public_ipv4()
}

async fn ensure_public_network_identity(bootstrap: &mut config::BootstrapConfig) -> bool {
    let mut changed = false;
    let detected_public_ip = detect_public_ipv4(bootstrap.public_ip.as_deref()).await;
    if bootstrap.public_ip != detected_public_ip {
        bootstrap.public_ip = detected_public_ip.clone();
        changed = true;
    }

    let endpoint = normalize_public_endpoint(
        bootstrap.public_endpoint.as_deref(),
        detected_public_ip.as_deref(),
        bootstrap.listen_port,
    );
    if bootstrap.public_endpoint != endpoint {
        bootstrap.public_endpoint = endpoint;
        changed = true;
    }

    changed
}

fn ensure_local_identity(
    bootstrap: &mut config::BootstrapConfig,
    state: &mut config::AgentState,
) -> Result<(bool, bool)> {
    let mut bootstrap_changed = false;
    let mut state_changed = false;

    if state.private_key.trim().is_empty() {
        let (private_key, public_key) = wireguard::WireGuardManager::generate_keypair()
            .context("Failed to generate missing WireGuard keypair")?;
        state.private_key = private_key;
        state.public_key = public_key;
        state_changed = true;
    } else {
        let derived_public_key = wireguard::WireGuardManager::derive_public_key(&state.private_key)
            .context("Failed to derive public key from private key")?;
        if state.public_key != derived_public_key {
            state.public_key = derived_public_key;
            state_changed = true;
        }
    }

    if bootstrap.hostname.as_deref().map(|value| value.trim().is_empty()).unwrap_or(true) {
        bootstrap.hostname = detect_hostname().or_else(|| Some("aria-agent".to_string()));
        bootstrap_changed = true;
    }

    if state.device_id.as_deref().map(|value| value.trim().is_empty()).unwrap_or(true) {
        let hostname = bootstrap.hostname.as_deref().unwrap_or("aria-agent");
        state.device_id = Some(
            detect_machine_id().unwrap_or_else(|| generate_fallback_device_id(hostname)),
        );
        state_changed = true;
    }

    if state.address.is_none() {
        if let Some(assigned_ip) = state.assigned_ip.as_deref() {
            if !assigned_ip.trim().is_empty() {
                state.address = Some(format!("{}/32", assigned_ip));
                state_changed = true;
            }
        }
    }

    Ok((bootstrap_changed, state_changed))
}

fn current_epoch_seconds() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs() as i64
}

fn needs_bootstrap_registration(state: &config::AgentState, now: i64) -> bool {
    if state.assigned_ip.as_deref().map(|value| value.trim().is_empty()).unwrap_or(true) {
        return true;
    }
    if state.current_credential.as_deref().map(|value| value.trim().is_empty()).unwrap_or(true) {
        return true;
    }
    match state.current_credential_expires_at {
        Some(expires_at) => expires_at <= now + 60,
        None => false,
    }
}

async fn bootstrap_register(
    bootstrap: &config::BootstrapConfig,
    state: &mut config::AgentState,
) -> Result<bool> {
    if !needs_bootstrap_registration(state, current_epoch_seconds()) {
        return Ok(false);
    }

    let token = bootstrap
        .enrollment_token
        .clone()
        .filter(|value| !value.trim().is_empty())
        .context("enrollment_token is required for first-time registration")?;

    let hostname = bootstrap
        .hostname
        .clone()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| "aria-agent".to_string());
    let machine_id = state
        .device_id
        .clone()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| generate_fallback_device_id(&hostname));
    let public_ip = bootstrap
        .public_ip
        .as_deref()
        .and_then(normalize_public_ipv4)
        .unwrap_or_default();
    let endpoint = normalize_public_endpoint(
        bootstrap.public_endpoint.as_deref(),
        (!public_ip.is_empty()).then_some(public_ip.as_str()),
        bootstrap.listen_port,
    )
    .unwrap_or_default();
    if public_ip.is_empty() {
        tracing::warn!("Unable to detect public IPv4 address, registering without public_ip");
    }

    let grpc_client = grpc_client::GrpcClient::new_with_options(
        bootstrap.controller_url.clone(),
        bootstrap.ca_cert.clone(),
        bootstrap.client_cert.clone(),
        bootstrap.client_key.clone(),
        bootstrap.tls_server_name.clone(),
    )
    .await
    .context("Failed to connect to Controller for bootstrap registration")?;

    let registration = grpc_client
        .register_with_details(
            state.public_key.clone(),
            endpoint,
            public_ip,
            hostname,
            token,
            bootstrap.region.clone().unwrap_or_else(|| "default".to_string()),
            machine_id.clone(),
            bootstrap.advertised_routes.clone().unwrap_or_default(),
        )
        .await
        .context("Failed to register agent with Controller")?;

    state.device_id = Some(machine_id);
    if let Some(node_id) = registration.node_id {
        state.node_id = Some(node_id);
    }
    state.assigned_ip = Some(registration.assigned_ip.clone());
    state.address = Some(format!("{}/32", registration.assigned_ip));
    if let Some(runtime_token) = registration.runtime_token {
        state.current_credential = Some(runtime_token);
    }
    state.current_credential_expires_at = registration.runtime_token_expires_at;
    state.last_desired_version = None;
    state.last_applied_version = None;
    state.last_sync_status = Some("registered".to_string());
    state.last_sync_message = Some("bootstrap registration completed".to_string());
    state.last_sync_at = Some(
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs() as i64,
    );
    Ok(true)
}

fn send_status_command() -> Result<()> {
    let stream = UnixStream::connect(SOCKET_PATH)?;
    let mut writer = BufWriter::new(&stream);
    let mut reader = BufReader::new(&stream);

    let req = Request {
        cmd: "get_status".to_string(),
        args: serde_json::json!({}),
    };

    let req_json = serde_json::to_string(&req)?;
    writer.write_all(req_json.as_bytes())?;
    writer.write_all(b"\n")?;
    writer.flush()?;

    let mut resp_line = String::new();
    reader.read_line(&mut resp_line)?;
    let resp: Response = serde_json::from_str(&resp_line)?;
    
    if resp.success {
        if let Some(data) = resp.data {
            println!("{}", serde_json::to_string_pretty(&data)?);
        }
    } else {
        eprintln!("Error: {}", resp.message.unwrap_or_else(|| "Unknown error".to_string()));
        std::process::exit(1);
    }

    Ok(())
}

fn send_peers_command() -> Result<()> {
    let stream = UnixStream::connect(SOCKET_PATH)?;
    let mut writer = BufWriter::new(&stream);
    let mut reader = BufReader::new(&stream);

    let req = Request {
        cmd: "get_peers".to_string(),
        args: serde_json::json!({}),
    };

    let req_json = serde_json::to_string(&req)?;
    writer.write_all(req_json.as_bytes())?;
    writer.write_all(b"\n")?;
    writer.flush()?;

    let mut resp_line = String::new();
    reader.read_line(&mut resp_line)?;
    let resp: Response = serde_json::from_str(&resp_line)?;
    
    if resp.success {
        if let Some(data) = resp.data {
            let peers = data["peers"].as_array().ok_or_else(|| anyhow::anyhow!("Invalid peers data"))?;
            let total = data["total"].as_u64().unwrap_or(0);
            
            println!("Total peers: {}", total);
            println!();
            
            // 表头
            println!("{:<8} {:<20} {:<25} {:<15} {:<12}", "Region", "Public Key", "Endpoint", "VPN IP", "Handshake");
            println!("{:-<8} {:-<20} {:-<25} {:-<15} {:-<12}", "", "", "", "", "");
            
            for peer in peers {
                let region = peer["region"].as_str().unwrap_or("unknown");
                let public_key = peer["public_key"].as_str().unwrap_or("unknown");
                let public_key_display = if public_key.len() > 20 {
                    format!("{}...{}", &public_key[..8], &public_key[public_key.len()-8..])
                } else {
                    public_key.to_string()
                };
                let endpoint = peer["endpoint"].as_str().unwrap_or("N/A");
                
                // 从 allowed_ips 中提取 VPN IP（/32 地址）
                let vpn_ip = peer["allowed_ips"].as_array()
                    .and_then(|ips| ips.iter().find(|ip| ip.as_str().map(|s| s.contains("/32")).unwrap_or(false)))
                    .and_then(|ip| ip.as_str())
                    .map(|ip| ip.replace("/32", ""))
                    .unwrap_or_else(|| "N/A".to_string());
                
                let last_handshake = peer["last_handshake_secs"].as_u64().unwrap_or(0);
                let handshake_display = format!("{}s", last_handshake);
                
                println!("{:<8} {:<20} {:<25} {:<15} {:<12}", 
                    region, public_key_display, endpoint, vpn_ip, handshake_display);
            }
            println!();
        }
    } else {
        eprintln!("Error: {}", resp.message.unwrap_or_else(|| "Unknown error".to_string()));
        std::process::exit(1);
    }

    Ok(())
}

fn send_route_command() -> Result<()> {
    let stream = UnixStream::connect(SOCKET_PATH)?;
    let mut writer = BufWriter::new(&stream);
    let mut reader = BufReader::new(&stream);

    let req = Request {
        cmd: "get_routes".to_string(),
        args: serde_json::json!({}),
    };

    let req_json = serde_json::to_string(&req)?;
    writer.write_all(req_json.as_bytes())?;
    writer.write_all(b"\n")?;
    writer.flush()?;

    let mut resp_line = String::new();
    reader.read_line(&mut resp_line)?;
    let resp: Response = serde_json::from_str(&resp_line)?;
    
    if resp.success {
        if let Some(data) = resp.data {
            let routes = data["routes"].as_array().ok_or_else(|| anyhow::anyhow!("Invalid routes data"))?;
            let total = data["total"].as_u64().unwrap_or(0);
            let table = data["table"].as_u64().unwrap_or(100);
            
            println!("VPN Routes (table {})", table);
            println!("Total: {} routes", total);
            println!();
            
            // 表头
            println!("{:<20} {:<60}", "Destination", "Nexthops");
            println!("{:-<20} {:-<60}", "", "");
            
            // 合并ECMP路由
            let mut current_dest = String::new();
            let mut nexthops = Vec::new();
            
            for route in routes {
                let route_str = route.as_str().unwrap_or("unknown");
                
                if route_str.contains("nexthop") {
                    // 这是nexthop行，提取信息
                    let parts: Vec<&str> = route_str.split_whitespace().collect();
                    if let Some(idx) = parts.iter().position(|&x| x == "dev") {
                        if idx + 1 < parts.len() {
                            nexthops.push(parts[idx + 1].to_string());
                        }
                    }
                } else {
                    // 这是目标地址行
                    if !current_dest.is_empty() && !nexthops.is_empty() {
                        // 输出前一个路由
                        let nexthop_str = nexthops.join(", ");
                        println!("{:<20} {:<60}", current_dest, nexthop_str);
                    }
                    current_dest = route_str.trim().to_string();
                    nexthops.clear();
                }
            }
            
            // 输出最后一个路由
            if !current_dest.is_empty() && !nexthops.is_empty() {
                let nexthop_str = nexthops.join(", ");
                println!("{:<20} {:<60}", current_dest, nexthop_str);
            }
            
            println!();
        }
    } else {
        eprintln!("Error: {}", resp.message.unwrap_or_else(|| "Unknown error".to_string()));
        std::process::exit(1);
    }

    Ok(())
}

fn send_down_command() -> Result<()> {
    println!("Stopping agent daemon...");
    println!("Use: systemctl stop aria-agent");
    Ok(())
}

// ==================== UnifiedAgent 启动函数 ====================

async fn run_unified_agent(
    interface: &str,
    log_level: &str,
    metrics_addr: &str,
    config_path: Option<&str>,
) -> Result<()> {
    let log_handle = init_logging(log_level)?;
    info!("Starting Aria Agent v{}", env!("CARGO_PKG_VERSION"));
    
    metrics::init_metrics(metrics_addr)
        .context("Failed to initialize metrics")?;
    info!("Metrics server started on {}", metrics_addr);
    
    let config_path = config_path.unwrap_or(DEFAULT_CONFIG_PATH);
    let config_manager = config::ConfigManager::new(config_path);
    let (mut bootstrap, mut state) = config_manager
        .load_parts_or_init(false)?
        .context("Agent is not initialized. Run 'aria-agent init' first.")?;

    let (mut bootstrap_changed, mut state_changed) = ensure_local_identity(&mut bootstrap, &mut state)?;
    if ensure_public_network_identity(&mut bootstrap).await {
        bootstrap_changed = true;
    }

    if needs_bootstrap_registration(&state, current_epoch_seconds()) {
        info!("Runtime registration state missing or expired, starting bootstrap registration");
        if bootstrap_register(&bootstrap, &mut state).await? {
            state_changed = true;
        }
    }

    if bootstrap_changed {
        config_manager.save_bootstrap(&bootstrap)?;
        info!("Bootstrap config saved to {}", config_manager.bootstrap_path());
    }

    if state_changed {
        config_manager.save_state(&state)?;
        info!("Runtime state saved to {}", config_manager.state_path());
    }

    let config = config::AgentConfig::from_parts(bootstrap, state);
    let mut agent = unified_agent::UnifiedAgent::new(
        config,
        config_path.to_string(),
        interface,
        log_handle,
    ).await?;
    agent.start().await
}

// ==================== 辅助函数 ====================

// Unix Socket 服务器已集成到 unified_agent.rs 中的 UnifiedAgent
// 使用 UnifiedAgent::start_unix_socket_server() 启动

#[cfg(test)]
mod tests {
    use super::{
        acl_command_request, is_public_ipv4, needs_bootstrap_registration,
        normalize_public_endpoint, normalize_public_ipv4, AclCommands,
    };
    use crate::config::AgentState;
    use serde_json::json;
    use std::net::Ipv4Addr;

    #[test]
    fn bootstrap_needed_when_assigned_ip_exists_but_runtime_credential_missing() {
        let state = AgentState {
            assigned_ip: Some("100.64.0.2".to_string()),
            current_credential: None,
            ..Default::default()
        };

        assert!(needs_bootstrap_registration(&state, 1_700_000_000));
    }

    #[test]
    fn bootstrap_needed_when_runtime_credential_is_expired() {
        let state = AgentState {
            assigned_ip: Some("100.64.0.2".to_string()),
            current_credential: Some("rt.old".to_string()),
            current_credential_expires_at: Some(1_699_999_000),
            ..Default::default()
        };

        assert!(needs_bootstrap_registration(&state, 1_700_000_000));
    }

    #[test]
    fn bootstrap_not_needed_when_runtime_credential_has_no_expiry_metadata() {
        let state = AgentState {
            assigned_ip: Some("100.64.0.2".to_string()),
            current_credential: Some("rt.unknown-expiry".to_string()),
            current_credential_expires_at: None,
            ..Default::default()
        };

        assert!(!needs_bootstrap_registration(&state, 1_700_000_000));
    }

    #[test]
    fn acl_cli_block_src_ip_uses_handler_key() {
        let req = acl_command_request(AclCommands::BlockSrcIp {
            ip: "10.0.0.10".to_string(),
        });

        assert_eq!(req.cmd, "acl_block_src_ip");
        assert_eq!(req.args, json!({"src_ip": "10.0.0.10"}));
    }

    #[test]
    fn acl_cli_block_dst_ip_uses_handler_key() {
        let req = acl_command_request(AclCommands::BlockDstIp {
            ip: "10.0.0.20".to_string(),
        });

        assert_eq!(req.cmd, "acl_block_dst_ip");
        assert_eq!(req.args, json!({"dst_ip": "10.0.0.20"}));
    }

    #[test]
    fn public_ipv4_filter_rejects_private_and_vpn_ranges() {
        for value in [
            "10.2.0.3",
            "172.16.0.1",
            "192.168.1.1",
            "100.64.0.2",
            "127.0.0.1",
            "169.254.1.1",
        ] {
            assert_eq!(normalize_public_ipv4(value), None);
        }
        assert!(!is_public_ipv4(Ipv4Addr::new(10, 2, 0, 3)));
    }

    #[test]
    fn public_endpoint_replaces_private_host_with_public_ip() {
        assert_eq!(
            normalize_public_endpoint(Some("10.2.0.3:51820"), Some("82.156.48.111"), 51820),
            Some("82.156.48.111:51820".to_string())
        );
        assert_eq!(
            normalize_public_endpoint(None, Some("82.156.48.111"), 51820),
            Some("82.156.48.111:51820".to_string())
        );
    }
}
