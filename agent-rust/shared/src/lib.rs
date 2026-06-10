pub const BPF_PIN_PATH: &str = "/sys/fs/bpf";

pub const TC_ACT_OK: u32 = 0;
pub const TC_ACT_SHOT: u32 = 2;

pub const XDP_PASS: u32 = 2;
pub const XDP_DROP: u32 = 1;

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct Acl5TupleKey {
    pub src_ip: u32,
    pub dst_ip: u32,
    pub src_port: u16,
    pub dst_port: u16,
    pub proto: u8,
    pub pad1: u8,
    pub pad2: u16,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct AclRuleValue {
    pub action: u32,
    pub rule_id: u32,
    pub bytes: u64,
    pub packets: u64,
}

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
#[derive(Clone, Copy, Debug, Default)]
pub struct PeerPair {
    pub src_ip: u32,
    pub dst_ip: u32,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct FlowDetailKey {
    pub rule_id: u32,
    pub rule_type: u32,
    pub src_ip: u32,
    pub dst_ip: u32,
    pub src_port: u16,
    pub dst_port: u16,
    pub proto: u8,
    pub pad1: u8,
    pub pad2: u16,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct FlowDetailStats {
    pub bytes: u64,
    pub packets: u64,
    pub last_seen: u64,
}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct DropEvent {
    pub rule_id: u32,
    pub reason: u32,
    pub src_ip: u32,
    pub dst_ip: u32,
    pub src_port: u16,
    pub dst_port: u16,
    pub proto: u8,
    pub pad1: u8,
    pub pad2: u16,
    pub timestamp: u64,
}

pub const ACTION_DROP: u32 = 0;
pub const ACTION_PASS: u32 = 1;

pub const RULE_TYPE_ACL: u32 = 0;
pub const RULE_TYPE_QOS: u32 = 1;

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

unsafe impl aya::Pod for PolicyKey {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct PolicyValue {
    pub action: u8,
    pub has_port_filter: u8,
    pub pad1: [u8; 2],
    pub bitmap_idx: u32,
}

unsafe impl aya::Pod for PolicyValue {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct PortKey {
    pub tap_id: u32,
    pub idx: u32,
    pub port: u16,
    pub pad: u16,
}

unsafe impl aya::Pod for PortKey {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct QosKey {
    pub tap_id: u32,
    pub group_id: u32,
    pub direction: u8,
    pub pad: [u8; 3],
}

unsafe impl aya::Pod for QosKey {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct QosConfig {
    pub rate_bps: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    pub mode: u8,
    pub pad: [u8; 6],
}

unsafe impl aya::Pod for QosConfig {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct TokenBucket {
    pub tokens: u64,
    pub last_refill_ns: u64,
    pub last_edt: u64,
}

unsafe impl aya::Pod for TokenBucket {}

#[repr(C)]
#[derive(Clone, Copy, Debug, Default)]
pub struct RuleStatsValue {
    pub packets: u64,
    pub bytes: u64,
    pub dropped_packets: u64,
    pub dropped_bytes: u64,
}

unsafe impl aya::Pod for RuleStatsValue {}

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

unsafe impl aya::Pod for QosStatsValue {}

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

unsafe impl aya::Pod for FirewallConfig {}

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

unsafe impl aya::Pod for TapConfig {}

pub const TAP_ID_UNASSIGNED: u32 = 0;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_struct_sizes() {
        assert_eq!(core::mem::size_of::<Acl5TupleKey>(), 16);
        assert_eq!(core::mem::size_of::<AclRuleValue>(), 24);
        assert_eq!(core::mem::size_of::<BucketState>(), 56);
        assert_eq!(core::mem::size_of::<PeerPair>(), 8);
        assert_eq!(core::mem::size_of::<FlowDetailKey>(), 24);
        assert_eq!(core::mem::size_of::<FlowDetailStats>(), 24);
        assert_eq!(core::mem::size_of::<DropEvent>(), 32);
        assert_eq!(core::mem::size_of::<PolicyKey>(), 20);
        assert_eq!(core::mem::size_of::<PolicyValue>(), 8);
        assert_eq!(core::mem::size_of::<PortKey>(), 12);
        assert_eq!(core::mem::size_of::<QosKey>(), 12);
        assert_eq!(core::mem::size_of::<QosConfig>(), 24);
        assert_eq!(core::mem::size_of::<TokenBucket>(), 24);
        assert_eq!(core::mem::size_of::<RuleStatsValue>(), 32);
        assert_eq!(core::mem::size_of::<QosStatsValue>(), 48);
        assert_eq!(core::mem::size_of::<FirewallConfig>(), 10);
        assert_eq!(core::mem::size_of::<TapConfig>(), 8);
    }
}
