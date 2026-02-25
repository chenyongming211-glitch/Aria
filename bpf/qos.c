// +build ignore

// bpf/qos.c - TC program for Quality of Service (QoS) with aggregation and port-level control support
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_tracing.h>

#define TC_ACT_OK     0
#define TC_ACT_SHOT   2

// Token bucket state structure (must match BucketState in Go)
struct bucket_state {
    struct bpf_spin_lock lock;  // Lock for thread safety (aggregation support)
    __u64 rate;                // Rate in bytes per nanosecond
    __u64 tokens;              // Current tokens in the bucket
    __u64 last_time;           // Last update time (nanoseconds)
    __u64 capacity;            // Maximum capacity of the bucket
};

// Five-tuple Flow Key for fine-grained control
struct flow_key {
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port; // Network byte order
    __u16 dst_port; // Network byte order
    __u8  proto;    // IPPROTO_TCP(6) / IPPROTO_UDP(17)
    __u8  pad[3];   // Memory alignment padding
};

// Peer pair structure (must match PeerPair in Go)
struct peer_pair {
    __u32 src_ip;
    __u32 dst_ip;
};

// Hash Map for per-IP rate limiting
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u32);        // IP address
    __type(value, struct bucket_state);
} ip_rate_map SEC(".maps");

// Hash Map for peer-to-peer rate limiting
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, struct peer_pair); // Peer pair
    __type(value, struct bucket_state);
} peer_rate_map SEC(".maps");

// Hash Map for service-level rate limiting (port-based)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536); // Five-tuple quantity may be large, increase capacity
    __type(key, struct flow_key);
    __type(value, struct bucket_state);
} service_rate_map SEC(".maps");

// Core TC classifier function for QoS with port-level control
SEC("classifier")
int tc_egress_qos(struct __sk_buff *skb) {
    // Parse to get IP header information
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    // Parse Ethernet header
    struct ethhdr *eth = data;
    if (data + sizeof(*eth) > data_end) {
        return TC_ACT_OK;
    }

    // Process only IPv4 packets
    if (eth->h_proto != bpf_htons(ETH_P_IP)) {
        return TC_ACT_OK;
    }

    // Parse IP header
    struct iphdr *ip = (struct iphdr *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        return TC_ACT_OK;
    }

    __u32 src_ip = ip->saddr;
    __u32 dst_ip = ip->daddr;
    __u64 pkt_len = skb->len;

    // 1. Get IP header length (IHL * 4 bytes)
    // ip->ihl is 4 bits, multiply by 4 to get byte count
    int ip_len = ip->ihl * 4;

    // 2. Locate transport layer header (TCP/UDP)
    void *trans_header = (void *)ip + ip_len;

    // Boundary check: ensure transport header has at least 4 bytes (containing source/dest ports)
    if (trans_header + 4 > data_end) {
        // No port info available, fall back to IP/Peer level
        goto ip_level_check;
    }

    __u16 src_port = 0;
    __u16 dst_port = 0;
    __u8  proto = ip->protocol;

    // 3. Extract ports based on protocol
    if (proto == IPPROTO_TCP) {
        struct tcphdr *tcp = trans_header;
        // Additional boundary check for complete TCP header
        if ((void *)(tcp + 1) > data_end) {
            goto ip_level_check;
        }
        src_port = tcp->source;
        dst_port = tcp->dest;
    } else if (proto == IPPROTO_UDP) {
        struct udphdr *udp = trans_header;
        if ((void *)(udp + 1) > data_end) {
            goto ip_level_check;
        }
        src_port = udp->source;
        dst_port = udp->dest;
    } else {
        // ICMP or other protocols without ports concept
        goto ip_level_check;
    }

    // 4. Construct five-tuple Key (Service-level QoS)
    struct flow_key key = {
        .src_ip = src_ip,
        .dst_ip = dst_ip,
        .src_port = src_port, // Keep network byte order (Big Endian)
        .dst_port = dst_port,
        .proto = proto,
        .pad = {0, 0, 0}
    };

    struct bucket_state *bucket = NULL;
    int decision = TC_ACT_OK;

    // 5. Check table (Service-level QoS) - Priority 1: Most Granular
    bucket = bpf_map_lookup_elem(&service_rate_map, &key);
    if (bucket) {
        // Found service-level rule, enforce it directly
        goto apply_rate_limit;
    }

ip_level_check:
    // Level 2: Peer-level QoS (SrcIP + DstIP)
    struct peer_pair peer_key = {};
    peer_key.src_ip = src_ip;
    peer_key.dst_ip = dst_ip;

    bucket = bpf_map_lookup_elem(&peer_rate_map, &peer_key);

    // Level 3: IP-level QoS (Fallback)
    if (!bucket) {
        bucket = bpf_map_lookup_elem(&ip_rate_map, &src_ip);
        if (!bucket) {
            // No rate limit rule for this traffic, allow it
            return TC_ACT_OK;
        }
    }

apply_rate_limit:
    // Acquire spinlock to ensure atomic operations for concurrent access
    // This is critical for multi-interface aggregation scenarios
    bpf_spin_lock(&bucket->lock);

    __u64 now = bpf_ktime_get_ns();
    __u64 time_passed = now - bucket->last_time;

    // Limit the time window to prevent token overflow
    if (time_passed > 1000000000) { // 1 second max
        time_passed = 1000000000;
    }

    // Add tokens based on elapsed time and configured rate
    __u64 tokens_to_add = time_passed * bucket->rate;
    __u64 new_tokens = bucket->tokens + tokens_to_add;

    // Cap tokens at bucket capacity
    if (new_tokens > bucket->capacity) {
        new_tokens = bucket->capacity;
    }

    // Check if we have enough tokens for this packet
    if (new_tokens >= pkt_len) {
        // Deduct tokens and allow packet
        bucket->tokens = new_tokens - pkt_len;
        bucket->last_time = now;
    } else {
        // Not enough tokens, drop the packet
        bucket->tokens = new_tokens;  // Keep remaining tokens
        bucket->last_time = now;
        decision = TC_ACT_SHOT;  // Drop the packet
    }

    // Release spinlock
    bpf_spin_unlock(&bucket->lock);

    return decision;
}

// Helper function to initialize a bucket (for external use)
static __always_inline void init_bucket(struct bucket_state *bucket, __u64 rate, __u64 capacity) {
    bucket->rate = rate;
    bucket->tokens = capacity;  // Start with full bucket
    bucket->last_time = bpf_ktime_get_ns();
    bucket->capacity = capacity;
    // lock is initialized to 0 by default
}

// Tracepoint for debugging - allows external programs to initialize buckets
SEC("tracepoint/bpf_prog_init")
int trace_init_bucket(struct trace_event_raw_bpf_prog_init_args *ctx) {
    // This is a stub - real initialization happens from userspace
    return 0;
}

char LICENSE[] SEC("license") = "GPL";