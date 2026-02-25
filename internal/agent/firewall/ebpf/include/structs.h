#ifndef __STRUCTS_H__
#define __STRUCTS_H__

#include <linux/types.h>
#include <bpf/bpf_helpers.h>

// ACL 五元组键结构
struct acl_5tuple_key {
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8 proto;
    __u8 pad1;  // 填充，保持8字节对齐
    __u16 pad2; // 填充，保持8字节对齐
};

// ACL 规则值结构
struct acl_rule_value {
    __u32 action;   // 0=DROP, 1=PASS
    __u32 rule_id;
    __u64 bytes;
    __u64 packets;
};

// QoS令牌桶状态结构
struct bucket_state {
    // 配置参数
    __u64 rate_bytes_ns;    // 每纳秒速率（字节）
    __u64 burst_bytes;      // 突发容量（字节）

    // 运行状态
    __u64 tokens;           // 当前令牌数
    __u64 last_update_ns;   // 最后更新时间（纳秒）

    // 统计信息
    __u64 pass_bytes;
    __u64 drop_bytes;

    // 并发保护
    struct bpf_spin_lock lock;
};

// 流量详情键结构
struct flow_detail_key {
    __u32 rule_id;
    __u32 rule_type;        // 1=App, 2=Peer, 3=Global, 4=ACL
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8 proto;
    __u8 pad1;              // 填充，保持8字节对齐
    __u16 pad2;             // 填充，保持8字节对齐
};

// 流量详情统计结构
struct flow_detail_stats {
    __u64 bytes;
    __u64 packets;
    __u64 last_seen;
};

// 丢包事件结构
struct drop_event_t {
    __u32 rule_id;
    __u32 reason;           // 丢包原因
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8 proto;
    __u8 pad1;              // 填充，保持8字节对齐
    __u16 pad2;             // 填充，保持8字节对齐
    __u64 timestamp;
};

#endif /* __STRUCTS_H__ */