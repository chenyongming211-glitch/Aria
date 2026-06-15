#![no_std]
#![no_main]

use aya_ebpf::{
    macros::{classifier, map, xdp},
    maps::{lpm_trie::Key, HashMap, LpmTrie, PerCpuHashMap},
    programs::{TcContext, XdpContext},
};
use network_types::{
    eth::EthHdr,
    ip::{IpProto, Ipv4Hdr, Ipv6Hdr},
    tcp::TcpHdr,
    udp::UdpHdr,
};

const XDP_PASS: u32 = 2;
const XDP_DROP: u32 = 1;
const TC_ACT_UNSPEC: i32 = -1;
const TC_ACT_OK: i32 = 0;
const TC_ACT_SHOT: i32 = 2;
const ETH_P_IP: u16 = 0x0800;
const ETH_P_IPV6: u16 = 0x86DD;
const IP_VERSION_4: u8 = 4;
const IP_VERSION_6: u8 = 6;

const TAP_ID_UNASSIGNED: u32 = 0;
const ID_WILDCARD: u32 = 0;
const PROTO_WILDCARD: u8 = 0;
const DIRECTION_INGRESS: u8 = 0;
const DIRECTION_EGRESS: u8 = 1;

const ACTION_ALLOW: u8 = 0;
const ACTION_DROP: u8 = 1;
const PORT_ACTION_DROP: u8 = 1;
const PORT_ACTION_PASS: u8 = 2;
const IPV4_IDENTITY_LOOKUP_BITS: u32 = 64;
const IPV6_IDENTITY_LOOKUP_BITS: u32 = 160;

type Ipv4IdentityKey = [u8; 8];
type Ipv6IdentityKey = [u8; 20];

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PolicyKey {
    pub tap_id: u32,
    pub generation: u32,
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
    pub priority: u16,
    pub bitmap_idx: u32,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PortKey {
    pub tap_id: u32,
    pub generation: u32,
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
    pub policy_generation: u32,
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
    pub policy_generation: u32,
}

#[map(name = "SRC_IPV4_ID_MAP", pin)]
static SRC_IPV4_ID_MAP: LpmTrie<Ipv4IdentityKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV4_ID_MAP", pin)]
static DST_IPV4_ID_MAP: LpmTrie<Ipv4IdentityKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "SRC_IPV6_ID_MAP", pin)]
static SRC_IPV6_ID_MAP: LpmTrie<Ipv6IdentityKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV6_ID_MAP", pin)]
static DST_IPV6_ID_MAP: LpmTrie<Ipv6IdentityKey, u32> = LpmTrie::with_max_entries(10000, 0);

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

#[classifier]
pub fn tc_egress_acl(ctx: TcContext) -> i32 {
    match try_tc_egress_acl(ctx) {
        Ok(ret) => ret,
        Err(_) => TC_ACT_UNSPEC,
    }
}

fn try_xdp_ingress_acl(ctx: XdpContext) -> Result<u32, u64> {
    let tap_id = TAP_ID_UNASSIGNED;
    if !acl_enabled(tap_id) {
        return Ok(XDP_PASS);
    }

    let pkt_len = (ctx.data_end() - ctx.data()) as u64;

    if let Ok(eth) = ptr_at::<EthHdr>(&ctx, 0) {
        match u16::from_be(eth.ether_type) {
            ETH_P_IP => return try_xdp_ingress_acl_ipv4(&ctx, pkt_len, EthHdr::LEN),
            ETH_P_IPV6 => return try_xdp_ingress_acl_ipv6(&ctx, pkt_len, EthHdr::LEN),
            _ => {}
        }
    }

    if let Ok(ip) = ptr_at::<Ipv4Hdr>(&ctx, 0) {
        if ip.version() == IP_VERSION_4 {
            return try_xdp_ingress_acl_ipv4(&ctx, pkt_len, 0);
        }
    }

    if let Ok(ip) = ptr_at::<Ipv6Hdr>(&ctx, 0) {
        if ip.version() == IP_VERSION_6 {
            return try_xdp_ingress_acl_ipv6(&ctx, pkt_len, 0);
        }
    }

    Ok(XDP_PASS)
}

