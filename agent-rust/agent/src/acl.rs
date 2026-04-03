use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, Mutex};

use aya::maps::HashMap as AyaHashMap;
use aya::maps::MapData;
use aya::Pod;
use aya::Ebpf;
use thiserror::Error;

use crate::identity::{IdentityManager, ID_WILDCARD, parse_single_ip};
use crate::metrics;

pub const ACTION_DROP: u32 = 0;
pub const ACTION_PASS: u32 = 1;

static RULE_ID_COUNTER: AtomicU32 = AtomicU32::new(1);

#[derive(Error, Debug)]
pub enum AclError {
    #[error("Identity error: {0}")]
    Identity(#[from] crate::identity::IdentityError),
    #[error("eBPF map error: {0}")]
    MapError(#[from] aya::maps::MapError),
    #[error("Map not found: {0}")]
    MapNotFound(String),
    #[error("Invalid parameter: {0}")]
    InvalidParam(String),
    #[error("Lock error")]
    LockError,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, serde::Serialize)]
pub struct PolicyValue {
    pub action: u32,
    pub rule_id: u32,
    pub bytes: u64,
    pub packets: u64,
}

unsafe impl Pod for PolicyValue {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PolicyKey {
    pub src_id: u32,
    pub dst_id: u32,
    pub dst_port: u16,
    pub protocol: u8,
    pub pad: u8,
}

unsafe impl Pod for PolicyKey {}

pub struct AclManager {
    policy_map: AyaHashMap<MapData, PolicyKey, PolicyValue>,
    block_src_id_map: AyaHashMap<MapData, u32, u8>,
    block_dst_id_map: AyaHashMap<MapData, u32, u8>,
    block_port_map: AyaHashMap<MapData, u16, u8>,
    identity_mgr: Arc<Mutex<IdentityManager>>,
}

impl AclManager {
    pub fn new(ebpf: &mut Ebpf, identity_mgr: Arc<Mutex<IdentityManager>>) -> Result<Self, AclError> {
        let policy_map: AyaHashMap<_, PolicyKey, PolicyValue> = ebpf
            .take_map("POLICY_MAP")
            .ok_or_else(|| AclError::MapNotFound("POLICY_MAP".to_string()))?
            .try_into()?;

        let block_src_id_map: AyaHashMap<_, u32, u8> = ebpf
            .take_map("BLOCK_SRC_ID_MAP")
            .ok_or_else(|| AclError::MapNotFound("BLOCK_SRC_ID_MAP".to_string()))?
            .try_into()?;

        let block_dst_id_map: AyaHashMap<_, u32, u8> = ebpf
            .take_map("BLOCK_DST_ID_MAP")
            .ok_or_else(|| AclError::MapNotFound("BLOCK_DST_ID_MAP".to_string()))?
            .try_into()?;

        let block_port_map: AyaHashMap<_, u16, u8> = ebpf
            .take_map("BLOCK_PORT_MAP")
            .ok_or_else(|| AclError::MapNotFound("BLOCK_PORT_MAP".to_string()))?
            .try_into()?;

        Ok(Self {
            policy_map,
            block_src_id_map,
            block_dst_id_map,
            block_port_map,
            identity_mgr,
        })
    }

    fn with_identity_mgr<F, T>(&self, f: F) -> Result<T, AclError>
    where
        F: FnOnce(&mut IdentityManager) -> Result<T, crate::identity::IdentityError>,
    {
        let mut mgr = self.identity_mgr.lock().map_err(|_| AclError::LockError)?;
        f(&mut *mgr).map_err(AclError::Identity)
    }

    fn with_identity_mgr_read<F, T>(&self, f: F) -> Result<T, AclError>
    where
        F: FnOnce(&IdentityManager) -> T,
    {
        let mgr = self.identity_mgr.lock().map_err(|_| AclError::LockError)?;
        Ok(f(&*mgr))
    }

    pub fn block_src_id(&mut self, id: u32) -> Result<(), AclError> {
        self.block_src_id_map.insert(id, 1, 0)?;
        Ok(())
    }

    pub fn unblock_src_id(&mut self, id: u32) -> Result<(), AclError> {
        self.block_src_id_map.remove(&id)?;
        Ok(())
    }

    pub fn block_dst_id(&mut self, id: u32) -> Result<(), AclError> {
        self.block_dst_id_map.insert(id, 1, 0)?;
        Ok(())
    }

    pub fn unblock_dst_id(&mut self, id: u32) -> Result<(), AclError> {
        self.block_dst_id_map.remove(&id)?;
        Ok(())
    }

    pub fn block_src_cidr(&mut self, cidr: &str) -> Result<u32, AclError> {
        let id = self.with_identity_mgr(|m| m.assign_id(cidr))?;
        self.block_src_id(id)?;
        Ok(id)
    }

    pub fn unblock_src_cidr(&mut self, cidr: &str) -> Result<(), AclError> {
        let id = self.with_identity_mgr_read(|m| m.get_id(cidr))?;
        if let Some(id) = id {
            self.unblock_src_id(id)?;
        }
        Ok(())
    }

