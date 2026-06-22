#![no_std]
#![no_main]

use aya_ebpf::{
    bindings::__sk_buff,
    helpers::bpf_ktime_get_ns,
    macros::{classifier, map},
    maps::{lpm_trie::Key, HashMap, LpmTrie, PerCpuArray, PerCpuHashMap},
    programs::TcContext,
    EbpfContext,
};
use network_types::{
    eth::EthHdr,
    ip::{Ipv4Hdr, Ipv6Hdr},
};

const TC_ACT_OK: i32 = 0;
const TC_ACT_SHOT: i32 = 2;
const ETH_P_IP: u16 = 0x0800;
const ETH_P_IPV6: u16 = 0x86DD;
const NS_PER_SEC: u64 = 1_000_000_000;
const IP_VERSION_4: u8 = 4;
const IP_VERSION_6: u8 = 6;

const TAP_ID_UNASSIGNED: u32 = 0;
const ID_WILDCARD: u32 = 0;
const DIRECTION_INGRESS: u8 = 0;
const DIRECTION_EGRESS: u8 = 1;
const QOS_MODE_SHAPING: u8 = 1;
const QOS_EDT_MAX_DELAY_NS: u64 = 9_000_000_000;
const IPV4_IDENTITY_LOOKUP_BITS: u32 = 64;
const IPV6_IDENTITY_LOOKUP_BITS: u32 = 160;

