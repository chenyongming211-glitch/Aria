# Aria Agent (Rust) - 基于身份 ID 的策略引擎

Rust 实现的 Aria Agent，使用 Aya 框架进行 eBPF 开发。

## 核心特性

- **基于身份 ID 的策略** - IP/CIDR 映射为统一的 32 位 ID，策略与拓扑解耦
- **IPv4/IPv6 双栈** - 完整支持 IPv4 和 IPv6，使用 LPM_TRIE 进行 CIDR 匹配
- **XDP ACL** - 高性能入向过滤（基于 ID 的五元组规则）
- **TC QoS** - 出向 Token Bucket 限速（基于 ID）
- **CIDR 支持** - 支持单 IP 和网段级别的策略
- **常驻 Daemon** - Unix socket IPC 通信
- **原子更新** - 无锁统计和令牌桶更新

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                      控制平面 (Go)                           │
│  - 分配 ID                                                   │
│  - 维护 IP/CIDR → ID 映射                                    │
│  - 下发策略 (基于 ID)                                        │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Rust Agent (Daemon)                     │
│  ┌─────────────────┐  ┌─────────────────┐                   │
│  │ IdentityManager │  │  ACL/QoS Mgr    │                   │
│  │ - IP → ID 映射   │  │  - 基于ID的策略  │                   │
│  └─────────────────┘  └─────────────────┘                   │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ eBPF Maps
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     eBPF 数据平面                            │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │ LPM_TRIE      │  │ Policy Map    │  │ QoS Maps      │   │
│  │ IPv4/IPv6     │  │ (src_id,      │  │ (src_id,      │   │
│  │ → ID 映射      │  │  dst_id, ...) │  │  dst_id, ...) │   │
│  └───────────────┘  └───────────────┘  └───────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## 数据结构

### 身份映射 (LPM_TRIE)

```rust
// IPv4 CIDR → ID
struct Ipv4LpmKey {
    prefixlen: u32,  // 0-32
    ip: u32,         // 网络字节序
}

// IPv6 CIDR → ID
struct Ipv6LpmKey {
    prefixlen: u32,  // 0-128
    ip: [u8; 16],    // 128位地址
}
```

### 策略 Key (基于 ID)

```rust
struct PolicyKey {
    src_id: u32,     // 源身份 ID (0 = 通配)
    dst_id: u32,     // 目的身份 ID (0 = 通配)
    dst_port: u16,   // 目的端口 (0 = 通配)
    protocol: u8,    // 协议 (0 = 通配)
}
```

### QoS Key (基于 ID)

```rust
// 按源 ID 限速
src_id: u32

// 按点对点限速
struct PairQoSKey {
    src_id: u32,
    dst_id: u32,
}

// 按服务限速
struct ServiceQoSKey {
    src_id: u32,
    dst_id: u32,
    dst_port: u16,
    protocol: u8,
}
```

## eBPF Maps

### 身份映射 Maps (ACL & QoS 共享)

**关键**：ACL 和 QoS 程序共享相同的身份映射表，通过 eBPF map pinning 机制实现。

| Map | 类型 | Key | Value | 用途 |
|-----|------|-----|-------|------|
| SRC_IPV4_ID_MAP | LPM_TRIE | Ipv4LpmKey | u32 | IPv4 源 IP → ID |
| DST_IPV4_ID_MAP | LPM_TRIE | Ipv4LpmKey | u32 | IPv4 目的 IP → ID |
| SRC_IPV6_ID_MAP | LPM_TRIE | Ipv6LpmKey | u32 | IPv6 源 IP → ID |
| DST_IPV6_ID_MAP | LPM_TRIE | Ipv6LpmKey | u32 | IPv6 目的 IP → ID |

这些 map 被 pin 到 `/sys/fs/bpf/aria/`，ACL 和 QoS 程序都从同一位置获取。

### ACL Maps

| Map | 类型 | Key | Value | 用途 |
|-----|------|-----|-------|------|
| SRC_IPV4_ID_MAP | LPM_TRIE | Ipv4LpmKey | u32 | IPv4 源 IP → ID |
| DST_IPV4_ID_MAP | LPM_TRIE | Ipv4LpmKey | u32 | IPv4 目的 IP → ID |
| SRC_IPV6_ID_MAP | LPM_TRIE | Ipv6LpmKey | u32 | IPv6 源 IP → ID |
| DST_IPV6_ID_MAP | LPM_TRIE | Ipv6LpmKey | u32 | IPv6 目的 IP → ID |
| POLICY_MAP | HASH | PolicyKey | PolicyValue | 五元组策略 |
| BLOCK_SRC_ID_MAP | HASH | u32 | u8 | 封锁源 ID |
| BLOCK_DST_ID_MAP | HASH | u32 | u8 | 封锁目的 ID |
| BLOCK_PORT_MAP | HASH | u16 | u8 | 封锁端口 |

