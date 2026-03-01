#![no_std]
#![no_main]

use aya_ebpf::{
    macros::{map, xdp},
    maps::{HashMap, LpmTrie, lpm_trie::Key},
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

const ACTION_DROP: u32 = 0;
const ACTION_PASS: u32 = 1;

const ID_WILDCARD: u32 = 0;

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct PolicyValue {
    pub action: u32,
    pub rule_id: u32,
    pub bytes: u64,
    pub packets: u64,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PolicyKey {
    pub src_id: u32,
    pub dst_id: u32,
    pub dst_port: u16,
    pub protocol: u8,
    pub pad: u8,
}

// 修复点：直接使用 u32 和 [u8; 16] 作为 LpmTrie 的数据泛型，无需自定义结构体
#[map(name = "SRC_IPV4_ID_MAP", pin)]
static SRC_IPV4_ID_MAP: LpmTrie<u32, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV4_ID_MAP", pin)]
static DST_IPV4_ID_MAP: LpmTrie<u32, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "SRC_IPV6_ID_MAP", pin)]
static SRC_IPV6_ID_MAP: LpmTrie<[u8; 16], u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV6_ID_MAP", pin)]
static DST_IPV6_ID_MAP: LpmTrie<[u8; 16], u32> = LpmTrie::with_max_entries(10000, 0);

#[map]
static POLICY_MAP: HashMap<PolicyKey, PolicyValue> = HashMap::with_max_entries(65536, 0);

#[map]
static BLOCK_SRC_ID_MAP: HashMap<u32, u8> = HashMap::with_max_entries(8192, 0);

#[map]
static BLOCK_DST_ID_MAP: HashMap<u32, u8> = HashMap::with_max_entries(8192, 0);

#[map]
static BLOCK_PORT_MAP: HashMap<u16, u8> = HashMap::with_max_entries(8192, 0);

#[xdp]
pub fn xdp_ingress_acl(ctx: XdpContext) -> u32 {
    match try_xdp_ingress_acl(ctx) {
        Ok(ret) => ret,
        Err(_) => XDP_PASS,
    }
}

fn try_xdp_ingress_acl(ctx: XdpContext) -> Result<u32, u64> {
    let eth = ptr_at::<EthHdr>(&ctx, 0)?;
    let pkt_len = (ctx.data_end() - ctx.data()) as u64;
    
    // 修复点：把未使用的 src_port 改为 _src_port 消除警告
    let (src_id, dst_id, proto_byte, _src_port, dst_port) = match u16::from_be(eth.ether_type) {
        ETH_P_IP => {
            let ip = ptr_at::<Ipv4Hdr>(&ctx, EthHdr::LEN)?;
            let (src_port, dst_port) = parse_ports_ipv4(&ctx, ip)?;
            let src_id = lookup_ipv4_id(&SRC_IPV4_ID_MAP, u32::from_be_bytes(ip.src_addr))?;
            let dst_id = lookup_ipv4_id(&DST_IPV4_ID_MAP, u32::from_be_bytes(ip.dst_addr))?;
            (src_id, dst_id, ip.proto as u8, src_port, dst_port)
        }
        ETH_P_IPV6 => {
            let ip = ptr_at::<Ipv6Hdr>(&ctx, EthHdr::LEN)?;
            let (src_port, dst_port) = parse_ports_ipv6(&ctx, ip)?;
            let src_id = lookup_ipv6_id(&SRC_IPV6_ID_MAP, ip.src_addr)?;
            let dst_id = lookup_ipv6_id(&DST_IPV6_ID_MAP, ip.dst_addr)?;
            (src_id, dst_id, ip.next_hdr as u8, src_port, dst_port)
        }
        _ => return Ok(XDP_PASS),
    };

    // 检查源 ID 黑名单
    if let Some(_) = unsafe { BLOCK_SRC_ID_MAP.get(&src_id) } {
        return Ok(XDP_DROP);
    }

    // 检查目标 ID 黑名单
    if let Some(_) = unsafe { BLOCK_DST_ID_MAP.get(&dst_id) } {
        return Ok(XDP_DROP);
    }

    // 检查端口黑名单
    if dst_port != 0 {
        if let Some(_) = unsafe { BLOCK_PORT_MAP.get(&dst_port) } {
            return Ok(XDP_DROP);
        }
    }

    let key = PolicyKey {
        src_id,
        dst_id,
        dst_port,
        protocol: proto_byte,
        pad: 0,
    };

    if let Some(policy) = unsafe { POLICY_MAP.get(&key) } {
        update_policy_stats(&key, policy, pkt_len);
        return Ok(if policy.action == ACTION_PASS { XDP_PASS } else { XDP_DROP });
    }

    let wildcard_src_key = PolicyKey {
        src_id: ID_WILDCARD,
        dst_id,
        dst_port,
        protocol: proto_byte,
        pad: 0,
    };
    if let Some(policy) = unsafe { POLICY_MAP.get(&wildcard_src_key) } {
        update_policy_stats(&wildcard_src_key, policy, pkt_len);
        return Ok(if policy.action == ACTION_PASS { XDP_PASS } else { XDP_DROP });
    }

    let wildcard_dst_key = PolicyKey {
        src_id,
        dst_id: ID_WILDCARD,
        dst_port,
        protocol: proto_byte,
        pad: 0,
    };
    if let Some(policy) = unsafe { POLICY_MAP.get(&wildcard_dst_key) } {
        update_policy_stats(&wildcard_dst_key, policy, pkt_len);
        return Ok(if policy.action == ACTION_PASS { XDP_PASS } else { XDP_DROP });
    }

    let full_wildcard_key = PolicyKey {
        src_id: ID_WILDCARD,
        dst_id: ID_WILDCARD,
        dst_port,
        protocol: proto_byte,
        pad: 0,
    };
    if let Some(policy) = unsafe { POLICY_MAP.get(&full_wildcard_key) } {
        update_policy_stats(&full_wildcard_key, policy, pkt_len);
        return Ok(if policy.action == ACTION_PASS { XDP_PASS } else { XDP_DROP });
    }

    if dst_port != 0 {
        let port_wildcard_key = PolicyKey {
            src_id,
            dst_id,
            dst_port: 0,
            protocol: proto_byte,
            pad: 0,
        };
        if let Some(policy) = unsafe { POLICY_MAP.get(&port_wildcard_key) } {
            update_policy_stats(&port_wildcard_key, policy, pkt_len);
            return Ok(if policy.action == ACTION_PASS { XDP_PASS } else { XDP_DROP });
        }
    }

    Ok(XDP_PASS)
}

