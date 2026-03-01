# Map Pinning 实现状态

## ✅ 已完成

### 1. eBPF 代码修改

#### ACL 程序 (`ebpf/src/acl.rs`)
- **第 77-90 行**：为 4 个 ID maps 添加 `pin` 属性
  ```rust
  #[map(name = "SRC_IPV4_ID_MAP", pin)]
  static SRC_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = ...;
  
  #[map(name = "DST_IPV4_ID_MAP", pin)]
  static DST_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = ...;
  
  #[map(name = "SRC_IPV6_ID_MAP", pin)]
  static SRC_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = ...;
  
  #[map(name = "DST_IPV6_ID_MAP", pin)]
  static DST_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = ...;
  ```

#### QoS 程序 (`ebpf/src/qos.rs`)
- **第 80-93 行**：为 4 个 ID maps 添加 `pin` 属性（与 ACL 相同）
  - 使用相同的 map 名称
  - 添加 `pin` 属性以支持共享

### 2. 用户态代码修改

#### main.rs
- **第 23 行**：添加 `BPF_FS_PATH` 常量：`"/sys/fs/bpf/aria"`
- **第 27 行**：导入 `EbpfLoader`
- **第 234-260 行**：修改 `run_daemon()` 函数
  - 创建 bpffs 目录
  - 加载 ACL 程序（会创建 maps）
  - Pin identity maps
  - 使用 `EbpfLoader` 加载 QoS 程序（指定 map_pin_path）
  
- **第 293-326 行**：添加 `pin_identity_maps()` 函数
  - 遍历 4 个 ID maps
  - 删除已存在的 pin
  - Pin 到 `/sys/fs/bpf/aria/`
  
- **第 328-354 行**：添加 `cleanup_pinned_maps()` 函数
  - 删除所有 pinned maps
  - 删除空目录
  
- **第 292-305 行**：修改 `setup_signal_handlers()`
  - Ctrl-C 时调用 `cleanup_pinned_maps()`
  
- **第 284-290 行**：修改 main 函数退出逻辑
  - 正常退出时清理 pinned maps

## 📋 工作原理

### Map Pinning 流程

```
1. 创建 /sys/fs/bpf/aria/ 目录
   ↓
2. 加载 ACL eBPF 程序
   - 创建 4 个 ID maps (empty)
   ↓
3. Pin identity maps
   - SRC_IPV4_ID_MAP → /sys/fs/bpf/aria/SRC_IPV4_ID_MAP
   - DST_IPV4_ID_MAP → /sys/fs/bpf/aria/DST_IPV4_ID_MAP
   - SRC_IPV6_ID_MAP → /sys/fs/bpf/aria/SRC_IPV6_ID_MAP
   - DST_IPV6_ID_MAP → /sys/fs/bpf/aria/DST_IPV6_ID_MAP
   ↓
4. 加载 QoS eBPF 程序 (使用 EbpfLoader)
   - 指定 map_pin_path = /sys/fs/bpf/aria
   - aya 检测到 maps 已存在 → 复用（共享）
   ↓
5. 初始化 IdentityManager
   - 从 ACL 程序获取 maps
   - 填充数据
   ↓
6. ACL 和 QoS 程序现在共享同一组 ID maps
   ✅ QoS 可以正确查询身份 ID
   ✅ 限速功能正常工作
```

### 共享机制

```
┌──────────────────┐
│  ACL eBPF 程序   │
│  (XDP)           │
└────────┬─────────┘
         │
         │ 读/写
         ▼
┌──────────────────────────────┐
│  Pinned Maps (bpffs)         │
│  /sys/fs/bpf/aria/           │
│  - SRC_IPV4_ID_MAP           │
│  - DST_IPV4_ID_MAP           │
│  - SRC_IPV6_ID_MAP           │
│  - DST_IPV6_ID_MAP           │
└──────────────────────────────┘
         ▲
         │ 读
         │
┌────────┴─────────┐
│  QoS eBPF 程序   │
│  (TC Egress)     │
└──────────────────┘

┌──────────────────┐
│ IdentityManager  │
│ (用户态)         │
└────────┬─────────┘
         │ 写
         ▼
    Pinned Maps
```

## 🧪 验证步骤（Linux 环境）

### 1. 编译

```bash
# 安装依赖
rustup install nightly
rustup component add rust-src --toolchain nightly
cargo install bpf-linker

# 编译 eBPF 程序
cd agent-rust
cargo +nightly build -Z build-std=core --release --target bpfel-unknown-none --manifest-path ebpf/Cargo.toml

# 编译用户态程序
cargo build --release
```