### QoS Maps

| Map | 类型 | Key | Value | 用途 |
|-----|------|-----|-------|------|
| SRC_ID_QOS_MAP | HASH | u32 | BucketState | 按源 ID 限速 |
| PAIR_ID_QOS_MAP | HASH | PairQoSKey | BucketState | 按点对点限速 |
| SERVICE_QOS_MAP | HASH | ServiceQoSKey | BucketState | 按服务限速 |

## 处理流程

### ACL (XDP)

```
1. 解析以太网头 → 判断 IPv4/IPv6
2. 解析 IP 头 → 获取 src_ip, dst_ip, protocol
3. LPM 查询: src_ip → src_id, dst_ip → dst_id
4. 检查 BLOCK_SRC_ID_MAP/BLOCK_DST_ID_MAP
5. 组装 PolicyKey { src_id, dst_id, dst_port, protocol }
6. 查询 POLICY_MAP → 决定 PASS/DROP
```

### QoS (TC Egress)

```
1. 解析以太网头 → 判断 IPv4/IPv6
2. 解析 IP 头 → 获取 src_ip, dst_ip, protocol
3. LPM 查询: src_ip → src_id, dst_ip → dst_id
4. 按优先级查找限速规则:
   - SERVICE_QOS_MAP (五元组)
   - PAIR_ID_QOS_MAP (点对点)
   - SRC_ID_QOS_MAP (源 ID)
5. Token Bucket 判断 → PASS/SHOT
```

## 构建要求

- Rust nightly (用于 eBPF 编译)
- bpf-linker
- libbpf 开发库
- Linux 内核 5.4+ (支持 eBPF)

## 构建

```bash
# 安装 nightly 和 rust-src
rustup install nightly
rustup component add rust-src --toolchain nightly

# 安装 bpf-linker
cargo install bpf-linker

# 构建 eBPF 程序
cargo +nightly build -Z build-std=core --release --target bpfel-unknown-none --manifest-path ebpf/Cargo.toml

# 构建用户态程序
cargo build --release
```

## Map Pinning 实现（关键）

ACL 和 QoS 程序需要共享相同的身份映射表。实现步骤：

### 1. eBPF Map 定义

在 eBPF 程序中为需要共享的 map 添加 pin 属性：

```rust
// ebpf/src/acl.rs
#[map(pin)]
static SRC_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(pin)]
static DST_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(pin)]
static SRC_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(pin)]
static DST_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);
```

### 2. 用户态代码处理

```rust
// agent/src/main.rs
use aya::maps::Map;

const BPF_FS_PATH: &str = "/sys/fs/bpf/aria";

fn run_daemon(interface: &str) -> Result<()> {
    // 创建 bpffs 目录
    std::fs::create_dir_all(BPF_FS_PATH)?;
    
    // 加载 ACL 程序
    let mut acl_ebpf = Ebpf::load(acl_bytes)?;
    
    // Pin 身份映射 maps
    pin_identity_maps(&mut acl_ebpf)?;
    
    // 加载 QoS 程序（会自动从 pin 路径获取 maps）
    let mut qos_ebpf = Ebpf::load(qos_bytes)?;
    
    // ...
}

fn pin_identity_maps(ebpf: &mut Ebpf) -> Result<()> {
    let maps = ["SRC_IPV4_ID_MAP", "DST_IPV4_ID_MAP", 
                "SRC_IPV6_ID_MAP", "DST_IPV6_ID_MAP"];
    
    for map_name in maps {
        let map = ebpf.map_mut(map_name)?;
        let pin_path = format!("{}/{}", BPF_FS_PATH, map_name);
        map.pin(&pin_path)?;
    }
    
    Ok(())
}
```

### 3. 验证

```bash
# 检查 maps 是否正确 pin
ls /sys/fs/bpf/aria/
# 应该看到:
# SRC_IPV4_ID_MAP  DST_IPV4_ID_MAP  SRC_IPV6_ID_MAP  DST_IPV6_ID_MAP

# 使用 bpftool 查看
bpftool map list
bpftool map show name SRC_IPV4_ID_MAP
```

## CLI 命令

### 身份管理

```bash
# 分配 ID（自动）
# 通常在设置策略时自动分配

# 查看所有 ID 映射
./aria-agent identity list
```

