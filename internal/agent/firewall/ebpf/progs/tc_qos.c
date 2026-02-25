// SPDX-License-Identifier: GPL-2.0 OR BSD-3-Clause
#include "aria_structs.h"
#include "aria_maps.h"
#include <bpf/bpf_helpers.h>
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/if_packet.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>

// 计算令牌桶令牌数量
static __always_inline __u64 calculate_tokens(struct bucket_state *state, __u64 now) {
    __u64 elapsed_ns = now - state->last_update_ns;

    // 计算新产生的令牌数 (按速率计算)
    __u64 new_tokens = state->tokens + (elapsed_ns * state->rate_bytes_ns);

    // 不超过突发容量
    if (new_tokens > state->burst_bytes) {
        new_tokens = state->burst_bytes;
    }

    return new_tokens;
}

// 尝试消费令牌（流量控制）
static __always_inline bool consume_tokens(struct bucket_state *state, __u64 bytes_to_consume) {
    __u64 now = bpf_ktime_get_ns();
    __u64 new_tokens;

    bpf_spin_lock(&state->lock);

    // 重新计算令牌数
    new_tokens = calculate_tokens(state, now);

    // 检查是否有足够令牌
    if (new_tokens >= bytes_to_consume) {
        // 扣除令牌
        state->tokens = new_tokens - bytes_to_consume;
        state->pass_bytes += bytes_to_consume;
    } else {
        // 令牌不足，增加丢弃统计
        state->drop_bytes += bytes_to_consume;
        bpf_spin_unlock(&state->lock);
        return false; // 拒绝
    }

    state->last_update_ns = now;
    bpf_spin_unlock(&state->lock);
    return true; // 通过
}

// TC程序 - 处理出站ACL和QoS
SEC("classifier")
int tc_qos_filter(struct __sk_buff *skb) {
    // 从skb解析网络包
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    // 解析以太网帧
    struct ethhdr *eth = data;
    if (data + sizeof(*eth) > data_end)
        return TC_ACT_OK;

    // 解析IP头部
    struct iphdr *ip = data + sizeof(*eth);
    if (data + sizeof(*eth) + sizeof(*ip) > data_end)
        return TC_ACT_OK;

    // 解析TCP/UDP头部
    struct tcphdr *tcp = NULL;
    struct udphdr *udp = NULL;
    __u32 packet_len = skb->len;

    if (ip->protocol == IPPROTO_TCP) {
        tcp = data + sizeof(*eth) + sizeof(*ip);
        if (data + sizeof(*eth) + sizeof(*ip) + sizeof(*tcp) > data_end)
            return TC_ACT_OK;
    } else if (ip->protocol == IPPROTO_UDP) {
        udp = data + sizeof(*eth) + sizeof(*ip);
        if (data + sizeof(*eth) + sizeof(*ip) + sizeof(*udp) > data_end)
            return TC_ACT_OK;
    }

    // 构建五元组键用于ACL检查
    struct acl_5tuple_key key = {};
    key.src_ip = ip->saddr;
    key.dst_ip = ip->daddr;
    key.proto = ip->protocol;

    if (tcp) {
        key.src_port = tcp->source;
        key.dst_port = tcp->dest;
    } else if (udp) {
        key.src_port = udp->source;
        key.dst_port = udp->dest;
    }

    // 检查出站五元组ACL规则
    struct acl_rule_value *acl_val = bpf_map_lookup_elem(&egress_5tuple_map, &key);
    if (acl_val) {
        if (acl_val->action == 0) {  // PASS
            // 继续检查QoS
        } else if (acl_val->action == 1) {  // DROP
            return TC_ACT_SHOT;
        }
    }

    // 检查出站IP阻断规则
    __u8 *ip_blocked = bpf_map_lookup_elem(&egress_ip_blk_map, &ip->daddr);
    if (ip_blocked && *ip_blocked) {
        return TC_ACT_SHOT;
    }

    // QoS流控处理 - 三级优先级：服务 > 对等体 > IP
    bool allowed = false;

    // 1. 服务级QoS（最高优先级）
    struct bucket_state *app_qos = bpf_map_lookup_elem(&app_qos_map, &key);
    if (app_qos) {
        allowed = consume_tokens(app_qos, packet_len);
        if (!allowed) {
            // 服务级限制拒绝，发送丢包事件
            struct drop_event_t event = {};
            event.rule_id = app_qos->lock; // 使用锁字段作为规则ID（实际应有独立的规则ID）
            event.reason = 1; // 服务级QoS限制
            event.src_ip = key.src_ip;
            event.dst_ip = key.dst_ip;
            event.src_port = key.src_port;
            event.dst_port = key.dst_port;
            event.proto = key.proto;
            event.timestamp = bpf_ktime_get_ns();

            bpf_ringbuf_output(&drop_alerts, &event, sizeof(event), 0);
            return TC_ACT_SHOT;
        }
    }

    // 2. 对等体QoS（如果未在服务级被处理）
    if (!allowed) {
        struct bucket_state *peer_qos = bpf_map_lookup_elem(&peer_qos_map, &key.dst_ip);
        if (peer_qos) {
            allowed = consume_tokens(peer_qos, packet_len);
            if (!allowed) {
                // 对等体级限制拒绝，发送丢包事件
                struct drop_event_t event = {};
                event.rule_id = peer_qos->lock; // 使用锁字段作为规则ID
                event.reason = 2; // 对等体级QoS限制
                event.src_ip = key.src_ip;
                event.dst_ip = key.dst_ip;
                event.src_port = key.src_port;
                event.dst_port = key.dst_port;
                event.proto = key.proto;
                event.timestamp = bpf_ktime_get_ns();

                bpf_ringbuf_output(&drop_alerts, &event, sizeof(event), 0);
                return TC_ACT_SHOT;
            }
        }
    }

    // 3. IP级QoS（最低优先级）
    if (!allowed) {
        struct bucket_state *ip_qos = bpf_map_lookup_elem(&global_qos_map, &key.dst_ip);
        if (ip_qos) {
            allowed = consume_tokens(ip_qos, packet_len);
            if (!allowed) {
                // IP级限制拒绝，发送丢包事件
                struct drop_event_t event = {};
                event.rule_id = ip_qos->lock; // 使用锁字段作为规则ID
                event.reason = 3; // IP级QoS限制
                event.src_ip = key.src_ip;
                event.dst_ip = key.dst_ip;
                event.src_port = key.src_port;
                event.dst_port = key.dst_port;
                event.proto = key.proto;
                event.timestamp = bpf_ktime_get_ns();

                bpf_ringbuf_output(&drop_alerts, &event, sizeof(event), 0);
                return TC_ACT_SHOT;
            }
        }
    }

    // 如果所有QoS检查通过或未设置QoS，则通过
    return TC_ACT_OK;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";