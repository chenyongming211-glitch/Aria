// Minimal vmlinux.h for eBPF programs
// This is a simplified version for demonstration purposes
// In a real environment, this would be generated from BTF information

#ifndef __VMLINUX_H__
#define __VMLINUX_H__

// Basic types
typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef unsigned long long u64;

typedef signed char s8;
typedef signed short s16;
typedef signed int s32;
typedef signed long long s64;

// Network headers
struct ethhdr {
    unsigned char h_dest[6];
    unsigned char h_source[6];
    unsigned short h_proto;
};

struct iphdr {
    unsigned char ihl:4;
    unsigned char version:4;
    unsigned char tos;
    unsigned short tot_len;
    unsigned short id;
    unsigned short frag_off;
    unsigned char ttl;
    unsigned char protocol;
    unsigned short check;
    unsigned int saddr;
    unsigned int daddr;
};

// eBPF structures
struct xdp_md {
    void *data;
    void *data_end;
    void *data_meta;
    u32 ingress_ifindex;
    u32 rx_queue_index;
    u32 egress_ifindex;
};

struct __sk_buff {
    __u32 len;
    __u32 pkt_type;
    __u32 mark;
    __u32 queue_mapping;
    __u32 protocol;
    __u32 vlan_present;
    __u32 vlan_tci;
    __u32 vlan_proto;
    __u32 priority;
    __u32 ingress_ifindex;
    __u32 ifindex;
    __u32 tc_index;
    __u32 cb[5];
    __u32 hash;
    __u32 tc_classid;
    __u32 data;
    __u32 data_end;
    __u32 napi_id;
    __u32 family;
    __u32 remote_ip4;
    __u32 local_ip4;
    __u32 remote_ip6[4];
    __u32 local_ip6[4];
    __u32 remote_port;
    __u32 local_port;
    __u32 data_meta;
    union {
        struct bpf_tunnel_key *tunnel;
        __u32 tunnel_ext;
    };
    __u32 cgroup_family;
    __u32 cpu;
    __u32 hash_recalc;
    __u32 reserved;
};

// bpf_spin_lock for synchronization
struct bpf_spin_lock {
    u32 val;
};

// BPF helpers (declarations only)
void *bpf_map_lookup_elem(const void *map, const void *key);
long bpf_map_update_elem(void *map, const void *key, const void *value, u64 flags);
long bpf_map_delete_elem(void *map, const void *key);
u64 bpf_ktime_get_ns(void);
void bpf_spin_lock(struct bpf_spin_lock *lock);
void bpf_spin_unlock(struct bpf_spin_lock *lock);

// Constants
#define ETH_P_IP 0x0800

// Endian conversion macros
#define bpf_htons(x) ((__u32)__builtin_bswap16(x))
#define bpf_htonl(x) ((__u32)__builtin_bswap32(x))

#endif /* __VMLINUX_H__ */