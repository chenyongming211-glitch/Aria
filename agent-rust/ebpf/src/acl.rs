#![no_std]
#![no_main]

use core::mem::MaybeUninit;

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

macro_rules! policy_key_ref {
    ($slot:expr, $generation:expr, $src_id:expr, $dst_id:expr, $proto:expr, $direction:expr) => {{
        let ptr = $slot.as_mut_ptr();
        unsafe {
            core::ptr::write_volatile(core::ptr::addr_of_mut!((*ptr).tap_id), TAP_ID_UNASSIGNED);
            core::ptr::write_volatile(core::ptr::addr_of_mut!((*ptr).generation), $generation);
            core::ptr::write_volatile(core::ptr::addr_of_mut!((*ptr).src_id), $src_id);
            core::ptr::write_volatile(core::ptr::addr_of_mut!((*ptr).dst_id), $dst_id);
            core::ptr::write_volatile(core::ptr::addr_of_mut!((*ptr).proto), $proto);
            core::ptr::write_volatile(core::ptr::addr_of_mut!((*ptr).direction), $direction);
            core::ptr::write_volatile(core::ptr::addr_of_mut!((*ptr).pad[0]), 0);
            core::ptr::write_volatile(core::ptr::addr_of_mut!((*ptr).pad[1]), 0);
            &*ptr
        }
    }};
}

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

    if let Ok(eth) = ptr_at::<EthHdr>(&ctx, 0) {
        match u16::from_be(eth.ether_type) {
            ETH_P_IP => return try_xdp_ingress_acl_ipv4(&ctx, EthHdr::LEN),
            ETH_P_IPV6 => return try_xdp_ingress_acl_ipv6(&ctx, EthHdr::LEN),
            _ => {}
        }
    }

    if let Ok(ip) = ptr_at::<Ipv4Hdr>(&ctx, 0) {
        if ip.version() == IP_VERSION_4 {
            return try_xdp_ingress_acl_ipv4(&ctx, 0);
        }
    }

    if let Ok(ip) = ptr_at::<Ipv6Hdr>(&ctx, 0) {
        if ip.version() == IP_VERSION_6 {
            return try_xdp_ingress_acl_ipv6(&ctx, 0);
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
fn try_xdp_ingress_acl_ipv4(ctx: &XdpContext, ip_offset: usize) -> Result<u32, u64> {
    let ip = ptr_at::<Ipv4Hdr>(ctx, ip_offset)?;
    let pkt_len = ipv4_packet_len(ip, ip_offset);
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
fn try_xdp_ingress_acl_ipv6(ctx: &XdpContext, ip_offset: usize) -> Result<u32, u64> {
    let ip = ptr_at::<Ipv6Hdr>(ctx, ip_offset)?;
    let pkt_len = ipv6_packet_len(ip, ip_offset);
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
    let mut found: u8 = 0;
    let mut best_priority: u16 = u16::MAX;
    let mut best_action: u8 = ACTION_ALLOW;
    let mut best_has_port_filter: u8 = 0;
    let mut best_bitmap_idx: u32 = 0;
    let mut best_generation: u32 = generation;
    let mut best_src_id: u32 = ID_WILDCARD;
    let mut best_dst_id: u32 = ID_WILDCARD;
    let mut best_proto: u8 = PROTO_WILDCARD;
    let mut best_direction: u8 = direction;

    macro_rules! consider_candidate {
        ($candidate_key:expr) => {{
            if let Some(policy) = unsafe { POLICY_TABLE.get($candidate_key) } {
                if found == 0 || policy.priority < best_priority {
                    found = 1;
                    best_priority = policy.priority;
                    best_action = policy.action;
                    best_has_port_filter = policy.has_port_filter;
                    best_bitmap_idx = policy.bitmap_idx;
                    best_generation = (*$candidate_key).generation;
                    best_src_id = (*$candidate_key).src_id;
                    best_dst_id = (*$candidate_key).dst_id;
                    best_proto = (*$candidate_key).proto;
                    best_direction = (*$candidate_key).direction;
                }
            }
        }};
    }

    macro_rules! lookup_candidates_for_proto {
        ($candidate_proto:expr) => {{
            let mut exact_key = MaybeUninit::<PolicyKey>::uninit();
            let exact_key_ref =
                policy_key_ref!(exact_key, generation, src_id, dst_id, $candidate_proto, direction);
            consider_candidate!(exact_key_ref);

            let mut wildcard_src_key = MaybeUninit::<PolicyKey>::uninit();
            let wildcard_src_key_ref = policy_key_ref!(
                wildcard_src_key,
                generation,
                ID_WILDCARD,
                dst_id,
                $candidate_proto,
                direction
            );
            consider_candidate!(wildcard_src_key_ref);

            let mut wildcard_dst_key = MaybeUninit::<PolicyKey>::uninit();
            let wildcard_dst_key_ref = policy_key_ref!(
                wildcard_dst_key,
                generation,
                src_id,
                ID_WILDCARD,
                $candidate_proto,
                direction
            );
            consider_candidate!(wildcard_dst_key_ref);

            let mut full_wildcard_key = MaybeUninit::<PolicyKey>::uninit();
            let full_wildcard_key_ref = policy_key_ref!(
                full_wildcard_key,
                generation,
                ID_WILDCARD,
                ID_WILDCARD,
                $candidate_proto,
                direction
            );
            consider_candidate!(full_wildcard_key_ref);
        }};
    }

    lookup_candidates_for_proto!(proto);
    if proto != PROTO_WILDCARD {
        lookup_candidates_for_proto!(PROTO_WILDCARD);
    }

    if found != 0 {
        let action = policy_action(
            generation,
            best_action,
            best_has_port_filter,
            best_bitmap_idx,
            dst_port,
        );
        let mut stats_key = MaybeUninit::<PolicyKey>::uninit();
        let stats_key_ref = policy_key_ref!(
            stats_key,
            best_generation,
            best_src_id,
            best_dst_id,
            best_proto,
            best_direction
        );
        update_rule_stats(stats_key_ref, pkt_len, action == ACTION_DROP);
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

#[inline(always)]
fn policy_action(
    generation: u32,
    action: u8,
    has_port_filter: u8,
    bitmap_idx: u32,
    dst_port: u16,
) -> u8 {
    if has_port_filter == 0 {
        return action;
    }

    if dst_port == 0 {
        return action;
    }

    let port_key = PortKey {
        tap_id: TAP_ID_UNASSIGNED,
        generation,
        idx: bitmap_idx,
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

    action
}

#[inline(always)]
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
fn ipv4_packet_len(ip: &Ipv4Hdr, ip_offset: usize) -> u64 {
    ip_offset as u64 + u16::from_be_bytes(ip.tot_len) as u64
}

#[inline(always)]
fn ipv6_packet_len(ip: &Ipv6Hdr, ip_offset: usize) -> u64 {
    ip_offset as u64 + Ipv6Hdr::LEN as u64 + u16::from_be_bytes(ip.payload_len) as u64
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