### ACL 策略

```bash
# 封锁源 CIDR
./aria-agent acl block-src --cidr 192.168.1.0/24
./aria-agent acl block-src --cidr 2001:db8::/32

# 封锁目的 CIDR
./aria-agent acl block-dst --cidr 10.0.0.0/8

# 封锁端口
./aria-agent acl block-port --port 8080

# 添加允许规则（基于 CIDR）
./aria-agent acl allow --src 192.168.1.0/24 --dst 10.0.0.1/32 --port 443 --protocol 6

# 添加拒绝规则
./aria-agent acl deny --src 0.0.0.0/0 --dst 10.0.0.1/32 --port 22 --protocol 6

# 移除规则
./aria-agent acl remove --src 192.168.1.0/24 --dst 10.0.0.1/32 --port 443 --protocol 6
```

### QoS 限速

```bash
# 按源 CIDR 限速
./aria-agent qos limit-src --cidr 192.168.1.0/24 --mbps 100
./aria-agent qos limit-src --cidr 2001:db8::/32 --mbps 100

# 按点对点限速
./aria-agent qos limit-pair --src 192.168.1.0/24 --dst 10.0.0.0/8 --mbps 50

# 按服务限速
./aria-agent qos limit-service --dst 10.0.0.1/32 --port 443 --protocol 6 --mbps 20

# 查看统计
./aria-agent qos stats-src --cidr 192.168.1.0/24
./aria-agent qos stats-pair --src 192.168.1.0/24 --dst 10.0.0.0/8
```

## 通配规则

- `src_id = 0` 或 `src_cidr = "0"` → 匹配任意源
- `dst_id = 0` 或 `dst_cidr = "0"` → 匹配任意目的
- `port = 0` → 匹配任意端口
- `protocol = 0` → 匹配任意协议

## 服务限速说明

**重要**：服务限速基于 `(源ID, 目的ID, 目的端口, 协议)` 五元组，**源端口参数被忽略**。

原因：
- 服务通常通过目的端口识别（如 HTTP:80, HTTPS:443）
- 限速目的端口可以控制特定服务的带宽
- 源端口通常是临时端口，不适合作为限速依据

示例：
```bash
# 限制访问 10.0.0.1:443 的带宽为 20 Mbps
./aria-agent qos limit-service --dst 10.0.0.1/32 --port 443 --protocol 6 --mbps 20

# 限制从 192.168.1.0/24 到 10.0.0.1:443 的带宽
./aria-agent qos limit-service --src 192.168.1.0/24 --dst 10.0.0.1/32 --port 443 --protocol 6 --mbps 20
```

## IPv6 扩展头限制

**当前限制**：eBPF 程序假设 IPv6 数据包无扩展头（Hop-by-Hop、Routing、Fragment 等）。

适用环境：
- ✅ 云环境/数据中心（通常无扩展头）
- ✅ 内部网络
- ❌ 复杂的 IPv6 部署（可能有扩展头）

如需支持扩展头，需要：
1. 在 eBPF 中实现扩展头跳过循环
2. 解析 Next Header 字段和 Header Ext Length
3. 限制最大跳过次数（防止无限循环）

## 设计优势

1. **拓扑无关** - 策略基于身份 ID，IP 变化不影响策略
2. **高效查找** - LPM_TRIE O(log n) 查找，HASH O(1) 策略匹配
3. **IPv4/IPv6 统一** - 统一的 ID 空间，无需分别处理
4. **CIDR 支持** - 原生支持网段级别的策略
5. **可扩展** - 32 位 ID 空间支持约 42 亿身份

## 状态

- [x] LPM_TRIE IP → ID 映射
- [x] 基于 ID 的 ACL 策略
- [x] 基于 ID 的 QoS 限速
- [x] IPv4/IPv6 双栈支持
- [x] CIDR 网段支持
- [x] 通配规则
- [x] 身份 ID 全局唯一性
- [x] Arc<Mutex> 安全共享
- [ ] **Map Pinning（QoS 需要实现）**
- [ ] CLI 命令完善
- [ ] 与 Controller gRPC 通信

## 已知限制

1. **QoS 程序 ID 映射未共享**（当前阻塞）
   - 需要实现 map pinning
   - 否则 QoS 限速功能无法工作

2. **IPv6 扩展头**
   - 当前不支持
   - 适用于云环境/数据中心

3. **服务限速**
   - 仅基于目的端口
   - 源端口参数被忽略

4. **统计准确性**
   - 高并发下可能有轻微误差
   - 需要精确统计可使用 PERCPU map
- [ ] WireGuard 隧道管理
