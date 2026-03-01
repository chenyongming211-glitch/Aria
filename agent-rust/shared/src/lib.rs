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
    }
}
