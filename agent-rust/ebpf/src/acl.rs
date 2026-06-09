#![no_std]
#![no_main]

use aya_ebpf::{
    macros::{map, xdp},
    maps::{lpm_trie::Key, HashMap, LpmTrie, PerCpuHashMap},
    programs::XdpContext,
};
use network_types::{
    eth::EthHdr,
    ip::{IpProto, Ipv4Hdr, Ipv6Hdr},
    tcp::TcpHdr,
    udp::UdpHdr,
};

const XDP_PASS: u32 = 2;
const XDP_DROP: u32 = 1;
const ETH_P_IP: u16 = 0x0800;
const ETH_P_IPV6: u16 = 0x86DD;
const IP_VERSION_4: u8 = 4;
const IP_VERSION_6: u8 = 6;

const TAP_ID_UNASSIGNED: u32 = 0;
const ID_WILDCARD: u32 = 0;
const PROTO_WILDCARD: u8 = 0;
const DIRECTION_INGRESS: u8 = 0;

const ACTION_ALLOW: u8 = 0;
const ACTION_DROP: u8 = 1;
const PORT_ACTION_DROP: u8 = 1;
const PORT_ACTION_PASS: u8 = 2;

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PolicyKey {
    pub tap_id: u32,
    pub src_id: u32,
    pub dst_id: u32,
    pub proto: u8,
    pub direction: u8,
    pub pad: [u8; 2],
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct PolicyValue {
    pub action: u8,
    pub has_port_filter: u8,
    pub pad1: [u8; 2],
    pub bitmap_idx: u32,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PortKey {
    pub tap_id: u32,
    pub idx: u32,
    pub port: u16,
    pub pad: u16,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct RuleStatsValue {
    pub packets: u64,
    pub bytes: u64,
    pub dropped_packets: u64,
    pub dropped_bytes: u64,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct FirewallConfig {
    pub conntrack_enabled: u8,
    pub monitoring_enabled: u8,
    pub num_cpus: u16,
    pub qos_enabled: u8,
    pub acl_enabled: u8,
    pub mirror_enabled: u8,
    pub tcprt_enabled: u8,
    pub ssl_enabled: u8,
    pub lb_enabled: u8,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct TapConfig {
    pub conntrack_enabled: u8,
    pub monitoring_enabled: u8,
    pub acl_enabled: u8,
    pub qos_enabled: u8,
    pub mirror_enabled: u8,
    pub tcprt_enabled: u8,
    pub lb_enabled: u8,
    pub pad: [u8; 1],
}

#[map(name = "SRC_IPV4_ID_MAP", pin)]
static SRC_IPV4_ID_MAP: LpmTrie<u32, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV4_ID_MAP", pin)]
static DST_IPV4_ID_MAP: LpmTrie<u32, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "SRC_IPV6_ID_MAP", pin)]
static SRC_IPV6_ID_MAP: LpmTrie<[u8; 16], u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV6_ID_MAP", pin)]
static DST_IPV6_ID_MAP: LpmTrie<[u8; 16], u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "POLICY_TABLE", pin)]
static POLICY_TABLE: HashMap<PolicyKey, PolicyValue> = HashMap::with_max_entries(65536, 0);

#[map(name = "PORT_BITMAP_POOL", pin)]
static PORT_BITMAP_POOL: HashMap<PortKey, u8> = HashMap::with_max_entries(262144, 0);

#[map(name = "RULE_STATS", pin)]
static RULE_STATS: PerCpuHashMap<PolicyKey, RuleStatsValue> =
    PerCpuHashMap::with_max_entries(65536, 0);

#[map(name = "FIREWALL_CONFIG", pin)]
static FIREWALL_CONFIG: HashMap<u32, FirewallConfig> = HashMap::with_max_entries(1, 0);

#[map(name = "TAP_CONFIG_MAP", pin)]
static TAP_CONFIG_MAP: HashMap<u32, TapConfig> = HashMap::with_max_entries(1024, 0);

#[xdp]
pub fn xdp_ingress_acl(ctx: XdpContext) -> u32 {
    match try_xdp_ingress_acl(ctx) {
        Ok(ret) => ret,
        Err(_) => XDP_PASS,
    }
}

fn try_xdp_ingress_acl(ctx: XdpContext) -> Result<u32, u64> {
    let tap_id = TAP_ID_UNASSIGNED;
    if !acl_enabled(tap_id) {
        return Ok(XDP_PASS);
    }

    let pkt_len = (ctx.data_end() - ctx.data()) as u64;
    let (ip_offset, ip_version) = packet_ip_offset(&ctx)?;

    let (src_id, dst_id, proto, _src_port, dst_port) = match ip_version {
        IP_VERSION_4 => {
            let ip = ptr_at::<Ipv4Hdr>(&ctx, ip_offset)?;
            let (src_port, dst_port) = parse_ports_ipv4(&ctx, ip_offset, ip)?;
            let src_id = lookup_ipv4_id(&SRC_IPV4_ID_MAP, u32::from_be_bytes(ip.src_addr));
            let dst_id = lookup_ipv4_id(&DST_IPV4_ID_MAP, u32::from_be_bytes(ip.dst_addr));
            (src_id, dst_id, ip.proto as u8, src_port, dst_port)
        }
        IP_VERSION_6 => {
            let ip = ptr_at::<Ipv6Hdr>(&ctx, ip_offset)?;
            let (src_port, dst_port) = parse_ports_ipv6(&ctx, ip_offset, ip)?;
            let src_id = lookup_ipv6_id(&SRC_IPV6_ID_MAP, ip.src_addr);
            let dst_id = lookup_ipv6_id(&DST_IPV6_ID_MAP, ip.dst_addr);
            (src_id, dst_id, ip.next_hdr as u8, src_port, dst_port)
        }
        _ => return Ok(XDP_PASS),
    };

    if let Some((key, policy)) = lookup_policy(tap_id, src_id, dst_id, proto, DIRECTION_INGRESS) {
        let action = policy_action(tap_id, policy, dst_port);
        update_rule_stats(&key, pkt_len, action == ACTION_DROP);
        return Ok(if action == ACTION_DROP {
            XDP_DROP
        } else {
            XDP_PASS
        });
    }

    Ok(XDP_PASS)
}

fn packet_ip_offset(ctx: &XdpContext) -> Result<(usize, u8), u64> {
    if let Ok(eth) = ptr_at::<EthHdr>(ctx, 0) {
        match u16::from_be(eth.ether_type) {
            ETH_P_IP => return Ok((EthHdr::LEN, IP_VERSION_4)),
            ETH_P_IPV6 => return Ok((EthHdr::LEN, IP_VERSION_6)),
            _ => {}
        }
    }

    let first = *ptr_at::<u8>(ctx, 0)?;
    match first >> 4 {
        IP_VERSION_4 => Ok((0, IP_VERSION_4)),
        IP_VERSION_6 => Ok((0, IP_VERSION_6)),
        _ => Err(0),
    }
}

fn acl_enabled(tap_id: u32) -> bool {
    if let Some(tap) = unsafe { TAP_CONFIG_MAP.get(&tap_id) } {
        return tap.acl_enabled != 0;
    }

    let key = 0;
    if let Some(config) = unsafe { FIREWALL_CONFIG.get(&key) } {
        return config.acl_enabled != 0;
    }

    true
}

fn lookup_policy(
    tap_id: u32,
    src_id: u32,
    dst_id: u32,
    proto: u8,
    direction: u8,
) -> Option<(PolicyKey, PolicyValue)> {
    if let Some(policy) = lookup_policy_for_proto(tap_id, src_id, dst_id, proto, direction) {
        return Some(policy);
    }

    if proto != PROTO_WILDCARD {
        return lookup_policy_for_proto(tap_id, src_id, dst_id, PROTO_WILDCARD, direction);
    }

    None
}

fn lookup_policy_for_proto(
    tap_id: u32,
    src_id: u32,
    dst_id: u32,
    proto: u8,
    direction: u8,
) -> Option<(PolicyKey, PolicyValue)> {
    let exact_key = PolicyKey {
        tap_id,
        src_id,
        dst_id,
        proto,
        direction,
        pad: [0; 2],
    };
    if let Some(policy) = unsafe { POLICY_TABLE.get(&exact_key) } {
        return Some((exact_key, *policy));
    }

    let wildcard_src_key = PolicyKey {
        tap_id,
        src_id: ID_WILDCARD,
        dst_id,
        proto,
        direction,
        pad: [0; 2],
    };
    if let Some(policy) = unsafe { POLICY_TABLE.get(&wildcard_src_key) } {
        return Some((wildcard_src_key, *policy));
    }

    let wildcard_dst_key = PolicyKey {
        tap_id,
        src_id,
        dst_id: ID_WILDCARD,
        proto,
        direction,
        pad: [0; 2],
    };
    if let Some(policy) = unsafe { POLICY_TABLE.get(&wildcard_dst_key) } {
        return Some((wildcard_dst_key, *policy));
    }

    let full_wildcard_key = PolicyKey {
        tap_id,
        src_id: ID_WILDCARD,
        dst_id: ID_WILDCARD,
        proto,
        direction,
        pad: [0; 2],
    };
    if let Some(policy) = unsafe { POLICY_TABLE.get(&full_wildcard_key) } {
        return Some((full_wildcard_key, *policy));
    }

    None
}

fn policy_action(tap_id: u32, policy: PolicyValue, dst_port: u16) -> u8 {
    if policy.has_port_filter == 0 {
        return policy.action;
    }

    if dst_port == 0 {
        return policy.action;
    }

    let port_key = PortKey {
        tap_id,
        idx: policy.bitmap_idx,
        port: dst_port,
        pad: 0,
    };

    if let Some(action) = unsafe { PORT_BITMAP_POOL.get(&port_key) } {
        if *action == PORT_ACTION_DROP {
            return ACTION_DROP;
        }
        if *action == PORT_ACTION_PASS {
            return ACTION_ALLOW;
        }
    }

    policy.action
}

fn update_rule_stats(key: &PolicyKey, pkt_len: u64, dropped: bool) {
    if let Some(stats_ptr) = RULE_STATS.get_ptr_mut(key) {
        let stats = unsafe { &mut *stats_ptr };
        stats.packets = stats.packets.wrapping_add(1);
        stats.bytes = stats.bytes.wrapping_add(pkt_len);
        if dropped {
            stats.dropped_packets = stats.dropped_packets.wrapping_add(1);
            stats.dropped_bytes = stats.dropped_bytes.wrapping_add(pkt_len);
        }
    }
}

fn lookup_ipv4_id(map: &LpmTrie<u32, u32>, ip: u32) -> u32 {
    let key = Key::new(32, ip);
    if let Some(id) = map.get(&key) {
        return *id;
    }
    ID_WILDCARD
}

fn lookup_ipv6_id(map: &LpmTrie<[u8; 16], u32>, ip: [u8; 16]) -> u32 {
    let key = Key::new(128, ip);
    if let Some(id) = map.get(&key) {
        return *id;
    }
    ID_WILDCARD
}

fn parse_ports_ipv4(ctx: &XdpContext, ip_offset: usize, ip: &Ipv4Hdr) -> Result<(u16, u16), u64> {
    let ip_hdr_len = (ip.ihl() as usize) * 4;
    match IpProto::from(ip.proto) {
        IpProto::Tcp => {
            let tcp = ptr_at::<TcpHdr>(ctx, ip_offset + ip_hdr_len)?;
            Ok((u16::from_be_bytes(tcp.source), u16::from_be_bytes(tcp.dest)))
        }
        IpProto::Udp => {
            let udp = ptr_at::<UdpHdr>(ctx, ip_offset + ip_hdr_len)?;
            Ok((u16::from_be_bytes(udp.src), u16::from_be_bytes(udp.dst)))
        }
        _ => Ok((0, 0)),
    }
}

fn parse_ports_ipv6(ctx: &XdpContext, ip_offset: usize, ip: &Ipv6Hdr) -> Result<(u16, u16), u64> {
    match IpProto::from(ip.next_hdr) {
        IpProto::Tcp => {
            let tcp = ptr_at::<TcpHdr>(ctx, ip_offset + Ipv6Hdr::LEN)?;
            Ok((u16::from_be_bytes(tcp.source), u16::from_be_bytes(tcp.dest)))
        }
        IpProto::Udp => {
            let udp = ptr_at::<UdpHdr>(ctx, ip_offset + Ipv6Hdr::LEN)?;
            Ok((u16::from_be_bytes(udp.src), u16::from_be_bytes(udp.dst)))
        }
        _ => Ok((0, 0)),
    }
}

#[inline(always)]
fn ptr_at<T>(ctx: &XdpContext, offset: usize) -> Result<&T, u64> {
    let start = ctx.data();
    let end = ctx.data_end();
    let len = core::mem::size_of::<T>();

    if start + offset + len > end {
        return Err(0);
    }

    Ok(unsafe { &*((start + offset) as *const T) })
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
