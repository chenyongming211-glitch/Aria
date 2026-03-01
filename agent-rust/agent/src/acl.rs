use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, Mutex};

use aya::maps::HashMap as AyaHashMap;
use aya::maps::MapData;
use aya::Pod;
use aya::Ebpf;
use thiserror::Error;

use crate::identity::{IdentityManager, ID_WILDCARD, parse_single_ip};

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
}
