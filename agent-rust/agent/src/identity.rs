use std::net::IpAddr;
use std::collections::HashMap;
use std::sync::atomic::{AtomicU32, Ordering};

use aya::maps::LpmTrie as AyaLpmTrie;
use aya::maps::MapData;
use aya::maps::lpm_trie::Key;
use aya::Ebpf;
use thiserror::Error;

static ID_COUNTER: AtomicU32 = AtomicU32::new(1);
pub const ID_WILDCARD: u32 = 0;

#[derive(Error, Debug)]
pub enum IdentityError {
    #[error("Invalid IP address: {0}")]
    InvalidIp(String),
    #[error("Invalid CIDR: {0}")]
    InvalidCidr(String),
    #[error("eBPF map error: {0}")]
    MapError(#[from] aya::maps::MapError),
    #[error("Map not found: {0}")]
    MapNotFound(String),
    #[error("ID not found: {0}")]
    #[allow(dead_code)]
    IdNotFound(u32),
}

#[derive(Clone, Debug, Hash, PartialEq, Eq)]
pub struct CidrEntry {
    pub network: IpAddr,
    pub prefix_len: u8,
}

struct IdentityMapSet {
    src_ipv4_id_map: AyaLpmTrie<MapData, u32, u32>,
    dst_ipv4_id_map: AyaLpmTrie<MapData, u32, u32>,
    src_ipv6_id_map: AyaLpmTrie<MapData, [u8; 16], u32>,
    dst_ipv6_id_map: AyaLpmTrie<MapData, [u8; 16], u32>,
}

pub struct IdentityManager {
    map_sets: Vec<IdentityMapSet>,
    cidr_to_id: HashMap<CidrEntry, u32>,
    id_to_cidr: HashMap<u32, CidrEntry>,
}

impl IdentityManager {
    pub fn new(primary_ebpf: &mut Ebpf, secondary_ebpf: Option<&mut Ebpf>) -> Result<Self, IdentityError> {
        let mut map_sets = vec![IdentityMapSet::take(primary_ebpf)?];
        if let Some(ebpf) = secondary_ebpf {
            map_sets.push(IdentityMapSet::take(ebpf)?);
        }

        Ok(Self {
            map_sets,
            cidr_to_id: HashMap::new(),
            id_to_cidr: HashMap::new(),
        })
    }

    pub fn assign_id(&mut self, cidr: &str) -> Result<u32, IdentityError> {
        let entry = parse_cidr(cidr)?;

        if let Some(&id) = self.cidr_to_id.get(&entry) {
            return Ok(id);
        }

        let id = ID_COUNTER.fetch_add(1, Ordering::SeqCst);

        for map_set in &mut self.map_sets {
            map_set.insert(&entry, id)?;
        }

        self.cidr_to_id.insert(entry.clone(), id);
        self.id_to_cidr.insert(id, entry);

        Ok(id)
    }

    #[allow(dead_code)]
    pub fn remove_id(&mut self, id: u32) -> Result<(), IdentityError> {
        let entry = self.id_to_cidr.remove(&id)
            .ok_or_else(|| IdentityError::IdNotFound(id))?;

        for map_set in &mut self.map_sets {
            map_set.remove(&entry);
        }

        self.cidr_to_id.remove(&entry);

        Ok(())
    }

    pub fn get_id(&self, cidr: &str) -> Option<u32> {
        let entry = parse_cidr(cidr).ok()?;
        self.cidr_to_id.get(&entry).copied()
    }

    #[allow(dead_code)]
    pub fn get_cidr(&self, id: u32) -> Option<&CidrEntry> {
        self.id_to_cidr.get(&id)
    }

