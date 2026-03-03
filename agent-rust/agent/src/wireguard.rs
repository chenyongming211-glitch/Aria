use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use thiserror::Error;
use wireguard_uapi::{DeviceInterface, WgSocket};

const DEFAULT_LISTEN_PORT: u16 = 51820;
const DEFAULT_MTU: u32 = 1360;

#[derive(Error, Debug)]
pub enum WireGuardError {
    #[error("Failed to create interface: {0}")]
    CreateInterface(String),
    #[error("Failed to configure interface: {0}")]
    ConfigureInterface(String),
    #[error("Invalid key: {0}")]
    InvalidKey(String),
    #[error("Netlink error: {0}")]
    NetlinkError(String),
    #[error("Interface not found: {0}")]
    InterfaceNotFound(String),
    #[error("Peer not found: {0}")]
    PeerNotFound(String),
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct InterfaceConfig {
    pub name: String,
    pub private_key: String,
    pub listen_port: u16,
    pub mtu: u32,
    pub address: Option<String>,
}

impl Default for InterfaceConfig {
    fn default() -> Self {
        Self {
            name: "aria0".to_string(),
            private_key: String::new(),
            listen_port: DEFAULT_LISTEN_PORT,
            mtu: DEFAULT_MTU,
            address: None,
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PeerConfig {
    pub public_key: String,
    pub endpoint: Option<String>,
    pub allowed_ips: Vec<String>,
    pub persistent_keepalive: u16,
}

impl Default for PeerConfig {
    fn default() -> Self {
        Self {
            public_key: String::new(),
            endpoint: None,
            allowed_ips: Vec::new(),
            persistent_keepalive: 0,
        }
    }
}

/// Peer 信息
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PeerInfo {
    pub public_key: String,
    pub endpoint: Option<String>,
    pub allowed_ips: Vec<String>,
    /// 自上次握手以来经过的秒数（None 表示从未握手）
    pub last_handshake: Option<u64>,
    pub rx_bytes: u64,
    pub tx_bytes: u64,
    pub persistent_keepalive: u16,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct InterfaceStats {
    pub interface_name: String,
    pub public_key: String,
    pub listen_port: u16,
    pub peers: Vec<PeerInfo>,
}

pub struct WireGuardManager {
    interface_name: String,
    config: Option<InterfaceConfig>,
}

impl WireGuardManager {
    pub fn new(interface_name: &str) -> Self {
        Self {
            interface_name: interface_name.to_string(),
            config: None,
        }
    }

    /// 检查接口是否存在并返回实际名称
    fn check_interface_exists(&self) -> Result<(bool, Option<String>)> {
        let output = std::process::Command::new("ip")
            .args(&["link", "show", &self.interface_name])
            .output()
            .context("Failed to check interface")?;
        
        if output.status.success() {
            Ok((true, Some(self.interface_name.clone())))
        } else {
            Ok((false, None))
        }
    }

    /// 验证接口是否是 WireGuard 类型
    fn is_wireguard_interface(&self, iface_name: &str) -> bool {
        let output = std::process::Command::new("wg")
            .args(&["show", iface_name])
            .output();
        
        match output {
            Ok(output) => output.status.success(),
            Err(_) => false,
        }
    }

    /// 配置已存在的接口（更新私钥和 IP）
    fn configure_interface(&mut self, iface_name: &str, private_key: &str, address: Option<&str>) -> Result<()> {
        tracing::info!("Configuring existing interface {}", iface_name);
        
        // 更新 IP 地址（如果有）
        if let Some(addr) = address {
            // 先删除旧 IP
            let _ = std::process::Command::new("ip")
                .args(&["addr", "flush", "dev", iface_name])
                .output();
            
            // 添加新 IP
            let output = std::process::Command::new("ip")
                .args(&["addr", "add", addr, "dev", iface_name])
                .output()
                .context("Failed to add IP address")?;
            
            if !output.status.success() {
                return Err(WireGuardError::ConfigureInterface(
                    format!("Failed to configure IP: {}", String::from_utf8_lossy(&output.stderr))
                ).into());
            }
        }
        
        // 更新私钥
        let mut child = std::process::Command::new("wg")
            .args(&["set", iface_name, "private-key", "/dev/stdin"])
            .stdin(std::process::Stdio::piped())
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .spawn()
            .context("Failed to spawn wg set")?;
        
        if let Some(mut stdin) = child.stdin.take() {
            use std::io::Write;
            stdin.write_all(private_key.as_bytes())
                .context("Failed to write private key")?;
        }
        
        let output = child.wait_with_output()
            .context("Failed to wait for wg set")?;
        
        if !output.status.success() {
            return Err(WireGuardError::ConfigureInterface(
                format!("Failed to configure WireGuard: {}", String::from_utf8_lossy(&output.stderr))
            ).into());
        }
        
        tracing::info!("Successfully configured interface {}", iface_name);
        Ok(())
    }
    
    /// 检查必要的命令是否存在
    pub fn check_dependencies() -> Result<()> {
        let commands = vec!["ip", "wg"];
        
        for cmd in commands {
            let output = std::process::Command::new("which")
                .arg(cmd)
                .output()
                .context(format!("Failed to check {} command", cmd))?;
            
            if !output.status.success() {
                return Err(anyhow::anyhow!(
                    "Required command '{}' not found. Please install wireguard-tools.",
                    cmd
                ));
            }
        }
        
        Ok(())
    }

    /// 确保 WireGuard 接口存在且配置正确（幂等性）
    /// 无论执行多少次，结果都是一样的：有一个配置正确的接口存在
    pub fn ensure_interface(&mut self, private_key: String, address: Option<String>, listen_port: u16, mtu: u32) -> Result<()> {
        tracing::info!("Ensuring WireGuard interface {}", self.interface_name);
        
        // 1. 检查接口是否已存在
        let (exists, actual_name) = self.check_interface_exists()
            .context("Failed to check interface existence")?;
        
        if exists {
            if let Some(iface_name) = actual_name {
                tracing::info!("Found existing interface {}, verifying...", iface_name);
                
                // 2. 验证接口是否是 WireGuard 类型
                if !self.is_wireguard_interface(&iface_name) {
                    tracing::warn!("Interface {} is not WireGuard type, cleaning up...", iface_name);
                    self.delete_interface()?;
                    // 继续创建新接口
                } else {
                    // 3. 接口存在且类型正确，复用它
                    tracing::info!("Reusing existing WireGuard interface {}", iface_name);
                    self.configure_interface(&iface_name, &private_key, address.as_deref())?;
                    
                    // 更新监听端口
                    tracing::debug!("Updating listen port to {}", listen_port);
                    let output = std::process::Command::new("wg")
                        .args(&["set", &iface_name, "listen-port", &listen_port.to_string()])
                        .output()
                        .context("Failed to execute wg set listen-port")?;
                    
                    if !output.status.success() {
                        let stderr = String::from_utf8_lossy(&output.stderr);
                        tracing::error!("Failed to update listen port: {}", stderr);
                        return Err(WireGuardError::ConfigureInterface(
                            format!("Failed to update listen port: {}", stderr)
                        ).into());
                    }
                    
                    // 更新 MTU
                    tracing::debug!("Updating MTU to {}", mtu);
                    let output = std::process::Command::new("ip")
                        .args(&["link", "set", "dev", &iface_name, "mtu", &mtu.to_string()])
                        .output()
                        .context("Failed to execute ip link set mtu")?;
                    
                    if !output.status.success() {
                        let stderr = String::from_utf8_lossy(&output.stderr);
                        tracing::error!("Failed to update MTU: {}", stderr);
                        return Err(WireGuardError::ConfigureInterface(
                            format!("Failed to update MTU: {}", stderr)
                        ).into());
                    }
                    
                    // 保存配置
                    self.config = Some(InterfaceConfig {
                        name: iface_name.clone(),
                        private_key,
                        listen_port,
                        mtu,
                        address,
                    });
                    
                    tracing::info!("Interface {} updated: port={}, mtu={}", 
                        iface_name, listen_port, mtu);
                    return Ok(());
                }
            }
        }
        
        // 4. 接口不存在或已被删除，创建新接口
        tracing::info!("Creating new WireGuard interface {}", self.interface_name);
        
        let config = InterfaceConfig {
            name: self.interface_name.clone(),
            private_key,
            listen_port,
            mtu,
            address,
        };
        
        self.create_interface(config)?;
        
        Ok(())
    }

    /// 生成新的密钥对
    pub fn generate_keypair() -> Result<(String, String)> {
        use x25519_dalek::{PublicKey, StaticSecret};
        use rand::rngs::OsRng;
        use rand::RngCore;

        let mut bytes = [0u8; 32];
        OsRng.fill_bytes(&mut bytes);
        
        let secret = StaticSecret::from(bytes);
        let public = PublicKey::from(&secret);
        
        let private_key = base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            secret.as_bytes(),
        );
        let public_key = base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            public.as_bytes(),
        );

        Ok((private_key, public_key))
    }

    /// 从私钥派生公钥
    pub fn derive_public_key(private_key: &str) -> Result<String> {
        use x25519_dalek::{PublicKey, StaticSecret};

        let bytes = base64::Engine::decode(
            &base64::engine::general_purpose::STANDARD,
            private_key,
        ).context("Failed to decode private key")?;

        if bytes.len() != 32 {
            return Err(WireGuardError::InvalidKey("Key must be 32 bytes".to_string()).into());
        }

        let mut key_bytes = [0u8; 32];
        key_bytes.copy_from_slice(&bytes);
        let secret = StaticSecret::from(key_bytes);
        let public = PublicKey::from(&secret);

        Ok(base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            public.as_bytes(),
        ))
    }

    /// 创建 WireGuard 接口
    pub fn create_interface(&mut self, config: InterfaceConfig) -> Result<()> {
        tracing::info!("Creating WireGuard interface: {}", self.interface_name);
        
        // 1. 删除已存在的接口
        let _ = std::process::Command::new("ip")
            .args(&["link", "del", &self.interface_name])
            .output();
        
        // 等待接口完全删除（重试机制，最多500ms）
        for _ in 0..10 {
            let output = std::process::Command::new("ip")
                .args(&["link", "show", &self.interface_name])
                .output();
            
            if let Ok(output) = output {
                if !output.status.success() {
                    // 接口已删除
                    break;
                }
            }
            
            std::thread::sleep(std::time::Duration::from_millis(50));
        }
        
        // 2. 创建 WireGuard 接口
        let output = std::process::Command::new("ip")
            .args(&["link", "add", "dev", &self.interface_name, "type", "wireguard"])
            .output()
            .context("Failed to execute ip link add")?;
        
        if !output.status.success() {
            return Err(WireGuardError::CreateInterface(
                String::from_utf8_lossy(&output.stderr).to_string()
            ).into());
        }
        
        // 3. 设置 MTU
        let output = std::process::Command::new("ip")
            .args(&["link", "set", "dev", &self.interface_name, "mtu", &config.mtu.to_string()])
            .output()
            .context("Failed to set MTU")?;
        
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            tracing::error!("Failed to set MTU: {}", stderr);
            
            // 尝试回滚删除接口（最多重试3次）
            let mut cleanup_success = false;
            for attempt in 1..=3 {
                tracing::warn!("Attempt {} to cleanup interface after MTU error", attempt);
                
                let cleanup_output = std::process::Command::new("ip")
                    .args(&["link", "del", &self.interface_name])
                    .output();
                
                match cleanup_output {
                    Ok(cleanup) if cleanup.status.success() => {
                        tracing::info!("Interface cleanup successful");
                        cleanup_success = true;
                        break;
                    }
                    Ok(cleanup) => {
                        tracing::error!("Cleanup attempt {} failed: {}", attempt,
                            String::from_utf8_lossy(&cleanup.stderr));
                    }
                    Err(e) => {
                        tracing::error!("Cleanup attempt {} error: {:?}", attempt, e);
                    }
                }
                
                if attempt < 3 {
                    std::thread::sleep(std::time::Duration::from_millis(100));
                }
            }
            
            if !cleanup_success {
                tracing::error!(
                    "CRITICAL: Failed to cleanup interface {} after MTU error. Manual cleanup required!",
                    self.interface_name
                );
            }
            
            return Err(WireGuardError::ConfigureInterface(
                format!("Failed to set MTU: {}. Cleanup {}", stderr,
                    if cleanup_success { "successful" } else { "failed - manual cleanup required" })
            ).into());
        }
        
        // 4. UP 接口
        let output = std::process::Command::new("ip")
            .args(&["link", "set", "dev", &self.interface_name, "up"])
            .output()
            .context("Failed to set interface up")?;
        
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            tracing::error!("Failed to set interface up: {}", stderr);
            
            // 尝试回滚删除接口（最多重试3次）
            let mut cleanup_success = false;
            for attempt in 1..=3 {
                tracing::warn!("Attempt {} to cleanup interface after UP error", attempt);
                
                let cleanup_output = std::process::Command::new("ip")
                    .args(&["link", "del", &self.interface_name])
                    .output();
                
                match cleanup_output {
                    Ok(cleanup) if cleanup.status.success() => {
                        tracing::info!("Interface cleanup successful");
                        cleanup_success = true;
                        break;
                    }
                    Ok(cleanup) => {
                        tracing::error!("Cleanup attempt {} failed: {}", attempt,
                            String::from_utf8_lossy(&cleanup.stderr));
                    }
                    Err(e) => {
                        tracing::error!("Cleanup attempt {} error: {:?}", attempt, e);
                    }
                }
                
                if attempt < 3 {
                    std::thread::sleep(std::time::Duration::from_millis(100));
                }
            }
            
            if !cleanup_success {
                tracing::error!(
                    "CRITICAL: Failed to cleanup interface {} after UP error. Manual cleanup required!",
                    self.interface_name
                );
            }
            
            return Err(WireGuardError::ConfigureInterface(
                format!("Failed to set interface up: {}. Cleanup {}", stderr,
                    if cleanup_success { "successful" } else { "failed - manual cleanup required" })
            ).into());
        }
        
