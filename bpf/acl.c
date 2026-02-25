// +build ignore

// bpf/acl.c - XDP program for Access Control List (ACL)
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#define BPF_F_CURRENT_NETNS (-1L)

// ACL Action codes
#define ACL_ACTION_PASS 0
#define ACL_ACTION_DROP 1
#define ACL_ACTION_REDIRECT 2

// XDP action codes
#define XDP_PASS 0
#define XDP_DROP 2
#define XDP_REDIRECT 4

// Structure for ACL value (matches ACLValue in Go)
struct acl_value {
    __u32 action;
    __u32 port;  // Used for redirect
};

// LPM Trie Map for ACL rules
struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __uint(max_entries, 10240);
    __type(key, struct {
        __u32 prefixlen;
        __u32 addr;
    });
    __type(value, struct acl_value);
    __uint(map_flags, BPF_F_NO_PREALLOC);
} acl_rules SEC(".maps");

SEC("xdp_acl")
int xdp_acl_filter(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    // Parse Ethernet header
    struct ethhdr *eth = data;
    if (data + sizeof(*eth) > data_end) {
        return XDP_PASS;
    }

    // Process only IPv4 packets
    if (eth->h_proto != bpf_htons(ETH_P_IP)) {
        return XDP_PASS;
    }

    // Parse IP header
    struct iphdr *ip = (struct iphdr *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        return XDP_PASS;
    }

    // Prepare LPM trie key for source IP lookup
    struct {
        __u32 prefixlen;
        __u32 addr;
    } src_key = {
        .prefixlen = 32,  // Exact match for the source IP
        .addr = ip->saddr,
    };

    // Look up source IP in ACL rules
    struct acl_value *acl_val = bpf_map_lookup_elem(&acl_rules, &src_key);

    if (acl_val) {
        // Rule found for source IP
        if (acl_val->action == ACL_ACTION_DROP) {
            return XDP_DROP;
        } else if (acl_val->action == ACL_ACTION_REDIRECT) {
            // TODO: Implement redirect logic
            // For now, just pass the packet
            return XDP_PASS;
        }
        // If action is PASS or unknown, continue processing
    }

    // Prepare LPM trie key for destination IP lookup
    struct {
        __u32 prefixlen;
        __u32 addr;
    } dst_key = {
        .prefixlen = 32,  // Exact match for the destination IP
        .addr = ip->daddr,
    };

    // Look up destination IP in ACL rules
    acl_val = bpf_map_lookup_elem(&acl_rules, &dst_key);

    if (acl_val) {
        // Rule found for destination IP
        if (acl_val->action == ACL_ACTION_DROP) {
            return XDP_DROP;
        } else if (acl_val->action == ACL_ACTION_REDIRECT) {
            // TODO: Implement redirect logic
            // For now, just pass the packet
            return XDP_PASS;
        }
        // If action is PASS or unknown, continue processing
    }

    // Perform longest prefix match for CIDR blocks
    // Check for /31 subnet
    src_key.prefixlen = 31;
    acl_val = bpf_map_lookup_elem(&acl_rules, &src_key);
    if (!acl_val) {
        src_key.addr &= ~1; // Clear lowest bit for even address
        acl_val = bpf_map_lookup_elem(&acl_rules, &src_key);
    }

    if (!acl_val) {
        src_key.addr |= 1; // Set lowest bit for odd address
        acl_val = bpf_map_lookup_elem(&acl_rules, &src_key);
    }

    if (acl_val) {
        if (acl_val->action == ACL_ACTION_DROP) {
            return XDP_DROP;
        }
    }

    // Similar check for /30 subnet, /29, ..., down to desired min prefix length
    // This is simplified - a real implementation would iterate through prefixes
    for (__u32 pl = 30; pl >= 8 && !acl_val; pl--) {
        src_key.prefixlen = pl;
        src_key.addr = ip->saddr & (~0U << (32 - pl));
        acl_val = bpf_map_lookup_elem(&acl_rules, &src_key);
    }

    if (acl_val) {
        if (acl_val->action == ACL_ACTION_DROP) {
            return XDP_DROP;
        }
    }

    // No matching block rule found, allow the packet
    return XDP_PASS;
}

char LICENSE[] SEC("license") = "GPL";