type Ipv4IdentityKey = [u8; 8];
type Ipv6IdentityKey = [u8; 20];

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct QosKey {
    pub tap_id: u32,
    pub generation: u32,
    pub group_id: u32,
    pub direction: u8,
    pub pad: [u8; 3],
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct QosConfig {
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    pub mode: u8,
    pub pad: [u8; 6],
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct TokenBucket {
    pub tokens: u64,
    pub last_refill_ns: u64,
    pub last_edt: u64,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct QosStatsValue {
    pub passed_packets: u64,
    pub passed_bytes: u64,
    pub dropped_packets: u64,
    pub dropped_bytes: u64,
    pub shaped_packets: u64,
    pub shaped_bytes: u64,
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

#[map(name = "QOS_CONFIG", pin)]
static QOS_CONFIG: HashMap<QosKey, QosConfig> = HashMap::with_max_entries(65536, 0);

#[map(name = "QOS_TOKEN_BUCKET", pin)]
static QOS_TOKEN_BUCKET: HashMap<QosKey, TokenBucket> = HashMap::with_max_entries(65536, 0);

#[map(name = "QOS_STATS", pin)]
static QOS_STATS: PerCpuHashMap<QosKey, QosStatsValue> = PerCpuHashMap::with_max_entries(65536, 0);

#[map(name = "QOS_STATS_BUF")]
static QOS_STATS_BUF: PerCpuArray<QosStatsValue> = PerCpuArray::with_max_entries(1, 0);

#[map(name = "FIREWALL_CONFIG", pin)]
static FIREWALL_CONFIG: HashMap<u32, FirewallConfig> = HashMap::with_max_entries(1, 0);

#[map(name = "TAP_CONFIG_MAP", pin)]
static TAP_CONFIG_MAP: HashMap<u32, TapConfig> = HashMap::with_max_entries(1024, 0);

#[classifier]
pub fn tc_ingress_qos(ctx: TcContext) -> i32 {
    match try_tc_qos(ctx, DIRECTION_INGRESS) {
        Ok(ret) => ret,
        Err(_) => TC_ACT_OK,
    }
}

#[classifier]
pub fn tc_egress_qos(ctx: TcContext) -> i32 {
    match try_tc_qos(ctx, DIRECTION_EGRESS) {
        Ok(ret) => ret,
        Err(_) => TC_ACT_OK,
    }
}

fn try_tc_qos(ctx: TcContext, direction: u8) -> Result<i32, u64> {
    let tap_id = TAP_ID_UNASSIGNED;
    if !qos_enabled(tap_id) {
        return Ok(TC_ACT_OK);
    }

    let pkt_len = ctx.skb.len() as u64;

    if let Ok(eth) = ptr_at::<EthHdr>(&ctx, 0) {
        match u16::from_be(eth.ether_type) {
            ETH_P_IP => return try_tc_qos_ipv4(&ctx, direction, pkt_len, EthHdr::LEN),
            ETH_P_IPV6 => return try_tc_qos_ipv6(&ctx, direction, pkt_len, EthHdr::LEN),
            _ => {}
        }
    }

    if let Ok(ip) = ptr_at::<Ipv4Hdr>(&ctx, 0) {
        if ip.version() == IP_VERSION_4 {
            return try_tc_qos_ipv4(&ctx, direction, pkt_len, 0);
        }
    }

    if let Ok(ip) = ptr_at::<Ipv6Hdr>(&ctx, 0) {
        if ip.version() == IP_VERSION_6 {
            return try_tc_qos_ipv6(&ctx, direction, pkt_len, 0);
        }
    }

    Ok(TC_ACT_OK)
}

#[inline(always)]
fn try_tc_qos_ipv4(
    ctx: &TcContext,
    direction: u8,
    pkt_len: u64,
    ip_offset: usize,
) -> Result<i32, u64> {
    let ip = ptr_at::<Ipv4Hdr>(ctx, ip_offset)?;
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
    apply_qos_for_ids(ctx, generation, src_id, dst_id, direction, pkt_len)
}

#[inline(always)]
fn try_tc_qos_ipv6(
    ctx: &TcContext,
    direction: u8,
    pkt_len: u64,
    ip_offset: usize,
) -> Result<i32, u64> {
    let ip = ptr_at::<Ipv6Hdr>(ctx, ip_offset)?;
    let generation = active_policy_generation(TAP_ID_UNASSIGNED);
    let src_id = lookup_ipv6_id(&SRC_IPV6_ID_MAP, generation, ip.src_addr);
    let dst_id = lookup_ipv6_id(&DST_IPV6_ID_MAP, generation, ip.dst_addr);
    apply_qos_for_ids(ctx, generation, src_id, dst_id, direction, pkt_len)
}

#[inline(always)]
fn apply_qos_for_ids(
    ctx: &TcContext,
    generation: u32,
    src_id: u32,
    dst_id: u32,
    direction: u8,
    pkt_len: u64,
) -> Result<i32, u64> {
    let group_id = if direction == DIRECTION_EGRESS {
        dst_id
    } else {
        src_id
    };

    if let Some((key, config)) =
        lookup_qos_config(TAP_ID_UNASSIGNED, generation, group_id, direction)
    {
        let (action, edt, priority) = apply_qos_bucket(&key, config, pkt_len, direction);
        if action == TC_ACT_OK && edt != 0 {
            apply_edt_prio(ctx, edt, priority);
        }
        return Ok(action);
    }

    Ok(TC_ACT_OK)
}

fn qos_enabled(tap_id: u32) -> bool {
    if let Some(tap) = unsafe { TAP_CONFIG_MAP.get(&tap_id) } {
        return tap.qos_enabled != 0;
    }

    let key = 0;
    if let Some(config) = unsafe { FIREWALL_CONFIG.get(&key) } {
        return config.qos_enabled != 0;
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

fn lookup_qos_config(
    tap_id: u32,
    generation: u32,
    group_id: u32,
    direction: u8,
) -> Option<(QosKey, QosConfig)> {
    let mut best: Option<(QosKey, QosConfig)> = None;
    let exact_key = QosKey {
        tap_id,
        generation,
        group_id,
        direction,
        pad: [0; 3],
    };
    if let Some(config) = unsafe { QOS_CONFIG.get(&exact_key) } {
        best = choose_qos_config(best, (exact_key, *config));
    }

    let fallback_key = QosKey {
        tap_id,
        generation,
        group_id: ID_WILDCARD,
        direction,
        pad: [0; 3],
    };
    if let Some(config) = unsafe { QOS_CONFIG.get(&fallback_key) } {
        best = choose_qos_config(best, (fallback_key, *config));
    }

    best
}

fn choose_qos_config(
    current: Option<(QosKey, QosConfig)>,
    candidate: (QosKey, QosConfig),
) -> Option<(QosKey, QosConfig)> {
    match current {
        Some(existing) if existing.1.priority <= candidate.1.priority => Some(existing),
        _ => Some(candidate),
    }
}

fn apply_qos_bucket(
    key: &QosKey,
    config: QosConfig,
    pkt_len: u64,
    direction: u8,
) -> (i32, u64, u8) {
    let now = unsafe { bpf_ktime_get_ns() };
    let rate_bytes_per_sec = if config.rate_bps >= 8 {
        config.rate_bps / 8
    } else {
        1
    };

    if let Some(bucket_ptr) = QOS_TOKEN_BUCKET.get_ptr_mut(key) {
        let bucket = unsafe { &mut *bucket_ptr };
        let (passed, dropped, shaped, edt) =
            decide_qos_bucket(bucket, config, pkt_len, direction, now, rate_bytes_per_sec);
        update_qos_stats(key, pkt_len, dropped, shaped);
        return if passed {
            (TC_ACT_OK, edt, config.priority)
        } else {
            (TC_ACT_SHOT, 0, config.priority)
        };
    }

    let mut bucket = TokenBucket {
        tokens: config.burst_bytes,
        last_refill_ns: now,
        last_edt: 0,
    };
    let (passed, dropped, shaped, edt) =
        decide_qos_bucket(&mut bucket, config, pkt_len, direction, now, rate_bytes_per_sec);
    let _ = QOS_TOKEN_BUCKET.insert(key, &bucket, 0);
    update_qos_stats(key, pkt_len, dropped, shaped);
    if passed {
        (TC_ACT_OK, edt, config.priority)
    } else {
        (TC_ACT_SHOT, 0, config.priority)
    }
}

fn decide_qos_bucket(
    bucket: &mut TokenBucket,
    config: QosConfig,
    pkt_len: u64,
    direction: u8,
    now: u64,
    rate_bytes_per_sec: u64,
) -> (bool, bool, bool, u64) {
    let elapsed = if now > bucket.last_refill_ns {
        now - bucket.last_refill_ns
    } else {
        0
    };
    let elapsed = if elapsed > NS_PER_SEC {
        NS_PER_SEC
    } else {
        elapsed
    };
    let refill = mul_div(elapsed, rate_bytes_per_sec, NS_PER_SEC);
    let tokens = bucket.tokens.saturating_add(refill);
    bucket.tokens = if tokens > config.burst_bytes {
        config.burst_bytes
    } else {
        tokens
    };
    bucket.last_refill_ns = now;

    if direction == DIRECTION_EGRESS && config.mode == QOS_MODE_SHAPING {
        let edt = next_edt(bucket.last_edt, now, pkt_len, rate_bytes_per_sec);
        if edt == 0 {
            bucket.tokens = 0;
            return (false, true, false, 0);
        }
        bucket.last_edt = edt;
        if bucket.tokens >= pkt_len {
            bucket.tokens -= pkt_len;
            return (true, false, false, edt);
        }
        bucket.tokens = 0;
        return (true, false, true, edt);
    }

    if bucket.tokens >= pkt_len {
        bucket.tokens -= pkt_len;
        return (true, false, false, 0);
    }

    (false, true, false, 0)
}

fn update_qos_stats(key: &QosKey, pkt_len: u64, dropped: bool, shaped: bool) {
    if let Some(stats_ptr) = QOS_STATS.get_ptr_mut(key) {
        let stats = unsafe { &mut *stats_ptr };
        if dropped {
            stats.dropped_packets = stats.dropped_packets.wrapping_add(1);
            stats.dropped_bytes = stats.dropped_bytes.wrapping_add(pkt_len);
            return;
        }

        stats.passed_packets = stats.passed_packets.wrapping_add(1);
        stats.passed_bytes = stats.passed_bytes.wrapping_add(pkt_len);
        if shaped {
            stats.shaped_packets = stats.shaped_packets.wrapping_add(1);
            stats.shaped_bytes = stats.shaped_bytes.wrapping_add(pkt_len);
        }
        return;
    }

    if let Some(stats_ptr) = QOS_STATS_BUF.get_ptr_mut(0) {
        let stats = unsafe { &mut *stats_ptr };
        stats.passed_packets = 0;
        stats.passed_bytes = 0;
        stats.dropped_packets = 0;
        stats.dropped_bytes = 0;
        stats.shaped_packets = 0;
        stats.shaped_bytes = 0;

        if dropped {
            stats.dropped_packets = 1;
            stats.dropped_bytes = pkt_len;
        } else {
            stats.passed_packets = 1;
            stats.passed_bytes = pkt_len;
            if shaped {
                stats.shaped_packets = 1;
                stats.shaped_bytes = pkt_len;
            }
        }
        let _ = QOS_STATS.insert(key, &*stats, 0);
    }
}

#[inline(always)]
fn apply_edt_prio(ctx: &TcContext, edt: u64, priority: u8) {
    let skb = ctx.as_ptr() as *mut __sk_buff;
    unsafe {
        (*skb).tstamp = edt;
        if priority != 0 {
            (*skb).priority = priority as u32;
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

fn next_edt(last_edt: u64, now: u64, pkt_len: u64, rate_bytes_per_sec: u64) -> u64 {
    if rate_bytes_per_sec == 0 {
        return now;
    }
    let base = if last_edt > now { last_edt } else { now };
    let delay_ns = mul_div(pkt_len, NS_PER_SEC, rate_bytes_per_sec);
    let edt = base.saturating_add(delay_ns);
    let max_edt = now.saturating_add(QOS_EDT_MAX_DELAY_NS);
    if edt > max_edt {
        0
    } else {
        edt
    }
}

#[inline(always)]
fn mul_div(left: u64, right: u64, divisor: u64) -> u64 {
    if divisor == 0 {
        return 0;
    }
    let whole = left / divisor;
    let rem = left % divisor;
    (whole * right) + ((rem * right) / divisor)
}

#[inline(always)]
fn ptr_at<T>(ctx: &TcContext, offset: usize) -> Result<&T, u64> {
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