        // 4.5 配置 IP 地址（如果有）
        if let Some(address) = &config.address {
            let output = std::process::Command::new("ip")
                .args(&["addr", "add", address, "dev", &self.interface_name])
                .output()
                .context("Failed to add IP address")?;
            
            if !output.status.success() {
                let stderr = String::from_utf8_lossy(&output.stderr).to_string();
                tracing::error!("Failed to add IP address {}: {}", address, stderr);
                
                // IP 地址配置失败视为严重错误，回滚接口创建
                tracing::warn!("Rolling back interface creation due to IP address configuration failure");
                
                // 尝试回滚删除接口（最多重试3次）
                let mut cleanup_success = false;
                for attempt in 1..=3 {
                    tracing::warn!("Attempt {} to cleanup interface after IP config error", attempt);
                    
                    let cleanup_output = std::process::Command::new("ip")
                        .args(&["link", "del", &self.interface_name])
                        .output();
                    
                    match cleanup_output {
                        Ok(cleanup) if cleanup.status.success() => {
                            tracing::info!("Interface cleanup successful");
                            cleanup_success = true;
                            break;
                        }
                        Ok(cleanup) => {
                            tracing::error!("Cleanup attempt {} failed: {}", attempt,
                                String::from_utf8_lossy(&cleanup.stderr));
                        }
                        Err(e) => {
                            tracing::error!("Cleanup attempt {} error: {:?}", attempt, e);
                        }
                    }
                    
                    if attempt < 3 {
                        std::thread::sleep(std::time::Duration::from_millis(100));
                    }
                }
                
                if !cleanup_success {
                    tracing::error!(
                        "CRITICAL: Failed to cleanup interface {} after IP config error. Manual cleanup required!",
                        self.interface_name
                    );
                }
                
                return Err(WireGuardError::ConfigureInterface(
                    format!("Failed to configure IP address {}: {}. Cleanup {}", address, stderr,
                        if cleanup_success { "successful" } else { "failed - manual cleanup required" })
                ).into());
            } else {
                tracing::info!("Added IP address {} to interface {}", address, self.interface_name);
            }
        }
        
