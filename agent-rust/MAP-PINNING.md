# Map Pinning 实施指南

## 问题

ACL 和 QoS 程序各自加载独立的 eBPF map 实例，导致：
- IdentityManager 仅初始化 ACL 程序的 ID maps
- QoS 程序的 ID maps 始终为空
- QoS 查询永远返回 `ID_WILDCARD`，限速功能失效

## 解决方案

使用 eBPF map pinning 机制，将 ID maps pin 到 bpffs，两个程序共享同一实例。

## 实施步骤

### 步骤 1：修改 eBPF Map 定义

在 `ebpf/src/acl.rs` 中为 ID maps 添加 `pin` 属性：

```rust
#[map(pin)]  // 添加此属性
static SRC_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(pin)]
static DST_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(pin)]
static SRC_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(pin)]
static DST_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);
```

**注意**：QoS 程序 (`ebpf/src/qos.rs`) 中的同名 maps 也需要添加 `#[map(pin)]`。

### 步骤 2：修改用户态初始化代码

在 `agent/src/main.rs` 中：

```rust
use aya::maps::Map;
use std::path::Path;

const BPF_FS_PATH: &str = "/sys/fs/bpf/aria";

fn run_daemon(interface: &str) -> Result<()> {
    info!("Starting Aria Agent daemon on interface: {}", interface);

    let acl_bytes = include_bytes_aligned!(concat!(env!("OUT_DIR"), "/acl"));
    let qos_bytes = include_bytes_aligned!(concat!(env!("OUT_DIR"), "/qos"));

    // 创建 bpffs 目录
    if !Path::new(BPF_FS_PATH).exists() {
        std::fs::create_dir_all(BPF_FS_PATH)?;
    }

    // 1. 加载 ACL 程序
    let mut acl_ebpf = Ebpf::load(acl_bytes)?;
    attach_xdp(&mut acl_ebpf, interface)?;

    // 2. Pin 身份映射 maps
    pin_identity_maps(&mut acl_ebpf)?;
    info!("Pinned identity maps to {}", BPF_FS_PATH);

    // 3. 加载 QoS 程序（会从 pin 路径获取共享的 maps）
    let mut qos_ebpf = Ebpf::load(qos_bytes)?;
    attach_tc(&mut qos_ebpf, interface)?;

    // 4. 初始化 IdentityManager（使用 ACL 的 maps）
    let identity_mgr = identity::IdentityManager::new(&mut acl_ebpf)?;
    let identity_mgr = Arc::new(Mutex::new(identity_mgr));

    // 5. 初始化管理器
    let acl_mgr = acl::AclManager::new(&mut acl_ebpf, identity_mgr.clone())?;
    let qos_mgr = qos::QoSManager::new(&mut qos_ebpf, identity_mgr.clone())?;

    // ... 其余代码
}

fn pin_identity_maps(ebpf: &mut Ebpf) -> Result<()> {
    let map_names = [
        "SRC_IPV4_ID_MAP",
        "DST_IPV4_ID_MAP",
        "SRC_IPV6_ID_MAP",
        "DST_IPV6_ID_MAP",
    ];

    for map_name in map_names {
        let map = ebpf
            .map_mut(map_name)
            .context(format!("Map {} not found", map_name))?;
        
        let pin_path = format!("{}/{}", BPF_FS_PATH, map_name);
        
        // 如果已经存在，先删除
        if Path::new(&pin_path).exists() {
            std::fs::remove_file(&pin_path)?;
        }
        
        map.pin(&pin_path)
            .context(format!("Failed to pin map {}", map_name))?;
        
        info!("Pinned {} to {}", map_name, pin_path);
    }

    Ok(())
}
```

### 步骤 3：清理逻辑

添加 daemon 关闭时的清理逻辑：

