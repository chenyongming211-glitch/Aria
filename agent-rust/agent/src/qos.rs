use std::time::{SystemTime, UNIX_EPOCH};
use std::sync::{Arc, Mutex};

use aya::maps::HashMap as AyaHashMap;
use aya::maps::MapData;
use aya::Pod;
use aya::Ebpf;
use thiserror::Error;

use crate::identity::{IdentityManager, ID_WILDCARD, parse_single_ip};
use crate::metrics;

#[derive(Error, Debug)]
pub enum QoSError {
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
pub struct BucketState {
    pub rate_bytes_per_sec: u64,
    pub burst_bytes: u64,
    pub tokens: u64,
    pub last_update_ns: u64,
    pub pass_bytes: u64,
    pub drop_bytes: u64,
    pub _pad: u32,
    pub rule_id: u32,
}

unsafe impl Pod for BucketState {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct ServiceQoSKey {
    pub src_id: u32,
    pub dst_id: u32,
    pub dst_port: u16,
    pub protocol: u8,
    pub pad: u8,
}

unsafe impl Pod for ServiceQoSKey {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PairQoSKey {
    pub src_id: u32,
    pub dst_id: u32,
}

unsafe impl Pod for PairQoSKey {}

pub struct QoSManager {
    src_id_qos_map: AyaHashMap<MapData, u32, BucketState>,
    pair_id_qos_map: AyaHashMap<MapData, PairQoSKey, BucketState>,
    service_qos_map: AyaHashMap<MapData, ServiceQoSKey, BucketState>,
    identity_mgr: Arc<Mutex<IdentityManager>>,
}

impl QoSManager {
    pub fn new(ebpf: &mut Ebpf, identity_mgr: Arc<Mutex<IdentityManager>>) -> Result<Self, QoSError> {
        let src_id_qos_map: AyaHashMap<_, u32, BucketState> = ebpf
            .take_map("SRC_ID_QOS_MAP")
            .ok_or_else(|| QoSError::MapNotFound("SRC_ID_QOS_MAP".to_string()))?
            .try_into()?;

        let pair_id_qos_map: AyaHashMap<_, PairQoSKey, BucketState> = ebpf
            .take_map("PAIR_ID_QOS_MAP")
            .ok_or_else(|| QoSError::MapNotFound("PAIR_ID_QOS_MAP".to_string()))?
            .try_into()?;

        let service_qos_map: AyaHashMap<_, ServiceQoSKey, BucketState> = ebpf
            .take_map("SERVICE_QOS_MAP")
            .ok_or_else(|| QoSError::MapNotFound("SERVICE_QOS_MAP".to_string()))?
            .try_into()?;

        Ok(Self {
            src_id_qos_map,
            pair_id_qos_map,
            service_qos_map,
            identity_mgr,
        })
    }

    fn with_identity_mgr<F, T>(&self, f: F) -> Result<T, QoSError>
    where
        F: FnOnce(&mut IdentityManager) -> Result<T, crate::identity::IdentityError>,
    {
        let mut mgr = self.identity_mgr.lock().map_err(|_| QoSError::LockError)?;
        f(&mut *mgr).map_err(QoSError::Identity)
    }

    fn with_identity_mgr_read<F, T>(&self, f: F) -> Result<T, QoSError>
    where
        F: FnOnce(&IdentityManager) -> T,
    {
        let mgr = self.identity_mgr.lock().map_err(|_| QoSError::LockError)?;
        Ok(f(&*mgr))
    }

    pub fn limit_src_id(&mut self, id: u32, mbps: u64) -> Result<(), QoSError> {
        let (rate, burst) = calculate_bucket_params(mbps);
        let now_ns = now_ns();

        let bucket = BucketState {
            rate_bytes_per_sec: rate,
            burst_bytes: burst,
            tokens: burst,
            last_update_ns: now_ns,
            pass_bytes: 0,
            drop_bytes: 0,
            _pad: 0,
            rule_id: 0,
        };

        self.src_id_qos_map.insert(id, bucket, 0)?;
        Ok(())
    }

    pub fn limit_src_cidr(&mut self, cidr: &str, mbps: u64) -> Result<u32, QoSError> {
        let id = self.with_identity_mgr(|m| m.assign_id(cidr))?;
        self.limit_src_id(id, mbps)?;
        Ok(id)
    }

