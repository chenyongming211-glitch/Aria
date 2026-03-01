#![no_std]
#![no_main]

use aya_ebpf::{
    macros::{classifier, map},
    maps::{HashMap, LpmTrie, lpm_trie::Key},
    programs::TcContext,
};
use network_types::{
    eth::EthHdr,
    ip::{IpProto, Ipv4Hdr, Ipv6Hdr},
    tcp::TcpHdr,
    udp::UdpHdr,
};

const TC_ACT_OK: i32 = 0;
const TC_ACT_SHOT: i32 = 2;
const ETH_P_IP: u16 = 0x0800;
const ETH_P_IPV6: u16 = 0x86DD;
const NS_PER_SEC: u64 = 1_000_000_000;

const ID_WILDCARD: u32 = 0;

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
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

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct ServiceQoSKey {
    pub src_id: u32,
    pub dst_id: u32,
    pub dst_port: u16,
    pub protocol: u8,
    pub pad: u8,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PairQoSKey {
    pub src_id: u32,
    pub dst_id: u32,
}

// Identity maps - shared with ACL program via pinning
#[map(name = "SRC_IPV4_ID_MAP", pin)]
static SRC_IPV4_ID_MAP: LpmTrie<u32, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV4_ID_MAP", pin)]
static DST_IPV4_ID_MAP: LpmTrie<u32, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "SRC_IPV6_ID_MAP", pin)]
static SRC_IPV6_ID_MAP: LpmTrie<[u8; 16], u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV6_ID_MAP", pin)]
static DST_IPV6_ID_MAP: LpmTrie<[u8; 16], u32> = LpmTrie::with_max_entries(10000, 0);

#[map]
static SRC_ID_QOS_MAP: HashMap<u32, BucketState> = HashMap::with_max_entries(8192, 0);

#[map]
static PAIR_ID_QOS_MAP: HashMap<PairQoSKey, BucketState> = HashMap::with_max_entries(8192, 0);

#[map]
static SERVICE_QOS_MAP: HashMap<ServiceQoSKey, BucketState> = HashMap::with_max_entries(65536, 0);

#[classifier]
pub fn tc_egress_qos(ctx: TcContext) -> i32 {
    match try_tc_egress_qos(ctx) {
        Ok(ret) => ret,
        Err(_) => TC_ACT_OK,
    }
}

fn try_tc_egress_qos(ctx: TcContext) -> Result<i32, u64> {
    let eth = ptr_at::<EthHdr>(&ctx, 0)?;
    let pkt_len = ctx.skb.len() as u64;
    
    let (src_id, dst_id, proto_byte, dst_port) = match u16::from_be(eth.ether_type) {
        ETH_P_IP => {
            let ip = ptr_at::<Ipv4Hdr>(&ctx, EthHdr::LEN)?;
            let dst_port = parse_dst_port_ipv4(&ctx, ip)?;
            let src_id = lookup_ipv4_id(&SRC_IPV4_ID_MAP, u32::from_be_bytes(ip.src_addr))?;
            let dst_id = lookup_ipv4_id(&DST_IPV4_ID_MAP, u32::from_be_bytes(ip.dst_addr))?;
            (src_id, dst_id, ip.proto as u8, dst_port)
        }
        ETH_P_IPV6 => {
            let ip = ptr_at::<Ipv6Hdr>(&ctx, EthHdr::LEN)?;
            let dst_port = parse_dst_port_ipv6(&ctx, ip)?;
            let src_id = lookup_ipv6_id(&SRC_IPV6_ID_MAP, ip.src_addr)?;
            let dst_id = lookup_ipv6_id(&DST_IPV6_ID_MAP, ip.dst_addr)?;
            (src_id, dst_id, ip.next_hdr as u8, dst_port)
        }
        _ => return Ok(TC_ACT_OK),
    };

    let service_key = ServiceQoSKey {
        src_id,
        dst_id,
        dst_port,
        protocol: proto_byte,
        pad: 0,
    };
    if let Some(bucket) = unsafe { SERVICE_QOS_MAP.get(&service_key) } {
        let bucket = *bucket;
        let (new_bucket, pass) = update_bucket(&bucket, pkt_len);
        let _ = SERVICE_QOS_MAP.insert(&service_key, &new_bucket, 0);
        return Ok(if pass { TC_ACT_OK } else { TC_ACT_SHOT });
    }

    let pair_key = PairQoSKey { src_id, dst_id };
    if let Some(bucket) = unsafe { PAIR_ID_QOS_MAP.get(&pair_key) } {
        let bucket = *bucket;
        let (new_bucket, pass) = update_bucket(&bucket, pkt_len);
        let _ = PAIR_ID_QOS_MAP.insert(&pair_key, &new_bucket, 0);
        return Ok(if pass { TC_ACT_OK } else { TC_ACT_SHOT });
    }

    if src_id != ID_WILDCARD {
        if let Some(bucket) = unsafe { SRC_ID_QOS_MAP.get(&src_id) } {
            let bucket = *bucket;
            let (new_bucket, pass) = update_bucket(&bucket, pkt_len);
            let _ = SRC_ID_QOS_MAP.insert(&src_id, &new_bucket, 0);
            return Ok(if pass { TC_ACT_OK } else { TC_ACT_SHOT });
        }
    }

    Ok(TC_ACT_OK)
}