        // 5. 使用 wg 命令配置私钥和监听端口（通过管道，避免 shell 暴露）
        let private_key = config.private_key.clone();
        let listen_port = config.listen_port.to_string();
        let ifname = self.interface_name.clone();
        
        let mut child = std::process::Command::new("wg")
            .args(&["set", &ifname, "private-key", "/dev/stdin", "listen-port", &listen_port])
            .stdin(std::process::Stdio::piped())
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .spawn()
            .context("Failed to spawn wg set")?;
        
        // 通过管道安全地传递私钥
        if let Some(mut stdin) = child.stdin.take() {
            use std::io::Write;
            stdin.write_all(private_key.as_bytes())
                .context("Failed to write private key to stdin")?;
        }
        
        let output = child.wait_with_output()
            .context("Failed to wait for wg set")?;
        
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            tracing::error!("WireGuard configuration failed: {}", stderr);
            
            // 配置失败时回滚：删除接口
            tracing::warn!("Rolling back interface creation due to wg configuration failure");
            let _ = std::process::Command::new("ip")
                .args(&["link", "del", &self.interface_name])
                .output();
            
            return Err(WireGuardError::ConfigureInterface(
                format!("Failed to configure WireGuard device: {}", stderr)
            ).into());
        }
        
        // 6. 验证配置
        let output = std::process::Command::new("wg")
            .args(&["show", &self.interface_name])
            .output()
            .context("Failed to verify WireGuard config")?;
        
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            tracing::error!("WireGuard config verification failed: {}", stderr);
            