    pub fn remove_src_id_limit(&mut self, id: u32) -> Result<(), QoSError> {
        self.src_id_qos_map.remove(&id)?;
        Ok(())
    }

    pub fn remove_src_cidr_limit(&mut self, cidr: &str) -> Result<(), QoSError> {
        let id = self.with_identity_mgr_read(|m| m.get_id(cidr))?;
        if let Some(id) = id {
            self.remove_src_id_limit(id)?;
        }
        Ok(())
    }

    pub fn limit_pair_id(&mut self, src_id: u32, dst_id: u32, mbps: u64) -> Result<(), QoSError> {
        let (rate, burst) = calculate_bucket_params(mbps);
        let now_ns = now_ns();

        let bucket = BucketState {
            rate_bytes_per_sec: rate,
            burst_bytes: burst,
            tokens: burst,
            last_update_ns: now_ns,
            pass_bytes: 0,
            drop_bytes: 0,
            _pad: 0,
            rule_id: 0,
        };

        let key = PairQoSKey { src_id, dst_id };
        self.pair_id_qos_map.insert(key, bucket, 0)?;
        Ok(())
    }

    pub fn limit_pair_cidr(
        &mut self,
        src_cidr: &str,
        dst_cidr: &str,
        mbps: u64,
    ) -> Result<(u32, u32), QoSError> {
        if src_cidr.is_empty() || dst_cidr.is_empty() {
            return Err(QoSError::InvalidParam("src_cidr and dst_cidr are required for pair limit".to_string()));
        }

        let src_id = self.with_identity_mgr(|m| m.assign_id(src_cidr))?;
        let dst_id = self.with_identity_mgr(|m| m.assign_id(dst_cidr))?;
        self.limit_pair_id(src_id, dst_id, mbps)?;
        Ok((src_id, dst_id))
    }

    pub fn remove_pair_id_limit(&mut self, src_id: u32, dst_id: u32) -> Result<(), QoSError> {
        let key = PairQoSKey { src_id, dst_id };
        self.pair_id_qos_map.remove(&key)?;
        Ok(())
    }

    pub fn remove_pair_cidr_limit(&mut self, src_cidr: &str, dst_cidr: &str) -> Result<(), QoSError> {
        let src_id = self.with_identity_mgr_read(|m| m.get_id(src_cidr))?.unwrap_or(ID_WILDCARD);
        let dst_id = self.with_identity_mgr_read(|m| m.get_id(dst_cidr))?.unwrap_or(ID_WILDCARD);
        self.remove_pair_id_limit(src_id, dst_id)?;
        Ok(())
    }

    pub fn limit_service_id(
        &mut self,
        src_id: u32,
        dst_id: u32,
        dst_port: u16,
        protocol: u8,
        mbps: u64,
    ) -> Result<(), QoSError> {
        let (rate, burst) = calculate_bucket_params(mbps);
        let now_ns = now_ns();

        let bucket = BucketState {
            rate_bytes_per_sec: rate,
            burst_bytes: burst,
            tokens: burst,
            last_update_ns: now_ns,
            pass_bytes: 0,
            drop_bytes: 0,
            _pad: 0,
            rule_id: 0,
        };

        let key = ServiceQoSKey {
            src_id,
            dst_id,
            dst_port: dst_port.to_be(),
            protocol,
            pad: 0,
        };

        self.service_qos_map.insert(key, bucket, 0)?;
        Ok(())
    }

    pub fn limit_service_cidr(
        &mut self,
        src_cidr: &str,
        dst_cidr: &str,
        dst_port: u16,
        protocol: u8,
        mbps: u64,
    ) -> Result<(u32, u32), QoSError> {
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

        self.limit_service_id(src_id, dst_id, dst_port, protocol, mbps)?;
        Ok((src_id, dst_id))
    }

    pub fn remove_service_id_limit(
        &mut self,
        src_id: u32,
        dst_id: u32,
        dst_port: u16,
        protocol: u8,
    ) -> Result<(), QoSError> {
        let key = ServiceQoSKey {
            src_id,
            dst_id,
            dst_port: dst_port.to_be(),
            protocol,
            pad: 0,
        };
        self.service_qos_map.remove(&key)?;
        Ok(())
    }

