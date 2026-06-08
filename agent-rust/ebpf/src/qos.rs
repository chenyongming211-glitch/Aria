#![no_std]
#![no_main]

use aya_ebpf::{
    helpers::bpf_ktime_get_ns,
    macros::{classifier, map},
    maps::{lpm_trie::Key, HashMap, LpmTrie, PerCpuHashMap},
    programs::TcContext,
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

const TAP_ID_UNASSIGNED: u32 = 0;
const ID_WILDCARD: u32 = 0;
const DIRECTION_INGRESS: u8 = 0;
const DIRECTION_EGRESS: u8 = 1;
const QOS_MODE_SHAPING: u8 = 1;

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct QosKey {
    pub tap_id: u32,
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

#[map(name = "QOS_CONFIG", pin)]
static QOS_CONFIG: HashMap<QosKey, QosConfig> = HashMap::with_max_entries(65536, 0);

#[map(name = "QOS_TOKEN_BUCKET", pin)]
static QOS_TOKEN_BUCKET: HashMap<QosKey, TokenBucket> = HashMap::with_max_entries(65536, 0);

#[map(name = "QOS_STATS", pin)]
static QOS_STATS: PerCpuHashMap<QosKey, QosStatsValue> =
    PerCpuHashMap::with_max_entries(65536, 0);

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

    let eth = ptr_at::<EthHdr>(&ctx, 0)?;
    let pkt_len = ctx.skb.len() as u64;

    let (src_id, dst_id) = match u16::from_be(eth.ether_type) {
        ETH_P_IP => {
            let ip = ptr_at::<Ipv4Hdr>(&ctx, EthHdr::LEN)?;
            let src_id = lookup_ipv4_id(&SRC_IPV4_ID_MAP, u32::from_be_bytes(ip.src_addr));
            let dst_id = lookup_ipv4_id(&DST_IPV4_ID_MAP, u32::from_be_bytes(ip.dst_addr));
            (src_id, dst_id)
        }
        ETH_P_IPV6 => {
            let ip = ptr_at::<Ipv6Hdr>(&ctx, EthHdr::LEN)?;
            let src_id = lookup_ipv6_id(&SRC_IPV6_ID_MAP, ip.src_addr);
            let dst_id = lookup_ipv6_id(&DST_IPV6_ID_MAP, ip.dst_addr);
            (src_id, dst_id)
        }
        _ => return Ok(TC_ACT_OK),
    };

    let group_id = if direction == DIRECTION_EGRESS {
        dst_id
    } else {
        src_id
    };

    if let Some((key, config)) = lookup_qos_config(tap_id, group_id, direction) {
        let passed = apply_qos_bucket(&key, config, pkt_len, direction);
        return Ok(if passed { TC_ACT_OK } else { TC_ACT_SHOT });
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

fn lookup_qos_config(tap_id: u32, group_id: u32, direction: u8) -> Option<(QosKey, QosConfig)> {
    let exact_key = QosKey {
        tap_id,
        group_id,
        direction,
        pad: [0; 3],
    };
    if let Some(config) = unsafe { QOS_CONFIG.get(&exact_key) } {
        return Some((exact_key, *config));
    }

    let fallback_key = QosKey {
        tap_id,
        group_id: ID_WILDCARD,
        direction,
        pad: [0; 3],
    };
    if let Some(config) = unsafe { QOS_CONFIG.get(&fallback_key) } {
        return Some((fallback_key, *config));
    }

    None
}

fn apply_qos_bucket(key: &QosKey, config: QosConfig, pkt_len: u64, direction: u8) -> bool {
    let now = unsafe { bpf_ktime_get_ns() };
    let mut bucket = match unsafe { QOS_TOKEN_BUCKET.get(key) } {
        Some(existing) => *existing,
        None => TokenBucket {
            tokens: config.burst_bytes,
            last_refill_ns: now,
            last_edt: now,
        },
    };

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
    let rate_bytes_per_sec = config.rate_bps / 8;
    let refill = mul_div(elapsed, rate_bytes_per_sec, NS_PER_SEC);
    let tokens = bucket.tokens.saturating_add(refill);
    bucket.tokens = if tokens > config.burst_bytes {
        config.burst_bytes
    } else {
        tokens
    };
    bucket.last_refill_ns = now;

    if bucket.tokens >= pkt_len {
        bucket.tokens -= pkt_len;
        if config.mode == QOS_MODE_SHAPING && direction == DIRECTION_EGRESS {
            bucket.last_edt = now;
            let _ = QOS_TOKEN_BUCKET.insert(key, &bucket, 0);
            update_qos_stats(key, pkt_len, false, true);
        } else {
            let _ = QOS_TOKEN_BUCKET.insert(key, &bucket, 0);
            update_qos_stats(key, pkt_len, false, false);
        }
        return true;
    }

    if config.mode == QOS_MODE_SHAPING && direction == DIRECTION_EGRESS {
        bucket.last_edt = next_edt(bucket.last_edt, now, pkt_len, rate_bytes_per_sec);
        let _ = QOS_TOKEN_BUCKET.insert(key, &bucket, 0);
        update_qos_stats(key, pkt_len, false, true);
        return true;
    }

    let _ = QOS_TOKEN_BUCKET.insert(key, &bucket, 0);
    update_qos_stats(key, pkt_len, true, false);
    false
}

fn update_qos_stats(key: &QosKey, pkt_len: u64, dropped: bool, shaped: bool) {
    if let Some(stats_ptr) = unsafe { QOS_STATS.get_ptr_mut(key) } {
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

fn next_edt(last_edt: u64, now: u64, pkt_len: u64, rate_bytes_per_sec: u64) -> u64 {
    if rate_bytes_per_sec == 0 {
        return now;
    }
    let base = if last_edt > now { last_edt } else { now };
    let delay_ns = mul_div(pkt_len, NS_PER_SEC, rate_bytes_per_sec);
    base.saturating_add(delay_ns)
}

#[inline(always)]
fn mul_div(left: u64, right: u64, divisor: u64) -> u64 {
    if divisor == 0 {
        return 0;
    }
    let whole = left / divisor;
    let rem = left % divisor;
    whole
        .saturating_mul(right)
        .saturating_add(rem.saturating_mul(right) / divisor)
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