// 修复点：直接传入 LpmTrie<u32, u32>，并使用 aya 的 Key::new 进行查询
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

fn parse_ports_ipv4(ctx: &XdpContext, ip: &Ipv4Hdr) -> Result<(u16, u16), u64> {
    let ip_hdr_len = (ip.ihl() as usize) * 4;
    match IpProto::from(ip.proto) {
        IpProto::Tcp => {
            let tcp = ptr_at::<TcpHdr>(&ctx, EthHdr::LEN + ip_hdr_len)?;
            Ok((u16::from_be_bytes(tcp.source), u16::from_be_bytes(tcp.dest)))
        }
        IpProto::Udp => {
            let udp = ptr_at::<UdpHdr>(&ctx, EthHdr::LEN + ip_hdr_len)?;
            Ok((u16::from_be_bytes(udp.src), u16::from_be_bytes(udp.dst)))
        }
        _ => Ok((0, 0)),
    }
}

fn parse_ports_ipv6(ctx: &XdpContext, ip: &Ipv6Hdr) -> Result<(u16, u16), u64> {
    match IpProto::from(ip.next_hdr) {
        IpProto::Tcp => {
            let tcp = ptr_at::<TcpHdr>(&ctx, EthHdr::LEN + Ipv6Hdr::LEN)?;
            Ok((u16::from_be_bytes(tcp.source), u16::from_be_bytes(tcp.dest)))
        }
        IpProto::Udp => {
            let udp = ptr_at::<UdpHdr>(&ctx, EthHdr::LEN + Ipv6Hdr::LEN)?;
            Ok((u16::from_be_bytes(udp.src), u16::from_be_bytes(udp.dst)))
        }
        _ => Ok((0, 0)),
    }
}

fn update_policy_stats(key: &PolicyKey, policy: &PolicyValue, pkt_len: u64) {
    let new_policy = PolicyValue {
        action: policy.action,
        rule_id: policy.rule_id,
        bytes: policy.bytes.wrapping_add(pkt_len),
        packets: policy.packets.wrapping_add(1),
    };
    let _ = unsafe { POLICY_MAP.insert(key, &new_policy, 0) };
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
