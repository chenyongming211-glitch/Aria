use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::net::{IpAddr, SocketAddr};
use thiserror::Error;

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

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PeerInfo {
    pub public_key: String,
    pub endpoint: Option<String>,
    pub allowed_ips: Vec<String>,
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

    /// 生成新的密钥对
    pub fn generate_keypair() -> Result<(String, String)> {
        use x25519_dalek::{EphemeralSecret, PublicKey};
        use rand::rngs::OsRng;

        let secret = EphemeralSecret::random_from_rng(OsRng);
        let public = PublicKey::from(&secret);
        
        let private_key = base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            secret.to_bytes(),
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
        // TODO: 实现使用 netlink 创建接口
        // 步骤:
        // 1. 使用 rtnetlink 创建 WireGuard 接口
        // 2. 设置 MTU
        // 3. 配置私钥和监听端口
        // 4. 启动接口
        
        self.config = Some(config);
        
        // 暂时返回成功，实际功能待实现
        tracing::info!(
            "WireGuard interface {} creation queued (implementation in progress)",
            self.interface_name
        );
        
        Ok(())
    }

    /// 添加 Peer
    pub fn add_peer(&mut self, peer: PeerConfig) -> Result<()> {
        // TODO: 实现 peer 配置
        tracing::info!(
            "Adding peer {} to interface {} (implementation in progress)",
            peer.public_key,
            self.interface_name
        );
        Ok(())
    }

    /// 删除 Peer
    pub fn remove_peer(&mut self, public_key: &str) -> Result<()> {
        // TODO: 实现 peer 删除
        tracing::info!(
            "Removing peer {} from interface {} (implementation in progress)",
            public_key,
            self.interface_name
        );
        Ok(())
    }

    /// 列出所有 Peers
    pub fn list_peers(&self) -> Result<Vec<PeerInfo>> {
        // TODO: 实现获取 peer 列表
        Ok(Vec::new())
    }

    /// 获取接口统计信息
    pub fn get_stats(&self) -> Result<InterfaceStats> {
        Ok(InterfaceStats {
            interface_name: self.interface_name.clone(),
            public_key: String::new(),
            listen_port: DEFAULT_LISTEN_PORT,
            peers: Vec::new(),
        })
    }

    /// 删除接口
    pub fn delete_interface(&mut self) -> Result<()> {
        // TODO: 实现接口删除
        tracing::info!(
            "Deleting interface {} (implementation in progress)",
            self.interface_name
        );
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