fn lookup_ipv4_id(map: &LpmTrie<u32, u32>, ip: u32) -> Result<u32, u64> {
    let key = Key::new(32, ip);
    if let Some(id) = map.get(&key) {
        return Ok(*id);
    }
    Ok(ID_WILDCARD)
}

fn lookup_ipv6_id(map: &LpmTrie<[u8; 16], u32>, ip: [u8; 16]) -> Result<u32, u64> {
    let key = Key::new(128, ip);
    if let Some(id) = map.get(&key) {
        return Ok(*id);
    }
    Ok(ID_WILDCARD)
}

fn parse_dst_port_ipv4(ctx: &TcContext, ip: &Ipv4Hdr) -> Result<u16, u64> {
    let ip_hdr_len = (ip.ihl() as usize) * 4;
    match IpProto::from(ip.proto) {
        IpProto::Tcp => {
            let tcp = ptr_at::<TcpHdr>(&ctx, EthHdr::LEN + ip_hdr_len)?;
            Ok(u16::from_be_bytes(tcp.dest))
        }
        IpProto::Udp => {
            let udp = ptr_at::<UdpHdr>(&ctx, EthHdr::LEN + ip_hdr_len)?;
            Ok(u16::from_be_bytes(udp.dst))
        }
        _ => Ok(0),
    }
}

fn parse_dst_port_ipv6(ctx: &TcContext, ip: &Ipv6Hdr) -> Result<u16, u64> {
    match IpProto::from(ip.next_hdr) {
        IpProto::Tcp => {
            let tcp = ptr_at::<TcpHdr>(&ctx, EthHdr::LEN + Ipv6Hdr::LEN)?;
            Ok(u16::from_be_bytes(tcp.dest))
        }
        IpProto::Udp => {
            let udp = ptr_at::<UdpHdr>(&ctx, EthHdr::LEN + Ipv6Hdr::LEN)?;
            Ok(u16::from_be_bytes(udp.dst))
        }
        _ => Ok(0),
    }
}

fn update_bucket(bucket: &BucketState, pkt_len: u64) -> (BucketState, bool) {
    let now = unsafe { aya_ebpf::helpers::bpf_ktime_get_ns() };
    let time_passed = if now > bucket.last_update_ns {
        now - bucket.last_update_ns
    } else {
        0
    };
    
    let time_passed = if time_passed > NS_PER_SEC {
        NS_PER_SEC
    } else {
        time_passed
    };

    let tokens_to_add = mul_div(time_passed, bucket.rate_bytes_per_sec, NS_PER_SEC);
    let new_tokens = if bucket.tokens.saturating_add(tokens_to_add) > bucket.burst_bytes {
        bucket.burst_bytes
    } else {
        bucket.tokens.saturating_add(tokens_to_add)
    };

    if new_tokens >= pkt_len {
        (
            BucketState {
                rate_bytes_per_sec: bucket.rate_bytes_per_sec,
                burst_bytes: bucket.burst_bytes,
                tokens: new_tokens - pkt_len,
                last_update_ns: now,
                pass_bytes: bucket.pass_bytes.wrapping_add(pkt_len),
                drop_bytes: bucket.drop_bytes,
                _pad: 0,
                rule_id: bucket.rule_id,
            },
            true,
        )
    } else {
        (
            BucketState {
                rate_bytes_per_sec: bucket.rate_bytes_per_sec,
                burst_bytes: bucket.burst_bytes,
                tokens: new_tokens,
                last_update_ns: now,
                pass_bytes: bucket.pass_bytes,
                drop_bytes: bucket.drop_bytes.wrapping_add(pkt_len),
                _pad: 0,
                rule_id: bucket.rule_id,
            },
            false,
        )
    }
}

#[inline(always)]
fn mul_div(time_passed: u64, rate: u64, ns_per_sec: u64) -> u64 {
    if ns_per_sec == 0 {
        return 0;
    }
    // 分离整数秒和小数秒，彻底杜绝 u64 溢出可能
    let sec = time_passed / ns_per_sec;
    let rem_ns = time_passed % ns_per_sec;
    
    // 即使 rate 是万兆 (10GB/s)，rem_ns * rate 也绝对不会溢出 u64
    (sec * rate) + ((rem_ns * rate) / ns_per_sec)
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
