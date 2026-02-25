// SPDX-License-Identifier: (GPL-2.0 OR BSD-3-Clause)
#ifndef __ARIA_EBPF_MAPS_H__
#define __ARIA_EBPF_MAPS_H__

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include "aria_structs.h"

// 入站ACL (XDP)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(key_size, sizeof(struct acl_5tuple_key));
    __uint(value_size, sizeof(struct acl_rule_value));
    __uint(max_entries, 65536);
} ingress_5tuple_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(key_size, sizeof(__u16));  // 端口号
    __uint(value_size, sizeof(__u8)); // 阻断标志
    __uint(max_entries, 8192);
} ingress_port_blk_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(key_size, sizeof(__u32));  // IP地址
    __uint(value_size, sizeof(__u8)); // 阻断标志
    __uint(max_entries, 8192);
} ingress_ip_blk_map SEC(".maps");

// 出站ACL (TC)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(key_size, sizeof(struct acl_5tuple_key));
    __uint(value_size, sizeof(struct acl_rule_value));
    __uint(max_entries, 65536);
} egress_5tuple_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(key_size, sizeof(__u32));  // IP地址
    __uint(value_size, sizeof(__u8)); // 阻断标志
    __uint(max_entries, 8192);
} egress_ip_blk_map SEC(".maps");

// QoS流控 (TC)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(key_size, sizeof(struct acl_5tuple_key));  // 应用级别的五元组
    __uint(value_size, sizeof(struct bucket_state));
    __uint(max_entries, 65536);
} app_qos_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(key_size, sizeof(__u32));  // 对端IP
    __uint(value_size, sizeof(struct bucket_state));
    __uint(max_entries, 8192);
} peer_qos_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(struct bucket_state));
    __uint(max_entries, 1);  // 用于该Agent的总物理出口兜底
} global_qos_map SEC(".maps");

// 全量可观测
struct {
    __uint(type, BPF_MAP_TYPE_LRU_PERCPU_HASH);
    __uint(key_size, sizeof(struct flow_detail_key));
    __uint(value_size, sizeof(struct flow_detail_stats));
    __uint(max_entries, 65536);
} rule_flow_table SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 512 * 1024);  // 512KB ring buffer
} drop_alerts SEC(".maps");

#endif /* __ARIA_EBPF_MAPS_H__ */