fn try_tc_egress_acl(ctx: TcContext) -> Result<i32, u64> {
    let tap_id = TAP_ID_UNASSIGNED;
    if !acl_enabled(tap_id) {
        return Ok(TC_ACT_UNSPEC);
    }

    let pkt_len = ctx.skb.len() as u64;

    if let Ok(eth) = ptr_at_tc::<EthHdr>(&ctx, 0) {
        match u16::from_be(eth.ether_type) {
            ETH_P_IP => return try_tc_egress_acl_ipv4(&ctx, pkt_len, EthHdr::LEN),
            ETH_P_IPV6 => return try_tc_egress_acl_ipv6(&ctx, pkt_len, EthHdr::LEN),
            _ => {}
        }
    }

    if let Ok(ip) = ptr_at_tc::<Ipv4Hdr>(&ctx, 0) {
        if ip.version() == IP_VERSION_4 {
            return try_tc_egress_acl_ipv4(&ctx, pkt_len, 0);
        }
    }

    if let Ok(ip) = ptr_at_tc::<Ipv6Hdr>(&ctx, 0) {
        if ip.version() == IP_VERSION_6 {
            return try_tc_egress_acl_ipv6(&ctx, pkt_len, 0);
        }
    }

    Ok(TC_ACT_UNSPEC)
}

#[inline(always)]
fn try_xdp_ingress_acl_ipv4(ctx: &XdpContext, pkt_len: u64, ip_offset: usize) -> Result<u32, u64> {
    let ip = ptr_at::<Ipv4Hdr>(ctx, ip_offset)?;
    let (_, dst_port) = parse_ports_ipv4(ctx, ip_offset, ip)?;
    let generation = active_policy_generation(TAP_ID_UNASSIGNED);
    let src_id = lookup_ipv4_id(
        &SRC_IPV4_ID_MAP,
        generation,
        u32::from_be_bytes(ip.src_addr),
    );
    let dst_id = lookup_ipv4_id(
        &DST_IPV4_ID_MAP,
        generation,
        u32::from_be_bytes(ip.dst_addr),
    );
    let action = acl_policy_action(
        generation,
        src_id,
        dst_id,
        ip.proto as u8,
        dst_port,
        pkt_len,
        DIRECTION_INGRESS,
    );
    Ok(if action == ACTION_DROP {
        XDP_DROP
    } else {
        XDP_PASS
    })
}

#[inline(always)]
fn try_xdp_ingress_acl_ipv6(ctx: &XdpContext, pkt_len: u64, ip_offset: usize) -> Result<u32, u64> {
    let ip = ptr_at::<Ipv6Hdr>(ctx, ip_offset)?;
    let (_, dst_port) = parse_ports_ipv6(ctx, ip_offset, ip)?;
    let generation = active_policy_generation(TAP_ID_UNASSIGNED);
    let src_id = lookup_ipv6_id(&SRC_IPV6_ID_MAP, generation, ip.src_addr);
    let dst_id = lookup_ipv6_id(&DST_IPV6_ID_MAP, generation, ip.dst_addr);
    let action = acl_policy_action(
        generation,
        src_id,
        dst_id,
        ip.next_hdr as u8,
        dst_port,
        pkt_len,
        DIRECTION_INGRESS,
    );
    Ok(if action == ACTION_DROP {
        XDP_DROP
    } else {
        XDP_PASS
    })
}

#[inline(always)]
fn try_tc_egress_acl_ipv4(ctx: &TcContext, pkt_len: u64, ip_offset: usize) -> Result<i32, u64> {
    let ip = ptr_at_tc::<Ipv4Hdr>(ctx, ip_offset)?;
    let (_, dst_port) = parse_ports_ipv4_tc(ctx, ip_offset, ip)?;
    let generation = active_policy_generation(TAP_ID_UNASSIGNED);
    let src_id = lookup_ipv4_id(
        &SRC_IPV4_ID_MAP,
        generation,
        u32::from_be_bytes(ip.src_addr),
    );
    let dst_id = lookup_ipv4_id(
        &DST_IPV4_ID_MAP,
        generation,
        u32::from_be_bytes(ip.dst_addr),
    );
    let action = acl_policy_action(
        generation,
        src_id,
        dst_id,
        ip.proto as u8,
        dst_port,
        pkt_len,
        DIRECTION_EGRESS,
    );
    Ok(if action == ACTION_DROP {
        TC_ACT_SHOT
    } else {
        TC_ACT_UNSPEC
    })
}