    pub fn remove_service_cidr_limit(
        &mut self,
        src_cidr: &str,
        dst_cidr: &str,
        dst_port: u16,
        protocol: u8,
    ) -> Result<(), QoSError> {
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

        self.remove_service_id_limit(src_id, dst_id, dst_port, protocol)?;
        Ok(())
    }

    pub fn get_src_id_stats(&self, id: u32) -> Result<Option<BucketState>, QoSError> {
        Ok(self.src_id_qos_map.get(&id, 0).ok())
    }

    pub fn get_src_cidr_stats(&self, cidr: &str) -> Result<Option<BucketState>, QoSError> {
        let id = self.with_identity_mgr_read(|m| m.get_id(cidr))?;
        if let Some(id) = id {
            self.get_src_id_stats(id)
        } else {
            Ok(None)
        }
    }

    pub fn get_pair_id_stats(&self, src_id: u32, dst_id: u32) -> Result<Option<BucketState>, QoSError> {
        let key = PairQoSKey { src_id, dst_id };
        Ok(self.pair_id_qos_map.get(&key, 0).ok())
    }

    pub fn get_pair_cidr_stats(&self, src_cidr: &str, dst_cidr: &str) -> Result<Option<BucketState>, QoSError> {
        let src_id = self.with_identity_mgr_read(|m| m.get_id(src_cidr))?.unwrap_or(ID_WILDCARD);
        let dst_id = self.with_identity_mgr_read(|m| m.get_id(dst_cidr))?.unwrap_or(ID_WILDCARD);
        self.get_pair_id_stats(src_id, dst_id)
    }

    pub fn get_service_id_stats(
        &self,
        src_id: u32,
        dst_id: u32,
        dst_port: u16,
        protocol: u8,
    ) -> Result<Option<BucketState>, QoSError> {
        let key = ServiceQoSKey {
            src_id,
            dst_id,
            dst_port: dst_port.to_be(),
            protocol,
            pad: 0,
        };
        Ok(self.service_qos_map.get(&key, 0).ok())
    }

    pub fn get_service_cidr_stats(
        &self,
        src_cidr: &str,
        dst_cidr: &str,
        dst_port: u16,
        protocol: u8,
    ) -> Result<Option<BucketState>, QoSError> {
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

        self.get_service_id_stats(src_id, dst_id, dst_port, protocol)
    }

    // ========== 便捷方法（供 main.rs 使用）==========

