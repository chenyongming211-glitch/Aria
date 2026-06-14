use std::collections::HashMap;
use std::collections::HashSet;
use std::net::IpAddr;
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
    group_to_id: HashMap<String, u32>,
    group_to_cidrs: HashMap<String, Vec<CidrEntry>>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct RuntimeIPGroup {
    pub key: String,
    pub cidrs: Vec<String>,
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
            group_to_id: HashMap::new(),
            group_to_cidrs: HashMap::new(),
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

    pub fn replace_groups(
        &mut self,
        groups: &[RuntimeIPGroup],
    ) -> Result<HashMap<String, u32>, IdentityError> {
        let mut normalized_groups = Vec::with_capacity(groups.len());
        let mut seen_entries: HashMap<CidrEntry, String> = HashMap::new();
        for group in groups {
            let key = group.key.trim().to_string();
            if key.is_empty() {
                return Err(IdentityError::InvalidCidr("empty group key".to_string()));
            }
            let mut cidrs = normalize_group_cidrs(&group.cidrs)?;
            cidrs.sort_by(|a, b| {
                format!("{}/{}", a.network, a.prefix_len)
                    .cmp(&format!("{}/{}", b.network, b.prefix_len))
            });
            cidrs.dedup();
            for entry in &cidrs {
                if let Some(existing_key) = seen_entries.get(entry) {
                    if existing_key != &key {
                        return Err(IdentityError::InvalidCidr(format!(
                            "duplicate CIDR {}/{} in runtime groups {} and {}",
                            entry.network, entry.prefix_len, existing_key, key
                        )));
                    }
                }
                seen_entries.insert(entry.clone(), key.clone());
            }
            normalized_groups.push((key, cidrs));
        }

        let desired_keys: HashSet<String> = normalized_groups
            .iter()
            .map(|(key, _)| key.clone())
            .collect();
        let stale_keys: Vec<String> = self
            .group_to_id
            .keys()
            .filter(|key| !desired_keys.contains(*key))
            .cloned()
            .collect();
        for key in stale_keys {
            self.remove_group(&key);
        }

        let mut result = HashMap::new();
        for (key, cidrs) in normalized_groups {
            let id = self.assign_group_id(&key);
            self.replace_group_cidrs(&key, id, &cidrs)?;
            result.insert(key, id);
        }
        Ok(result)
    }

    fn assign_group_id(&mut self, key: &str) -> u32 {
        if let Some(id) = self.group_to_id.get(key) {
            return *id;
        }
        let id = ID_COUNTER.fetch_add(1, Ordering::SeqCst);
        self.group_to_id.insert(key.to_string(), id);
        id
    }

    fn replace_group_cidrs(
        &mut self,
        key: &str,
        id: u32,
        cidrs: &[CidrEntry],
    ) -> Result<(), IdentityError> {
        let old = self.group_to_cidrs.remove(key).unwrap_or_default();
        for entry in &old {
            if !cidrs.contains(entry) {
                for map_set in &mut self.map_sets {
                    map_set.remove(entry);
                }
                self.cidr_to_id.remove(entry);
            }
        }

        for entry in cidrs {
            if let Some(existing_id) = self.cidr_to_id.get(entry) {
                let existing_id = *existing_id;
                if existing_id != id {
                    if self.group_owns_id(existing_id) {
                        return Err(IdentityError::InvalidCidr(format!(
                            "CIDR {}/{} already belongs to runtime group {}",
                            entry.network, entry.prefix_len, existing_id
                        )));
                    }
                    self.remove_standalone_cidr(entry, existing_id);
                }
            }
            for map_set in &mut self.map_sets {
                map_set.insert(entry, id)?;
            }
            self.cidr_to_id.insert(entry.clone(), id);
        }
        self.group_to_cidrs.insert(key.to_string(), cidrs.to_vec());
        Ok(())
    }

    fn remove_group(&mut self, key: &str) {
        let group_id = self.group_to_id.get(key).copied();
        let cidrs = self.group_to_cidrs.remove(key).unwrap_or_default();
        for entry in &cidrs {
            let mapped_to_group = match group_id {
                Some(id) => self.cidr_to_id.get(entry) == Some(&id),
                None => true,
            };
            if mapped_to_group {
                for map_set in &mut self.map_sets {
                    map_set.remove(entry);
                }
                self.cidr_to_id.remove(entry);
            }
        }
        self.group_to_id.remove(key);
    }

    fn group_owns_id(&self, id: u32) -> bool {
        self.group_to_id.values().any(|group_id| *group_id == id)
    }

    fn remove_standalone_cidr(&mut self, entry: &CidrEntry, id: u32) {
        for map_set in &mut self.map_sets {
            map_set.remove(entry);
        }
        self.cidr_to_id.remove(entry);
        self.id_to_cidr.remove(&id);
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

fn normalize_group_cidrs(cidrs: &[String]) -> Result<Vec<CidrEntry>, IdentityError> {
    let mut entries = Vec::with_capacity(cidrs.len());
    for cidr in cidrs {
        let trimmed = cidr.trim();
        if trimmed.is_empty() {
            continue;
        }
        entries.push(parse_cidr(trimmed)?);
    }
    if entries.is_empty() {
        return Err(IdentityError::InvalidCidr(
            "runtime IP group requires at least one CIDR".to_string(),
        ));
    }
    Ok(entries)
}

pub fn parse_single_ip(ip: &str) -> Result<CidrEntry, IdentityError> {
    let addr: IpAddr = ip
        .parse()
        .map_err(|_| IdentityError::InvalidIp(ip.to_string()))?;

    let prefix_len = if addr.is_ipv4() { 32 } else { 128 };
    Ok(CidrEntry { network: addr, prefix_len })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_identity_manager() -> IdentityManager {
        IdentityManager {
            map_sets: Vec::new(),
            cidr_to_id: HashMap::new(),
            id_to_cidr: HashMap::new(),
            group_to_id: HashMap::new(),
            group_to_cidrs: HashMap::new(),
        }
    }

    #[test]
    fn replace_groups_migrates_existing_standalone_cidr() {
        let mut identity = test_identity_manager();
        let standalone_id = identity
            .assign_id("100.64.0.27/32")
            .expect("standalone CIDR id");

        let group_ids = identity
            .replace_groups(&[RuntimeIPGroup {
                key: "inline-group".to_string(),
                cidrs: vec!["100.64.0.27/32".to_string()],
            }])
            .expect("replace groups should migrate standalone CIDR");
        let group_id = *group_ids.get("inline-group").expect("group id");

        assert_ne!(standalone_id, group_id);
        assert_eq!(identity.get_id("100.64.0.27/32"), Some(group_id));
        assert!(!identity.id_to_cidr.contains_key(&standalone_id));
    }

    #[test]
    fn replace_groups_rejects_duplicate_cidr_across_product_groups() {
        let mut identity = test_identity_manager();

        let err = identity
            .replace_groups(&[
                RuntimeIPGroup {
                    key: "group-a".to_string(),
                    cidrs: vec!["100.64.0.27/32".to_string()],
                },
                RuntimeIPGroup {
                    key: "group-b".to_string(),
                    cidrs: vec!["100.64.0.27/32".to_string()],
                },
            ])
            .expect_err("duplicate product group CIDR should fail");

        assert!(format!("{err}").contains("duplicate CIDR"));
    }
}