            // 验证失败时回滚
            tracing::warn!("Rolling back interface creation due to verification failure");
            let _ = std::process::Command::new("ip")
                .args(&["link", "del", &self.interface_name])
                .output();
            
            return Err(WireGuardError::ConfigureInterface(
                format!("WireGuard config verification failed: {}", stderr)
            ).into());
        }
        
        self.config = Some(config);
        
        tracing::info!("Successfully created interface {}", self.interface_name);
        Ok(())
    }

    /// 添加 Peer (使用 wg 命令)
    pub fn add_peer(&mut self, peer: PeerConfig) -> Result<()> {
        tracing::info!("Adding peer {} to interface {}", peer.public_key, self.interface_name);
        
        let mut args = vec!["set".to_string(), self.interface_name.clone(), "peer".to_string(), peer.public_key.clone()];
        
        // 添加 endpoint（仅当非空时）
        if let Some(ep) = &peer.endpoint {
            if !ep.is_empty() {
                args.push("endpoint".to_string());
                args.push(ep.clone());
            }
        }
        
        // 添加 allowed-ips（始终添加，空列表会清空原有的）
        args.push("allowed-ips".to_string());
        args.push(peer.allowed_ips.join(",")); // 空列表时为空字符串
        
        // 添加 persistent-keepalive
        if peer.persistent_keepalive > 0 {
            args.push("persistent-keepalive".to_string());
            args.push(peer.persistent_keepalive.to_string());
        }
        
        let output = std::process::Command::new("wg")
            .args(&args.iter().map(|s| s.as_str()).collect::<Vec<_>>())
            .output()
            .context("Failed to execute wg set")?;
        
        if !output.status.success() {
            return Err(WireGuardError::ConfigureInterface(
                String::from_utf8_lossy(&output.stderr).to_string()
            ).into());
        }
        
        tracing::info!("Successfully added peer {}", &peer.public_key[..16]);
        Ok(())
    }

    /// 删除 Peer (使用 wg 命令)
    pub fn remove_peer(&mut self, public_key: &str) -> Result<()> {
        tracing::info!("Removing peer {} from interface {}", public_key, self.interface_name);
        
        let output = std::process::Command::new("wg")
            .args(&["set", &self.interface_name, "peer", public_key, "remove"])
            .output()
            .context("Failed to execute wg set")?;
        
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            // 如果接口不存在，视为成功（peer 已经不存在了）
            if stderr.contains("No such device") || stderr.contains("Interface does not exist") {
                tracing::warn!("Interface {} not found, assuming peer already removed", self.interface_name);
                return Ok(());
            } else if stderr.contains("Permission denied") {
                return Err(WireGuardError::ConfigureInterface(
                    format!("Permission denied: {}", stderr)
                ).into());
            } else {
                // 默认为 PeerNotFound，但包含原始错误信息
                return Err(WireGuardError::PeerNotFound(
                    format!("Failed to remove peer {}: {}", public_key, stderr)
                ).into());
            }
        }
        
        tracing::info!("Successfully removed peer {}", &public_key[..16.min(public_key.len())]);
        Ok(())
    }

    /// 列出所有 Peers (使用 netlink API)
    pub fn list_peers(&self) -> Result<Vec<PeerInfo>> {
        let mut wg = WgSocket::connect()
            .context("Failed to connect to WireGuard netlink")?;
        
        let device = wg.get_device(DeviceInterface::from_name(&self.interface_name))
            .context("Failed to get device info")?;
        
        let peers = device.peers.into_iter().map(|p| {
            let last_hs = if p.last_handshake_time.as_secs() > 0 {
                Some(p.last_handshake_time.as_secs())
            } else {
                None
            };
            
            PeerInfo {
                public_key: base64::Engine::encode(
                    &base64::engine::general_purpose::STANDARD,
                    &p.public_key,
                ),
                endpoint: p.endpoint.map(|e| e.to_string()),
                allowed_ips: p.allowed_ips.iter().map(|ip| format!("{}/{}", ip.ipaddr, ip.cidr_mask)).collect(),
                last_handshake: last_hs,
                rx_bytes: p.rx_bytes,
                tx_bytes: p.tx_bytes,
                persistent_keepalive: p.persistent_keepalive_interval,
            }
        }).collect();
        
        Ok(peers)
    }

    /// 获取接口统计信息
    pub fn get_stats(&self) -> Result<InterfaceStats> {
        let mut wg = WgSocket::connect()
            .context("Failed to connect to WireGuard netlink")?;
        
        let device = wg.get_device(DeviceInterface::from_name(&self.interface_name))
            .context("Failed to get device info")?;
        
        // 修复：使用 public_key 而不是 private_key
        let public_key = device.public_key
            .map(|k| base64::Engine::encode(
                &base64::engine::general_purpose::STANDARD,
                &k,
            ))
            .unwrap_or_default();
        
        let peers = device.peers.into_iter().map(|p| {
            let last_hs = if p.last_handshake_time.as_secs() > 0 {
                Some(p.last_handshake_time.as_secs())
            } else {
                None
            };
            
            PeerInfo {
                public_key: base64::Engine::encode(
                    &base64::engine::general_purpose::STANDARD,
                    &p.public_key,
                ),
                endpoint: p.endpoint.map(|e| e.to_string()),
                allowed_ips: p.allowed_ips.iter().map(|ip| format!("{}/{}", ip.ipaddr, ip.cidr_mask)).collect(),
                last_handshake: last_hs,
                rx_bytes: p.rx_bytes,
                tx_bytes: p.tx_bytes,
                persistent_keepalive: p.persistent_keepalive_interval,
            }
        }).collect();
        
        Ok(InterfaceStats {
            interface_name: self.interface_name.clone(),
            public_key,
            listen_port: device.listen_port,
            peers,
        })
    }

    /// 删除接口
    pub fn delete_interface(&mut self) -> Result<()> {
        tracing::info!("Deleting interface {}", self.interface_name);
        
        let output = std::process::Command::new("ip")
            .args(&["link", "del", &self.interface_name])
            .output()
            .context("Failed to execute ip link del")?;
        
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            // 接口不存在不算错误
            if stderr.contains("Cannot find device") || stderr.contains("does not exist") {
                tracing::info!("Interface {} already deleted", self.interface_name);
                self.config = None;
                return Ok(());
            }
            
            // 其他错误（如权限不足）不清空 config，让调用方决定如何处理
            tracing::error!("Failed to delete interface {}: {}", self.interface_name, stderr);
            return Err(WireGuardError::ConfigureInterface(
                format!("Failed to delete interface: {}", stderr)
            ).into());
        }
        
        self.config = None;
        tracing::info!("Successfully deleted interface {}", self.interface_name);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_keypair() {
        let (private, public) = WireGuardManager::generate_keypair().unwrap();
        assert_eq!(private.len(), 44); // base64 编码的 32 字节
        assert_eq!(public.len(), 44);
    }

    #[test]
    fn test_derive_public_key() {
        let (private, expected_public) = WireGuardManager::generate_keypair().unwrap();
        let derived_public = WireGuardManager::derive_public_key(&private).unwrap();
        assert_eq!(expected_public, derived_public);
    }
}