    pub fn limit_ip(&mut self, ip: &str, mbps: u64) -> Result<(), QoSError> {
        let cidr = if ip.contains('/') { ip.to_string() } else {
            let entry = parse_single_ip(ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.limit_src_cidr(&cidr, mbps)?;
        Ok(())
    }

    pub fn limit_peer_pair(&mut self, src_ip: &str, dst_ip: &str, mbps: u64) -> Result<(), QoSError> {
        let src_cidr = if src_ip.contains('/') { src_ip.to_string() } else {
            let entry = parse_single_ip(src_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        let dst_cidr = if dst_ip.contains('/') { dst_ip.to_string() } else {
            let entry = parse_single_ip(dst_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.limit_pair_cidr(&src_cidr, &dst_cidr, mbps)?;
        Ok(())
    }

    pub fn limit_port(&mut self, port: u16, mbps: u64, protocol: u8) -> Result<(), QoSError> {
        self.limit_service_id(ID_WILDCARD, ID_WILDCARD, port, protocol, mbps)
    }

    /// 服务限速 - 基于 (源ID, 目的ID, 目的端口, 协议)
    /// 注意：源端口参数被忽略，仅用于接口兼容性
    /// 限速基于目的端口，因为服务通常通过目的端口识别
    pub fn limit_service(
        &mut self,
        src_ip: &str,
        dst_ip: &str,
        _src_port: u16,
        dst_port: u16,
        protocol: u8,
        mbps: u64,
    ) -> Result<(), QoSError> {
        let src_cidr = if src_ip.is_empty() || src_ip == "0" {
            "".to_string()
        } else if src_ip.contains('/') {
            src_ip.to_string()
        } else {
            let entry = parse_single_ip(src_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        
        let dst_cidr = if dst_ip.is_empty() || dst_ip == "0" {
            "".to_string()
        } else if dst_ip.contains('/') {
            dst_ip.to_string()
        } else {
            let entry = parse_single_ip(dst_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        
        self.limit_service_cidr(&src_cidr, &dst_cidr, dst_port, protocol, mbps)?;
        Ok(())
    }

    pub fn remove_ip_limit(&mut self, ip: &str) -> Result<(), QoSError> {
        let cidr = if ip.contains('/') { ip.to_string() } else {
            let entry = parse_single_ip(ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.remove_src_cidr_limit(&cidr)
    }

    pub fn remove_peer_limit(&mut self, src_ip: &str, dst_ip: &str) -> Result<(), QoSError> {
        let src_cidr = if src_ip.contains('/') { src_ip.to_string() } else {
            let entry = parse_single_ip(src_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        let dst_cidr = if dst_ip.contains('/') { dst_ip.to_string() } else {
            let entry = parse_single_ip(dst_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.remove_pair_cidr_limit(&src_cidr, &dst_cidr)
    }

    pub fn remove_service_limit(
        &mut self,
        src_ip: &str,
        dst_ip: &str,
        _src_port: u16,
        dst_port: u16,
        protocol: u8,
    ) -> Result<(), QoSError> {
        let src_cidr = if src_ip.is_empty() || src_ip == "0" {
            "".to_string()
        } else if src_ip.contains('/') {
            src_ip.to_string()
        } else {
            let entry = parse_single_ip(src_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        
        let dst_cidr = if dst_ip.is_empty() || dst_ip == "0" {
            "".to_string()
        } else if dst_ip.contains('/') {
            dst_ip.to_string()
        } else {
            let entry = parse_single_ip(dst_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        
        self.remove_service_cidr_limit(&src_cidr, &dst_cidr, dst_port, protocol)
    }

    pub fn get_ip_stats(&self, ip: &str) -> Result<Option<BucketState>, QoSError> {
        let cidr = if ip.contains('/') { ip.to_string() } else {
            let entry = parse_single_ip(ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.get_src_cidr_stats(&cidr)
    }

    pub fn get_peer_stats(&self, src_ip: &str, dst_ip: &str) -> Result<Option<BucketState>, QoSError> {
        let src_cidr = if src_ip.contains('/') { src_ip.to_string() } else {
            let entry = parse_single_ip(src_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        let dst_cidr = if dst_ip.contains('/') { dst_ip.to_string() } else {
            let entry = parse_single_ip(dst_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        self.get_pair_cidr_stats(&src_cidr, &dst_cidr)
    }

    pub fn get_service_stats(
        &self,
        src_ip: &str,
        dst_ip: &str,
        _src_port: u16,
        dst_port: u16,
        protocol: u8,
    ) -> Result<Option<BucketState>, QoSError> {
        let src_cidr = if src_ip.is_empty() || src_ip == "0" {
            "".to_string()
        } else if src_ip.contains('/') {
            src_ip.to_string()
        } else {
            let entry = parse_single_ip(src_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        
        let dst_cidr = if dst_ip.is_empty() || dst_ip == "0" {
            "".to_string()
        } else if dst_ip.contains('/') {
            dst_ip.to_string()
        } else {
            let entry = parse_single_ip(dst_ip)?;
            format!("{}/{}", entry.network, entry.prefix_len)
        };
        
        self.get_service_cidr_stats(&src_cidr, &dst_cidr, dst_port, protocol)
    }
    
    pub fn get_all_qos_stats(&self) -> Result<Vec<(String, u32, u64, u64)>, QoSError> {
        let mut stats = Vec::new();
        let mut errors = 0;
        
        for entry in self.src_id_qos_map.iter() {
            match entry {
                Ok((_, value)) => {
                    stats.push((
                        "ip".to_string(),
                        value.rule_id,
                        value.pass_bytes,
                        value.drop_bytes,
                    ));
                }
                Err(e) => {
                    errors += 1;
                    tracing::warn!("Failed to read src_id_qos_map entry: {:?}", e);
                }
            }
        }
        
        for entry in self.pair_id_qos_map.iter() {
            match entry {
                Ok((_, value)) => {
                    stats.push((
                        "peer".to_string(),
                        value.rule_id,
                        value.pass_bytes,
                        value.drop_bytes,
                    ));
                }
                Err(e) => {
                    errors += 1;
                    tracing::warn!("Failed to read pair_id_qos_map entry: {:?}", e);
                }
            }
        }
        
        for entry in self.service_qos_map.iter() {
            match entry {
                Ok((_, value)) => {
                    stats.push((
                        "service".to_string(),
                        value.rule_id,
                        value.pass_bytes,
                        value.drop_bytes,
                    ));
                }
                Err(e) => {
                    errors += 1;
                    tracing::warn!("Failed to read service_qos_map entry: {:?}", e);
                }
            }
        }
        
        if errors > 0 {
            tracing::warn!(
                "get_all_qos_stats completed with {} errors, {} entries read successfully",
                errors, stats.len()
            );
        }
        
        Ok(stats)
    }
    
    pub fn clear_all_rules(&mut self) -> Result<(), QoSError> {
        let mut read_errors = 0;
        
        let ip_keys: Vec<u32> = self.src_id_qos_map.iter()
            .filter_map(|entry| {
                match entry {
                    Ok((key, _)) => Some(key),
                    Err(e) => {
                        read_errors += 1;
                        tracing::warn!("Failed to read src_id_qos_map entry: {:?}", e);
                        None
                    }
                }
            })
            .collect();
        
        let pair_keys: Vec<PairQoSKey> = self.pair_id_qos_map.iter()
            .filter_map(|entry| {
                match entry {
                    Ok((key, _)) => Some(key),
                    Err(e) => {
                        read_errors += 1;
                        tracing::warn!("Failed to read pair_id_qos_map entry: {:?}", e);
                        None
                    }
                }
            })
            .collect();
        
        let service_keys: Vec<ServiceQoSKey> = self.service_qos_map.iter()
            .filter_map(|entry| {
                match entry {
                    Ok((key, _)) => Some(key),
                    Err(e) => {
                        read_errors += 1;
                        tracing::warn!("Failed to read service_qos_map entry: {:?}", e);
                        None
                    }
                }
            })
            .collect();
        
        let mut removed_count = 0;
        let mut failed_count = 0;
        
        for key in ip_keys {
            for attempt in 1..=3 {
                match self.src_id_qos_map.remove(&key) {
                    Ok(_) => {
                        removed_count += 1;
                        break;
                    }
                    Err(e) if attempt < 3 => {
                        tracing::debug!("Attempt {} failed to remove IP QoS rule: {:?}", attempt, e);
                    }
                    Err(e) => {
                        tracing::error!("Failed to remove IP QoS rule after 3 attempts: {:?}", e);
                        failed_count += 1;
                    }
                }
            }
        }
        
        for key in pair_keys {
            for attempt in 1..=3 {
                match self.pair_id_qos_map.remove(&key) {
                    Ok(_) => {
                        removed_count += 1;
                        break;
                    }
                    Err(e) if attempt < 3 => {
                        tracing::debug!("Attempt {} failed to remove pair QoS rule: {:?}", attempt, e);
                    }
                    Err(e) => {
                        tracing::error!("Failed to remove pair QoS rule after 3 attempts: {:?}", e);
                        failed_count += 1;
                    }
                }
            }
        }
        
        for key in service_keys {
            for attempt in 1..=3 {
                match self.service_qos_map.remove(&key) {
                    Ok(_) => {
                        removed_count += 1;
                        break;
                    }
                    Err(e) if attempt < 3 => {
                        tracing::debug!("Attempt {} failed to remove service QoS rule: {:?}", attempt, e);
                    }
                    Err(e) => {
                        tracing::error!("Failed to remove service QoS rule after 3 attempts: {:?}", e);
                        failed_count += 1;
                    }
                }
            }
        }
        
        if failed_count > 0 || read_errors > 0 {
            tracing::error!(
                "clear_all_rules completed: {} rules removed, {} deletions failed, {} read errors",
                removed_count, failed_count, read_errors
            );
            metrics::record_cleanup_failure("qos_maps", (failed_count + read_errors) as u64);
        } else {
            tracing::info!("Cleared {} QoS rules successfully", removed_count);
        }
        
        Ok(())
    }
}

fn calculate_bucket_params(mbps: u64) -> (u64, u64) {
    let rate = mbps * 1_000_000 / 8;
    let burst = rate / 10;
    let burst = burst.max(1500);
    (rate, burst)
}

fn now_ns() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos() as u64)
        .unwrap_or(0)
}
