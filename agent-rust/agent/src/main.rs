use std::os::unix::net::{UnixListener, UnixStream};
use std::io::{BufReader, BufWriter, BufRead, Write};
use std::path::Path;
use std::sync::{Arc, Mutex};
use std::sync::atomic::{AtomicBool, Ordering};

use anyhow::{Context, Result};
use aya::{
    include_bytes_aligned,
    programs::{tc, SchedClassifier, TcAttachType, Xdp, XdpFlags},
    Ebpf,
    EbpfLoader,
};
use clap::{Parser, Subcommand};
use serde::{Deserialize, Serialize};
use tracing::{info, error, warn};
use tracing_subscriber::{
    layer::SubscriberExt,
    util::SubscriberInitExt,
    EnvFilter,
    Registry,
    reload,
    fmt,
};

mod acl;
mod qos;
mod identity;
mod config;
mod metrics;
mod unified_agent;
mod grpc_client;
mod wireguard;
mod routing;
mod system_optimization;

const SOCKET_PATH: &str = "/run/aria-agent.sock";
const BPF_FS_PATH: &str = "/sys/fs/bpf/aria";

static RUNNING: AtomicBool = AtomicBool::new(true);

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
        region: Option<String>,
        #[arg(long)]
        interface: Option<String>,
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
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("Failed to install rustls provider");
    
    let cli = Cli::parse();

    match cli.command {
        Commands::Up { interface, log_level, metrics_addr, config } => {
            let rt = tokio::runtime::Runtime::new()
                .context("Failed to create tokio runtime")?;
            rt.block_on(run_unified_agent(&interface, &log_level, &metrics_addr, config.as_deref()))?;
        }
        Commands::Init { server, token, region, interface, advertise_routes } => {
            run_init(&server, &token, region.as_deref(), interface.as_deref(), advertise_routes.as_deref())?
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

    let req = match cmd {
        AclCommands::BlockSrcIp { ip } => Request {
            cmd: "acl_block_src_ip".to_string(),
            args: serde_json::json!({"ip": ip}),
        },
        AclCommands::UnblockSrcIp { ip } => Request {
            cmd: "acl_unblock_src_ip".to_string(),
            args: serde_json::json!({"ip": ip}),
        },
        AclCommands::BlockDstIp { ip } => Request {
            cmd: "acl_block_dst_ip".to_string(),
            args: serde_json::json!({"ip": ip}),
        },
        AclCommands::UnblockDstIp { ip } => Request {
            cmd: "acl_unblock_dst_ip".to_string(),
            args: serde_json::json!({"ip": ip}),
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

#[derive(Serialize)]
struct PeerInfo {
    public_key: String,
    endpoint: Option<String>,
    allowed_ips: Vec<String>,
    last_handshake: Option<u64>,
    rx_bytes: u64,
    tx_bytes: u64,
    status: String,
}

fn get_wireguard_peers() -> Result<Vec<PeerInfo>> {
    let output = std::process::Command::new("wg")
        .args(&["show", "all", "dump"])
        .output()
        .context("Failed to execute wg show")?;
    
    if !output.status.success() {
        return Err(anyhow::anyhow!("wg show failed: {}", String::from_utf8_lossy(&output.stderr)));
    }

    let lines: Vec<&str> = std::str::from_utf8(&output.stdout)?
        .lines()
        .collect();
    
    let mut peers = Vec::new();
    
    for line in lines.iter().skip(1) {
        let fields: Vec<&str> = line.split('\t').collect();
        if fields.len() < 8 {
            continue;
        }

        let public_key = fields[0].to_string();
        let endpoint = if fields[2].is_empty() { None } else { Some(fields[2].to_string()) };
        let allowed_ips: Vec<String> = fields[3].split(',').map(|s| s.to_string()).collect();
        let last_handshake: Option<u64> = fields[4].parse().ok();
        let rx_bytes: u64 = fields[5].parse().unwrap_or(0);
        let tx_bytes: u64 = fields[6].parse().unwrap_or(0);

        let status = match last_handshake {
            Some(hs) if hs > 0 => {
                let elapsed = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)?
                    .as_secs() - hs;
                if elapsed < 180 { "online".to_string() } else { "offline".to_string() }
            },
            _ => "never".to_string(),
        };

        peers.push(PeerInfo {
            public_key,
            endpoint,
            allowed_ips,
            last_handshake,
            rx_bytes,
            tx_bytes,
            status,
        });
    }

    Ok(peers)
}

#[derive(Serialize)]
struct RouteInfo {
    destination: String,
    interface: String,
    gateway: Option<String>,
    metric: Option<u32>,
}

fn get_routing_table() -> Result<Vec<RouteInfo>> {
    let output = std::process::Command::new("ip")
        .args(&["route", "show", "table", "100"])
        .output()
        .context("Failed to execute ip route")?;
    
    if !output.status.success() {
        return Err(anyhow::anyhow!("ip route failed: {}", String::from_utf8_lossy(&output.stderr)));
    }

    let routes_str = std::str::from_utf8(&output.stdout)?;
    let mut routes = Vec::new();

    for line in routes_str.lines() {
        if line.is_empty() {
            continue;
        }

        let parts: Vec<&str> = line.split_whitespace().collect();
        if let Some(dest) = parts.first() {
            routes.push(RouteInfo {
                destination: dest.to_string(),
                interface: "aria0".to_string(),
                gateway: None,
                metric: None,
            });
        }
    }

    Ok(routes)
}

#[derive(Serialize)]
struct StatusInfo {
    version: String,
    interface: String,
    status: String,
    peer_count: usize,
}

fn get_agent_status() -> Result<StatusInfo> {
    let peers = get_wireguard_peers()?;
    
    Ok(StatusInfo {
        version: env!("CARGO_PKG_VERSION").to_string(),
        interface: "aria0".to_string(),
        status: "running".to_string(),
        peer_count: peers.len(),
    })
}

fn run_init(server: &str, token: &str, region: Option<&str>, interface: Option<&str>, advertise_routes: Option<&str>) -> Result<()> {
    println!("Initializing Aria Agent...");
    println!("  Server: {}", server);
    println!("  Token: {}...", &token[..16.min(token.len())]);
    
    let iface = interface.unwrap_or("aria0");
    let config = config::AgentConfig {
        controller_url: server.to_string(),
        ca_cert: String::new(),
        client_cert: String::new(),
        client_key: String::new(),
        device_id: None,
        private_key: String::new(),
        public_key: String::new(),
        assigned_ip: None,
        address: None,
        interface_name: iface.to_string(),
        listen_port: 51820,
        mtu: 1360,
        region: region.map(|s| s.to_string()),
        customer_id: None,
        advertised_routes: advertise_routes.map(|s| s.split(',').map(|r| r.trim().to_string()).collect()),
        hostname: None,
        sync_interval: std::time::Duration::from_secs(5),
        multi_tunnel: true,
    };

    let config_manager = config::ConfigManager::new("/etc/aria/agent.yaml");
    config_manager.save(&config)?;
    
    println!("Config saved to /etc/aria/agent.yaml");
    println!("Run 'aria-agent up' to start the agent.");
    
    Ok(())
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
            println!("{:-<80}", "");
            
            for peer in peers {
                let public_key = peer["public_key"].as_str().unwrap_or("unknown");
                let endpoint = peer["endpoint"].as_str().unwrap_or("N/A");
                let region = peer["region"].as_str().unwrap_or("unknown");
                let allowed_ips = peer["allowed_ips"].as_array()
                    .map(|ips| ips.iter().filter_map(|ip| ip.as_str()).collect::<Vec<_>>().join(", "))
                    .unwrap_or_else(|| "N/A".to_string());
                let last_handshake = peer["last_handshake_secs"].as_u64().unwrap_or(0);
                
                println!("Region:      {}", region);
                println!("Public Key:  {}...{}", &public_key[..16.min(public_key.len())], 
                    if public_key.len() > 16 { &public_key[public_key.len()-8..] } else { "" });
                println!("Endpoint:    {}", endpoint);
                println!("Allowed IPs: {}", allowed_ips);
                println!("Handshake:   {} seconds ago", last_handshake);
                println!("{:-<80}", "");
            }
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
            
            println!("VPN Routes (table {}):", table);
            println!("Total: {} routes", total);
            println!("{:-<80}", "");
            
            for route in routes {
                let route_str = route.as_str().unwrap_or("unknown");
                // 检查是否是 ECMP 路由（包含 nexthop）
                if route_str.contains("nexthop") {
                    // ECMP 路由格式化
                    let lines: Vec<&str> = route_str.lines().collect();
                    if !lines.is_empty() {
                        println!("{}", lines[0].trim());
                        for line in &lines[1..] {
                            println!("  {}", line.trim());
                        }
                        println!("{:-<80}", "");
                    }
                } else {
                    // 普通路由
                    println!("{}", route_str);
                    println!("{:-<40}", "");
                }
            }
        }
    } else {
        eprintln!("Error: {}", resp.message.unwrap_or_else(|| "Unknown error".to_string()));
        std::process::exit(1);
    }

    Ok(())
}

fn send_up_command() -> Result<()> {
    println!("Starting agent daemon...");
    println!("Use: systemctl start aria-agent");
    Ok(())
}

fn send_down_command() -> Result<()> {
    println!("Stopping agent daemon...");
    println!("Use: systemctl stop aria-agent");
    Ok(())
}

fn attach_xdp(ebpf: &mut Ebpf, interface: &str) -> Result<()> {
    let program: &mut Xdp = ebpf.program_mut("xdp_ingress_acl")
        .context("XDP program not found")?
        .try_into()?;
    
    program.load()?;
    program.attach(interface, XdpFlags::default())?;
    
    info!("XDP program attached to {}", interface);
    Ok(())
}

fn attach_tc(ebpf: &mut Ebpf, interface: &str) -> Result<()> {
    let program: &mut SchedClassifier = ebpf.program_mut("tc_egress_qos")
        .context("TC program not found")?
        .try_into()?;
    
    program.load()?;
    
    tc::qdisc_add_clsact(interface)?;
    program.attach(interface, TcAttachType::Egress)?;
    
    info!("TC program attached to {} (egress)", interface);
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
    
    let config = if let Some(path) = config_path {
        let config_manager = config::ConfigManager::new(path);
        config_manager.load()?
    } else {
        let config_manager = config::ConfigManager::new("/etc/aria/agent.yaml");
        config_manager.load_or_init(false)?.unwrap_or_default()
    };
    
    let mut agent = unified_agent::UnifiedAgent::new(config, interface, log_handle).await?;
    agent.start().await
}

// ==================== 辅助函数 ====================

// Unix Socket 服务器已集成到 unified_agent.rs 中的 UnifiedAgent
// 使用 UnifiedAgent::start_unix_socket_server() 启动