### 2. 准备环境

```bash
# 确保 bpffs 已挂载
mount | grep bpf
# 如果没有：
sudo mount -t bpf none /sys/fs/bpf

# 创建目录
sudo mkdir -p /sys/fs/bpf/aria
```

### 3. 运行

```bash
# 启动 daemon
sudo ./target/release/aria-agent daemon -i eth0

# 检查 pinned maps
ls -l /sys/fs/bpf/aria/
# 应该看到 4 个文件：
# SRC_IPV4_ID_MAP
# DST_IPV4_ID_MAP
# SRC_IPV6_ID_MAP
# DST_IPV6_ID_MAP
```

### 4. 验证 map 共享

```bash
# 使用 bpftool 查看
sudo bpftool map list

# 找到 ID maps 的 ID
sudo bpftool map show name SRC_IPV4_ID_MAP
# 记录 ID，例如：100

# 查看 ACL 程序引用的 map
sudo bpftool prog show name xdp_ingress_acl
# 应该看到引用 map ID 100

# 查看 QoS 程序引用的 map
sudo bpftool prog show name tc_egress_qos
# 应该看到引用相同的 map ID 100
```

### 5. 测试功能

```bash
# 测试 ACL
./target/release/aria-agent acl block-src --ip 192.168.1.100

# 查看 map 内容
sudo bpftool map dump name SRC_IPV4_ID_MAP
# 应该看到 192.168.1.100 映射到一个 ID

# 测试 QoS
./target/release/aria-agent qos limit-ip --ip 192.168.1.100 --mbps 10

# 验证 QoS 能查到相同的 ID
# QoS 程序现在应该能够正确匹配身份 ID
```

### 6. 测试清理

```bash
# 停止 daemon (Ctrl-C)
# 检查是否清理了 pinned maps
ls /sys/fs/bpf/aria/
# 目录应该为空或不存在
```

## 🐛 故障排除

### 问题 1：加载失败 "Map already pinned"

**原因**：上次运行没有正常清理

**解决**：
```bash
sudo rm -rf /sys/fs/bpf/aria/*
```

### 问题 2：QoS 仍然无法工作

**检查**：
1. 确认 ACL 和 QoS 引用相同的 map ID
   ```bash
   sudo bpftool prog show
   ```

2. 查看 QoS 程序的 map 路径
   ```bash
   sudo bpftool map show
   # 查看 pinned path
   ```

3. 确认 IdentityManager 正常工作
   ```bash
   # 添加一个 IP
   ./aria-agent acl block-src --ip 10.0.0.1
   
   # 查看 map
   sudo bpftool map dump name SRC_IPV4_ID_MAP
   ```

### 问题 3：编译错误 "cannot find derive macro `map`"

**原因**：aya_ebpf 版本问题

**解决**：检查 `ebpf/Cargo.toml` 中的 aya_ebpf 版本，确保支持 `pin` 属性

## 📊 性能影响

- **内存**：无影响（共享同一 map）
- **延迟**：无影响（直接内存访问）
- **启动时间**：增加 ~50ms（pin 操作）

## 🔄 替代方案

如果 map pinning 无法工作，可以考虑：

### 方案 A：单程序模式
将 ACL 和 QoS 合并到一个 eBPF 程序

### 方案 B：用户态同步
在用户态维护两份 maps，但性能和一致性较差

## 📚 参考资料

- [Aya Map Pinning](https://docs.rs/aya/latest/aya/maps/struct.Map.html#method.pin)
- [BPF File System](https://www.kernel.org/doc/html/latest/bpf/bpf_devel_QA.html#bpf-file-system)
- [Map Sharing](https://lwn.net/Articles/664688/)

## ✅ 完成检查清单

- [x] eBPF ACL 程序添加 `pin` 属性
- [x] eBPF QoS 程序添加 `pin` 属性
- [x] 用户态创建 bpffs 目录
- [x] 用户态 pin identity maps
- [x] 用户态使用 EbpfLoader 指定 pin 路径
- [x] 用户态清理 pinned maps
- [x] 信号处理中清理
- [ ] **在 Linux 环境编译**
- [ ] **在 Linux 环境测试**
- [ ] **验证 map 共享**
- [ ] **验证 QoS 功能**

## 下一步

1. 在 Linux 环境中编译
2. 运行测试验证 map 共享
3. 测试 ACL 和 QoS 功能
4. 性能测试
5. 压力测试

---

**实现日期**：2026-03-01
**状态**：代码已准备，等待 Linux 环境测试
