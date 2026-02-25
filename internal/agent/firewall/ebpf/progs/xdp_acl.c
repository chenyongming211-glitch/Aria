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

// XDP程序 - 处理入站ACL
SEC("xdp")
int xdp_acl_filter(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    // 解析以太网帧
    struct ethhdr *eth = data;
    if (data + sizeof(*eth) > data_end)
        return XDP_PASS;

    // 解析IP头部
    struct iphdr *ip = data + sizeof(*eth);
    if (data + sizeof(*eth) + sizeof(*ip) > data_end)
        return XDP_PASS;

    // 解析TCP/UDP头部
    struct tcphdr *tcp = NULL;
    struct udphdr *udp = NULL;

    if (ip->protocol == IPPROTO_TCP) {
        tcp = data + sizeof(*eth) + sizeof(*ip);
        if (data + sizeof(*eth) + sizeof(*ip) + sizeof(*tcp) > data_end)
            return XDP_PASS;
    } else if (ip->protocol == IPPROTO_UDP) {
        udp = data + sizeof(*eth) + sizeof(*ip);
        if (data + sizeof(*eth) + sizeof(*ip) + sizeof(*udp) > data_end)
            return XDP_PASS;
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

    // 检查五元组ACL规则
    struct acl_rule_value *acl_val = bpf_map_lookup_elem(&ingress_5tuple_map, &key);
    if (acl_val) {
        if (acl_val->action == 0) {  // PASS
            return XDP_PASS;
        } else if (acl_val->action == 1) {  // DROP
            return XDP_DROP;
        }
    }

    // 检查目标端口阻断规则
    if (tcp) {
        __u8 *port_blocked = bpf_map_lookup_elem(&ingress_port_blk_map, &tcp->dest);
        if (port_blocked && *port_blocked) {
            return XDP_DROP;
        }
    } else if (udp) {
        __u8 *port_blocked = bpf_map_lookup_elem(&ingress_port_blk_map, &udp->dest);
        if (port_blocked && *port_blocked) {
            return XDP_DROP;
        }
    }

    // 检查目标IP阻断规则
    __u8 *ip_blocked = bpf_map_lookup_elem(&ingress_ip_blk_map, &ip->daddr);
    if (ip_blocked && *ip_blocked) {
        return XDP_DROP;
    }

    return XDP_PASS;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";