    pub fn block_dst_cidr(&mut self, cidr: &str) -> Result<u32, AclError> {
        let id = self.with_identity_mgr(|m| m.assign_id(cidr))?;
        self.block_dst_id(id)?;
        Ok(id)
    }

    pub fn unblock_dst_cidr(&mut self, cidr: &str) -> Result<(), AclError> {
        let id = self.with_identity_mgr_read(|m| m.get_id(cidr))?;
        if let Some(id) = id {
            self.unblock_dst_id(id)?;
        }
        Ok(())
    }

    pub fn block_src_ip(&mut self, ip: &str) -> Result<u32, AclError> {
        let cidr = if ip.contains('/') { ip.to_string() } else { 
            let entry = parse_single_ip(ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.block_src_cidr(&cidr)
    }

    pub fn unblock_src_ip(&mut self, ip: &str) -> Result<(), AclError> {
        let cidr = if ip.contains('/') { ip.to_string() } else {
            let entry = parse_single_ip(ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.unblock_src_cidr(&cidr)
    }

    pub fn block_dst_ip(&mut self, ip: &str) -> Result<u32, AclError> {
        let cidr = if ip.contains('/') { ip.to_string() } else {
            let entry = parse_single_ip(ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.block_dst_cidr(&cidr)
    }

    pub fn unblock_dst_ip(&mut self, ip: &str) -> Result<(), AclError> {
        let cidr = if ip.contains('/') { ip.to_string() } else {
            let entry = parse_single_ip(ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.unblock_dst_cidr(&cidr)
    }

    pub fn block_port(&mut self, port: u16) -> Result<(), AclError> {
        self.block_port_map.insert(port, 1, 0)?;
        Ok(())
    }

    pub fn unblock_port(&mut self, port: u16) -> Result<(), AclError> {
        self.block_port_map.remove(&port)?;
        Ok(())
    }

    pub fn allow(
        &mut self,
        src_cidr: &str,
        dst_cidr: &str,
        dst_port: u16,
        protocol: u8,
    ) -> Result<u32, AclError> {
        self.apply_policy(src_cidr, dst_cidr, dst_port, protocol, ACTION_PASS)
    }

    pub fn deny(
        &mut self,
        src_cidr: &str,
        dst_cidr: &str,
        dst_port: u16,
        protocol: u8,
    ) -> Result<u32, AclError> {
        self.apply_policy(src_cidr, dst_cidr, dst_port, protocol, ACTION_DROP)
    }

    pub fn apply_policy(
        &mut self,
        src_cidr: &str,
        dst_cidr: &str,
        dst_port: u16,
        protocol: u8,
        action: u32,
    ) -> Result<u32, AclError> {
        let src_id = if src_cidr.is_empty() || src_cidr == "0" {
            ID_WILDCARD
        } else {
            self.with_identity_mgr(|m| m.assign_id(src_cidr))?
        };

        let dst_id = if dst_cidr.is_empty() || dst_cidr == "0" {
            ID_WILDCARD
        } else {
            self.with_identity_mgr(|m| m.assign_id(dst_cidr))?
        };

        let rule_id = RULE_ID_COUNTER.fetch_add(1, Ordering::SeqCst);

        let key = PolicyKey {
            src_id,
            dst_id,
            dst_port: dst_port.to_be(),
            protocol,
            pad: 0,
        };

        let value = PolicyValue {
            action,
            rule_id,
            bytes: 0,
            packets: 0,
        };

        self.policy_map.insert(key, value, 0)?;
        Ok(rule_id)
    }

    pub fn remove_rule(
        &mut self,
        src_cidr: &str,
        dst_cidr: &str,
        dst_port: u16,
        protocol: u8,
    ) -> Result<(), AclError> {
        let src_id = if src_cidr.is_empty() || src_cidr == "0" {
            ID_WILDCARD
        } else {
            self.with_identity_mgr_read(|m| m.get_id(src_cidr))?.unwrap_or(ID_WILDCARD)
        };

        let dst_id = if dst_cidr.is_empty() || dst_cidr == "0" {
            ID_WILDCARD
        } else {
            self.with_identity_mgr_read(|m| m.get_id(dst_cidr))?.unwrap_or(ID_WILDCARD)
        };

        let key = PolicyKey {
            src_id,
            dst_id,
            dst_port: dst_port.to_be(),
            protocol,
            pad: 0,
        };

        self.policy_map.remove(&key)?;
        Ok(())
    }

    pub fn get_rule_stats(
        &self,
        src_cidr: &str,
        dst_cidr: &str,
        dst_port: u16,
        protocol: u8,
    ) -> Result<Option<PolicyValue>, AclError> {
        let src_id = if src_cidr.is_empty() || src_cidr == "0" {
            ID_WILDCARD
        } else {
            self.with_identity_mgr_read(|m| m.get_id(src_cidr))?.unwrap_or(ID_WILDCARD)
        };

        let dst_id = if dst_cidr.is_empty() || dst_cidr == "0" {
            ID_WILDCARD
        } else {
            self.with_identity_mgr_read(|m| m.get_id(dst_cidr))?.unwrap_or(ID_WILDCARD)
        };

        let key = PolicyKey {
            src_id,
            dst_id,
            dst_port: dst_port.to_be(),
            protocol,
            pad: 0,
        };

        Ok(self.policy_map.get(&key, 0).ok())
    }
    
    pub fn get_all_rule_stats(&self) -> Result<Vec<(PolicyKey, PolicyValue)>, AclError> {
        let mut stats = Vec::new();
        let mut errors = 0;
        
        for entry in self.policy_map.iter() {
            match entry {
                Ok((key, value)) => {
                    stats.push((key, value));
                }
                Err(e) => {
                    errors += 1;
                    tracing::warn!("Failed to read policy map entry: {:?}", e);
                }
            }
        }
        
        if errors > 0 {
            tracing::warn!(
                "get_all_rule_stats completed with {} errors, {} entries read successfully",
                errors, stats.len()
            );
        }
        
        Ok(stats)
    }
    
    pub fn clear_all_rules(&mut self) -> Result<(), AclError> {
        // 第一次遍历：收集所有 key
        let mut first_pass_errors = 0;
        let keys: Vec<PolicyKey> = self.policy_map.iter()
            .filter_map(|entry| {
                match entry {
                    Ok((key, _)) => Some(key),
                    Err(e) => {
                        first_pass_errors += 1;
                        tracing::warn!("Failed to read policy map entry in first pass: {:?}", e);
                        None
                    }
                }
            })
            .collect();
        
        let total_count = keys.len();
        let mut removed_count = 0;
        let mut failed_keys = Vec::new();
        
        // 第二次遍历：尝试删除，最多重试3次
        for key in keys {
            let mut success = false;
            
            for attempt in 1..=3 {
                match self.policy_map.remove(&key) {
                    Ok(_) => {
                        success = true;
                        removed_count += 1;
                        break;
                    }
                    Err(e) if attempt < 3 => {
                        tracing::debug!("Attempt {} failed to remove policy rule, retrying immediately: {:?}", attempt, e);
                    }
                    Err(e) => {
                        tracing::error!(
                            "Failed to remove policy rule after 3 attempts (src_id={}, dst_id={}, port={}, proto={}): {:?}",
                            key.src_id, key.dst_id, key.dst_port, key.protocol, e
                        );
                        failed_keys.push(key);
                    }
                }
            }
        }
        
        // 如果第一次遍历有读取失败，尝试再次迭代清理
        if first_pass_errors > 0 {
            tracing::warn!("Retrying to clear {} entries that failed to read in first pass", first_pass_errors);
            
            let retry_keys: Vec<PolicyKey> = self.policy_map.iter()
                .filter_map(|entry| entry.ok())
                .map(|(key, _)| key)
                .collect();
            
            for key in retry_keys {
                for attempt in 1..=2 {
                    match self.policy_map.remove(&key) {
                        Ok(_) => {
                            removed_count += 1;
                            tracing::info!("Successfully removed previously unreadable entry on retry");
                            break;
                        }
                        Err(e) if attempt < 2 => {
                            tracing::debug!("Retry attempt {} failed: {:?}", attempt, e);
                        }
                        Err(e) => {
                            tracing::error!("Failed to remove entry even after retry: {:?}", e);
                        }
                    }
                }
            }
        }
        
        if !failed_keys.is_empty() || first_pass_errors > 0 {
            tracing::error!(
                "clear_all_rules completed: {} rules removed, {} deletions failed, {} read errors",
                removed_count, failed_keys.len(), first_pass_errors
            );
            metrics::record_cleanup_failure("acl_policy_map", (failed_keys.len() + first_pass_errors) as u64);
        } else {
            tracing::info!("Cleared {} ACL rules successfully", removed_count);
        }
        
        Ok(())
    }

    pub fn clear_all_blacklists(&mut self) -> Result<(), AclError> {
        let src_ids: Vec<u32> = self
            .block_src_id_map
            .iter()
            .filter_map(|entry| entry.ok().map(|(key, _)| key))
            .collect();
        for id in src_ids {
            let _ = self.block_src_id_map.remove(&id);
        }

        let dst_ids: Vec<u32> = self
            .block_dst_id_map
            .iter()
            .filter_map(|entry| entry.ok().map(|(key, _)| key))
            .collect();
        for id in dst_ids {
            let _ = self.block_dst_id_map.remove(&id);
        }

        let ports: Vec<u16> = self
            .block_port_map
            .iter()
            .filter_map(|entry| entry.ok().map(|(key, _)| key))
            .collect();
        for port in ports {
            let _ = self.block_port_map.remove(&port);
        }

        Ok(())
    }
}