```rust
fn cleanup_pinned_maps() -> Result<()> {
    let map_names = [
        "SRC_IPV4_ID_MAP",
        "DST_IPV4_ID_MAP",
        "SRC_IPV6_ID_MAP",
        "DST_IPV6_ID_MAP",
    ];

    for map_name in map_names {
        let pin_path = format!("{}/{}", BPF_FS_PATH, map_name);
        if Path::new(&pin_path).exists() {
            std::fs::remove_file(&pin_path)?;
            info!("Removed pinned map: {}", pin_path);
        }
    }

    // 删除目录
    if Path::new(BPF_FS_PATH).exists() {
        std::fs::remove_dir(BPF_FS_PATH)?;
    }

    Ok(())
}
```

在 main 函数的退出处理中调用：

```rust
// 在 setup_signal_handlers 中
ctrlc::set_handler(move || {
    info!("Received shutdown signal");
    RUNNING.store(false, Ordering::SeqCst);
    let _ = cleanup_pinned_maps();
}).expect("Error setting Ctrl-C handler");
```

## 验证步骤

### 1. 检查 bpffs 挂载

```bash
mount | grep bpf
# 应该看到:
# none on /sys/fs/bpf type bpf (rw,relatime)
```

如果没有挂载：
```bash
mount -t bpf none /sys/fs/bpf
```

### 2. 启动 daemon 并检查

```bash
# 启动 daemon
sudo ./aria-agent daemon -i eth0

# 检查 pinned maps
ls -l /sys/fs/bpf/aria/
# 应该看到:
# SRC_IPV4_ID_MAP
# DST_IPV4_ID_MAP
# SRC_IPV6_ID_MAP
# DST_IPV6_ID_MAP

# 使用 bpftool 查看
sudo bpftool map list
sudo bpftool map show name SRC_IPV4_ID_MAP
```

### 3. 测试 QoS 功能

```bash
# 添加限速规则
./aria-agent qos limit-ip --ip 192.168.1.100 --mbps 10

# 检查 ID 分配（应该能看到 ID）
./aria-agent identity list

# 测试限速是否生效
# 使用 iperf3 或 curl 测试带宽
```

## 故障排除

### 问题 1：Pin 失败

```
Error: Failed to pin map SRC_IPV4_ID_MAP: Permission denied
```

**解决**：
- 使用 `sudo` 运行
- 检查 `/sys/fs/bpf` 权限

### 问题 2：Map 已存在

```
Error: File exists
```

**解决**：
- 手动删除旧的 pins：`sudo rm -rf /sys/fs/bpf/aria/*`
- 或在代码中添加删除逻辑（已在示例中包含）

### 问题 3：QoS 仍然无效

**检查**：
1. 确认 ACL 和 QoS 的 map ID 相同
   ```bash
   sudo bpftool map show name SRC_IPV4_ID_MAP
   # 记录 ID
   
   # 查看两个程序使用的 map
   sudo bpftool prog show name xdp_ingress_acl
   sudo bpftool prog show name tc_egress_qos
   # 确认它们引用相同的 map ID
   ```

2. 检查 IdentityManager 是否正常工作
   ```bash
   # 添加一个 IP
   ./aria-agent acl block-src --ip 192.168.1.100
   
   # 查看 map 内容
   sudo bpftool map dump name SRC_IPV4_ID_MAP
   ```

## 替代方案

如果 map pinning 实现困难，可以考虑：

### 方案 A：单程序模式

将 ACL 和 QoS 合并到一个 eBPF 程序中：
- 优点：无需 map 共享
- 缺点：无法使用 XDP (ACL) + TC (QoS) 的最佳组合

### 方案 B：用户态同步

在用户态维护两份 maps：
- 优点：实现简单
- 缺点：
  - 性能开销（每次更新需要写两次）
  - 一致性问题（如果一次更新失败）

## 参考资料

- [Aya Map Pinning Documentation](https://docs.rs/aya/latest/aya/maps/struct.Map.html#method.pin)
- [BPF Documentation - Map Pinning](https://www.kernel.org/doc/html/latest/bpf/maps.html#map-pinning)
- [Cilium Map Sharing Example](https://github.com/cilium/cilium/blob/main/bpf/lib/maps.h)

## 下一步

1. 实现 map pinning（优先级：高）
2. 在 Linux 环境中编译和测试
3. 添加集成测试验证 QoS 功能
4. 性能测试（map 访问延迟）