    #[allow(dead_code)]
    pub fn list_all(&self) -> Vec<(u32, String)> {
        self.id_to_cidr
            .iter()
            .map(|(id, entry)| {
                let cidr = format!("{}/{}", entry.network, entry.prefix_len);
                (*id, cidr)
            })
            .collect()
    }
}

impl IdentityMapSet {
    fn take(ebpf: &mut Ebpf) -> Result<Self, IdentityError> {
        let src_ipv4_id_map: AyaLpmTrie<_, u32, u32> = ebpf
            .take_map("SRC_IPV4_ID_MAP")
            .ok_or_else(|| IdentityError::MapNotFound("SRC_IPV4_ID_MAP".to_string()))?
            .try_into()?;

        let dst_ipv4_id_map: AyaLpmTrie<_, u32, u32> = ebpf
            .take_map("DST_IPV4_ID_MAP")
            .ok_or_else(|| IdentityError::MapNotFound("DST_IPV4_ID_MAP".to_string()))?
            .try_into()?;

        let src_ipv6_id_map: AyaLpmTrie<_, [u8; 16], u32> = ebpf
            .take_map("SRC_IPV6_ID_MAP")
            .ok_or_else(|| IdentityError::MapNotFound("SRC_IPV6_ID_MAP".to_string()))?
            .try_into()?;

        let dst_ipv6_id_map: AyaLpmTrie<_, [u8; 16], u32> = ebpf
            .take_map("DST_IPV6_ID_MAP")
            .ok_or_else(|| IdentityError::MapNotFound("DST_IPV6_ID_MAP".to_string()))?
            .try_into()?;

        Ok(Self {
            src_ipv4_id_map,
            dst_ipv4_id_map,
            src_ipv6_id_map,
            dst_ipv6_id_map,
        })
    }

    fn insert(&mut self, entry: &CidrEntry, id: u32) -> Result<(), IdentityError> {
        match entry.network {
            IpAddr::V4(ipv4) => {
                let ip = u32::from_be_bytes(ipv4.octets());
                let key = Key::new(entry.prefix_len as u32, ip);
                self.src_ipv4_id_map.insert(&key, id, 0)?;
                self.dst_ipv4_id_map.insert(&key, id, 0)?;
            }
            IpAddr::V6(ipv6) => {
                let ip = ipv6.octets();
                let key = Key::new(entry.prefix_len as u32, ip);
                self.src_ipv6_id_map.insert(&key, id, 0)?;
                self.dst_ipv6_id_map.insert(&key, id, 0)?;
            }
        }
        Ok(())
    }

    fn remove(&mut self, entry: &CidrEntry) {
        match entry.network {
            IpAddr::V4(ipv4) => {
                let ip = u32::from_be_bytes(ipv4.octets());
                let key = Key::new(entry.prefix_len as u32, ip);
                let _ = self.src_ipv4_id_map.remove(&key);
                let _ = self.dst_ipv4_id_map.remove(&key);
            }
            IpAddr::V6(ipv6) => {
                let ip = ipv6.octets();
                let key = Key::new(entry.prefix_len as u32, ip);
                let _ = self.src_ipv6_id_map.remove(&key);
                let _ = self.dst_ipv6_id_map.remove(&key);
            }
        }
    }
}

fn parse_cidr(cidr: &str) -> Result<CidrEntry, IdentityError> {
    let parts: Vec<&str> = cidr.split('/').collect();
    if parts.len() != 2 {
        return Err(IdentityError::InvalidCidr(cidr.to_string()));
    }

    let network: IpAddr = parts[0]
        .parse()
        .map_err(|_| IdentityError::InvalidIp(parts[0].to_string()))?;

    let prefix_len: u8 = parts[1]
        .parse()
        .map_err(|_| IdentityError::InvalidCidr(cidr.to_string()))?;

    let max_prefix = if network.is_ipv4() { 32 } else { 128 };
    if prefix_len > max_prefix {
        return Err(IdentityError::InvalidCidr(cidr.to_string()));
    }

    Ok(CidrEntry { network, prefix_len })
}

pub fn parse_single_ip(ip: &str) -> Result<CidrEntry, IdentityError> {
    let addr: IpAddr = ip
        .parse()
        .map_err(|_| IdentityError::InvalidIp(ip.to_string()))?;

    let prefix_len = if addr.is_ipv4() { 32 } else { 128 };
    Ok(CidrEntry { network: addr, prefix_len })
}