#[inline(always)]
fn try_tc_egress_acl_ipv6(ctx: &TcContext, pkt_len: u64, ip_offset: usize) -> Result<i32, u64> {
    let ip = ptr_at_tc::<Ipv6Hdr>(ctx, ip_offset)?;
    let (_, dst_port) = parse_ports_ipv6_tc(ctx, ip_offset, ip)?;
    let generation = active_policy_generation(TAP_ID_UNASSIGNED);
    let src_id = lookup_ipv6_id(&SRC_IPV6_ID_MAP, generation, ip.src_addr);
    let dst_id = lookup_ipv6_id(&DST_IPV6_ID_MAP, generation, ip.dst_addr);
    let action = acl_policy_action(
        generation,
        src_id,
        dst_id,
        ip.next_hdr as u8,
        dst_port,
        pkt_len,
        DIRECTION_EGRESS,
    );
    Ok(if action == ACTION_DROP {
        TC_ACT_SHOT
    } else {
        TC_ACT_UNSPEC
    })
}

#[inline(always)]
fn acl_policy_action(
    generation: u32,
    src_id: u32,
    dst_id: u32,
    proto: u8,
    dst_port: u16,
    pkt_len: u64,
    direction: u8,
) -> u8 {
    if let Some((key, policy)) =
        lookup_policy(TAP_ID_UNASSIGNED, generation, src_id, dst_id, proto, direction)
    {
        let action = policy_action(TAP_ID_UNASSIGNED, generation, policy, dst_port);
        update_rule_stats(&key, pkt_len, action == ACTION_DROP);
        return action;
    }

    ACTION_ALLOW
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

fn active_policy_generation(tap_id: u32) -> u32 {
    if let Some(tap) = unsafe { TAP_CONFIG_MAP.get(&tap_id) } {
        return tap.policy_generation;
    }

    let key = 0;
    if let Some(config) = unsafe { FIREWALL_CONFIG.get(&key) } {
        return config.policy_generation;
    }

    0
}

fn lookup_policy(
    tap_id: u32,
    generation: u32,
    src_id: u32,
    dst_id: u32,
    proto: u8,
    direction: u8,
) -> Option<(PolicyKey, PolicyValue)> {
    let mut best = lookup_policy_for_proto(tap_id, generation, src_id, dst_id, proto, direction);

    if proto != PROTO_WILDCARD {
        if let Some(policy) = lookup_policy_for_proto(
            tap_id,
            generation,
            src_id,
            dst_id,
            PROTO_WILDCARD,
            direction,
        )
        {
            best = choose_policy(best, policy);
        }
    }

    best
}

fn lookup_policy_for_proto(
    tap_id: u32,
    generation: u32,
    src_id: u32,
    dst_id: u32,
    proto: u8,
    direction: u8,
) -> Option<(PolicyKey, PolicyValue)> {
    let mut best: Option<(PolicyKey, PolicyValue)> = None;
    let exact_key = PolicyKey {
        tap_id,
        generation,
        src_id,
        dst_id,
        proto,
        direction,
        pad: [0; 2],
    };
    if let Some(policy) = unsafe { POLICY_TABLE.get(&exact_key) } {
        best = choose_policy(best, (exact_key, *policy));
    }

    let wildcard_src_key = PolicyKey {
        tap_id,
        generation,
        src_id: ID_WILDCARD,
        dst_id,
        proto,
        direction,
        pad: [0; 2],
    };
    if let Some(policy) = unsafe { POLICY_TABLE.get(&wildcard_src_key) } {
        best = choose_policy(best, (wildcard_src_key, *policy));
    }

    let wildcard_dst_key = PolicyKey {
        tap_id,
        generation,
        src_id,
        dst_id: ID_WILDCARD,
        proto,
        direction,
        pad: [0; 2],
    };
    if let Some(policy) = unsafe { POLICY_TABLE.get(&wildcard_dst_key) } {
        best = choose_policy(best, (wildcard_dst_key, *policy));
    }

    let full_wildcard_key = PolicyKey {
        tap_id,
        generation,
        src_id: ID_WILDCARD,
        dst_id: ID_WILDCARD,
        proto,
        direction,
        pad: [0; 2],
    };
    if let Some(policy) = unsafe { POLICY_TABLE.get(&full_wildcard_key) } {
        best = choose_policy(best, (full_wildcard_key, *policy));
    }

    best
}

fn choose_policy(
    current: Option<(PolicyKey, PolicyValue)>,
    candidate: (PolicyKey, PolicyValue),
) -> Option<(PolicyKey, PolicyValue)> {
    match current {
        Some(existing) if existing.1.priority <= candidate.1.priority => Some(existing),
        _ => Some(candidate),
    }
}

fn policy_action(tap_id: u32, generation: u32, policy: PolicyValue, dst_port: u16) -> u8 {
    if policy.has_port_filter == 0 {
        return policy.action;
    }

    if dst_port == 0 {
        return policy.action;
    }

    let port_key = PortKey {
        tap_id,
        generation,
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

fn lookup_ipv4_id(map: &LpmTrie<Ipv4IdentityKey, u32>, generation: u32, ip: u32) -> u32 {
    let key = Key::new(IPV4_IDENTITY_LOOKUP_BITS, ipv4_identity_key(generation, ip));
    if let Some(id) = map.get(&key) {
        return *id;
    }
    ID_WILDCARD
}

fn lookup_ipv6_id(map: &LpmTrie<Ipv6IdentityKey, u32>, generation: u32, ip: [u8; 16]) -> u32 {
    let key = Key::new(IPV6_IDENTITY_LOOKUP_BITS, ipv6_identity_key(generation, ip));
    if let Some(id) = map.get(&key) {
        return *id;
    }
    ID_WILDCARD
}

fn ipv4_identity_key(generation: u32, ip: u32) -> Ipv4IdentityKey {
    let generation_bytes = generation.to_be_bytes();
    let ip_bytes = ip.to_be_bytes();
    [
        generation_bytes[0],
        generation_bytes[1],
        generation_bytes[2],
        generation_bytes[3],
        ip_bytes[0],
        ip_bytes[1],
        ip_bytes[2],
        ip_bytes[3],
    ]
}

fn ipv6_identity_key(generation: u32, ip: [u8; 16]) -> Ipv6IdentityKey {
    let generation_bytes = generation.to_be_bytes();
    [
        generation_bytes[0],
        generation_bytes[1],
        generation_bytes[2],
        generation_bytes[3],
        ip[0],
        ip[1],
        ip[2],
        ip[3],
        ip[4],
        ip[5],
        ip[6],
        ip[7],
        ip[8],
        ip[9],
        ip[10],
        ip[11],
        ip[12],
        ip[13],
        ip[14],
        ip[15],
    ]
}

fn parse_ports_ipv4(ctx: &XdpContext, ip_offset: usize, ip: &Ipv4Hdr) -> Result<(u16, u16), u64> {
    match IpProto::from(ip.proto) {
        IpProto::Tcp => {
            let tcp = ptr_at::<TcpHdr>(ctx, ip_offset + Ipv4Hdr::LEN)?;
            Ok((u16::from_be_bytes(tcp.source), u16::from_be_bytes(tcp.dest)))
        }
        IpProto::Udp => {
            let udp = ptr_at::<UdpHdr>(ctx, ip_offset + Ipv4Hdr::LEN)?;
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

fn parse_ports_ipv4_tc(ctx: &TcContext, ip_offset: usize, ip: &Ipv4Hdr) -> Result<(u16, u16), u64> {
    match IpProto::from(ip.proto) {
        IpProto::Tcp => {
            let tcp = ptr_at_tc::<TcpHdr>(ctx, ip_offset + Ipv4Hdr::LEN)?;
            Ok((u16::from_be_bytes(tcp.source), u16::from_be_bytes(tcp.dest)))
        }
        IpProto::Udp => {
            let udp = ptr_at_tc::<UdpHdr>(ctx, ip_offset + Ipv4Hdr::LEN)?;
            Ok((u16::from_be_bytes(udp.src), u16::from_be_bytes(udp.dst)))
        }
        _ => Ok((0, 0)),
    }
}

fn parse_ports_ipv6_tc(ctx: &TcContext, ip_offset: usize, ip: &Ipv6Hdr) -> Result<(u16, u16), u64> {
    match IpProto::from(ip.next_hdr) {
        IpProto::Tcp => {
            let tcp = ptr_at_tc::<TcpHdr>(ctx, ip_offset + Ipv6Hdr::LEN)?;
            Ok((u16::from_be_bytes(tcp.source), u16::from_be_bytes(tcp.dest)))
        }
        IpProto::Udp => {
            let udp = ptr_at_tc::<UdpHdr>(ctx, ip_offset + Ipv6Hdr::LEN)?;
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

#[inline(always)]
fn ptr_at_tc<T>(ctx: &TcContext, offset: usize) -> Result<&T, u64> {
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
