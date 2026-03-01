# Map Pinning 完整实现 - 代码修改清单

## 修改概览

| 类别 | 文件 | 修改内容 | 行数 |
|------|------|---------|------|
| eBPF | `ebpf/src/acl.rs` | 添加 map pinning 属性 | 14 |
| eBPF | `ebpf/src/qos.rs` | 添加 map pinning 属性 | 14 |
| 用户态 | `agent/src/main.rs` | 实现 pinning 逻辑 | 140+ |
| 文档 | `MAP-PINNING-STATUS.md` | 完整实施指南 | 280+ |

## 详细修改

### 1. eBPF 程序修改

#### `ebpf/src/acl.rs` (第 77-90 行)

**修改前：**
```rust
#[map]
static SRC_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map]
static DST_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map]
static SRC_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map]
static DST_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);
```

**修改后：**
```rust
// Identity maps - shared with QoS program via pinning
#[map(name = "SRC_IPV4_ID_MAP", pin)]
static SRC_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV4_ID_MAP", pin)]
static DST_IPV4_ID_MAP: LpmTrie<Ipv4LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "SRC_IPV6_ID_MAP", pin)]
static SRC_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);

#[map(name = "DST_IPV6_ID_MAP", pin)]
static DST_IPV6_ID_MAP: LpmTrie<Ipv6LpmKey, u32> = LpmTrie::with_max_entries(10000, 0);
```

**关键变化：**
- 添加 `name = "..."` 确保两个程序使用相同的 map 名称
- 添加 `pin` 属性启用 map pinning
- 添加注释说明这些 maps 将与 QoS 程序共享

#### `ebpf/src/qos.rs` (第 80-93 行)

**修改内容与 ACL 完全相同**

**目的：**
- QoS 程序使用相同的 map 名称
- 当加载时，aya 会检测到 maps 已存在并复用

### 2. 用户态代码修改

#### `agent/src/main.rs`

##### 修改 1：导入和常量 (第 1-30 行)

**添加：**
```rust
use aya::{
    programs::{tc, SchedClassifier, TcAttachType, Xdp, XdpFlags},
    Ebpf,
    EbpfLoader,  // ← 新增
};

const BPF_FS_PATH: &str = "/sys/fs/bpf/aria";  // ← 新增
```

##### 修改 2：run_daemon() 函数 (第 234-260 行)

**新增逻辑：**
```rust
fn run_daemon(interface: &str) -> Result<()> {
    // 1. 创建 bpffs 目录
    if !Path::new(BPF_FS_PATH).exists() {
        std::fs::create_dir_all(BPF_FS_PATH)?;
    }

    // 2. 加载 ACL 程序（创建 maps）
    let mut acl_ebpf = Ebpf::load(acl_bytes)?;
    attach_xdp(&mut acl_ebpf, interface)?;
    
    // 3. Pin identity maps
    pin_identity_maps(&mut acl_ebpf)?;

    // 4. 加载 QoS 程序（复用 pinned maps）
    let mut qos_ebpf = EbpfLoader::new()
        .map_pin_path(BPF_FS_PATH)  // ← 关键：指定 pin 路径
        .load(qos_bytes)?;
    attach_tc(&mut qos_ebpf, interface)?;

    // 5. 初始化 IdentityManager
    let identity_mgr = identity::IdentityManager::new(&mut acl_ebpf)?;
    let identity_mgr = Arc::new(Mutex::new(identity_mgr));

    // 6. 初始化管理器
    let acl_mgr = acl::AclManager::new(&mut acl_ebpf, identity_mgr.clone())?;
    let qos_mgr = qos::QoSManager::new(&mut qos_ebpf, identity_mgr.clone())?;
    
    // ...
}
```

##### 修改 3：新增 pin_identity_maps() 函数 (第 293-326 行)

```rust
fn pin_identity_maps(ebpf: &mut Ebpf) -> Result<()> {
    let map_names = [
        "SRC_IPV4_ID_MAP",
        "DST_IPV4_ID_MAP",
        "SRC_IPV6_ID_MAP",
        "DST_IPV6_ID_MAP",
    ];

    for map_name in map_names {
        let map = ebpf.map_mut(map_name)?;
        let pin_path = format!("{}/{}", BPF_FS_PATH, map_name);
        
        // 删除已存在的 pin
        if Path::new(&pin_path).exists() {
            std::fs::remove_file(&pin_path)?;
        }
        
        // Pin map
        map.pin(&pin_path)?;
    }

    Ok(())
}
```

**功能：**
- 遍历 4 个 ID maps
- 删除旧的 pin（如果存在）
- Pin 到 `/sys/fs/bpf/aria/`

##### 修改 4：新增 cleanup_pinned_maps() 函数 (第 328-354 行)

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
        }
    }

    // 删除空目录
    if Path::new(BPF_FS_PATH).exists() {
        if std::fs::read_dir(BPF_FS_PATH)?.count() == 0 {
            std::fs::remove_dir(BPF_FS_PATH)?;
        }
    }

    Ok(())
}
```

**功能：**
- 删除所有 pinned maps
- 如果目录为空则删除
- 在程序退出时调用

##### 修改 5：信号处理 (第 292-305 行)

```rust
fn setup_signal_handlers() {
    ctrlc::set_handler(move || {
        info!("Received shutdown signal");
        RUNNING.store(false, Ordering::SeqCst);
        
        // ← 新增：清理 pinned maps
        if let Err(e) = cleanup_pinned_maps() {
            error!("Failed to cleanup pinned maps: {}", e);
        }
    }).expect("Error setting Ctrl-C handler");
}
```

##### 修改 6：正常退出清理 (第 284-290 行)

```rust
    info!("Shutting down daemon...");
    
    // ← 新增：清理 pinned maps
    if let Err(e) = cleanup_pinned_maps() {
        error!("Failed to cleanup pinned maps: {}", e);
    }
    
    if Path::new(SOCKET_PATH).exists() {
        std::fs::remove_file(SOCKET_PATH)?;
    }
```

## 工作原理

### 1. Map Pinning 流程

```
启动 daemon
    ↓
创建 /sys/fs/bpf/aria/
    ↓
加载 ACL eBPF
    ├─ 创建 4 个 ID maps (空)
    └─ attach 到 XDP
    ↓
Pin ID maps
    ├─ SRC_IPV4_ID_MAP → /sys/fs/bpf/aria/SRC_IPV4_ID_MAP
    ├─ DST_IPV4_ID_MAP → /sys/fs/bpf/aria/DST_IPV4_ID_MAP
    ├─ SRC_IPV6_ID_MAP → /sys/fs/bpf/aria/SRC_IPV6_ID_MAP
    └─ DST_IPV6_ID_MAP → /sys/fs/bpf/aria/DST_IPV6_ID_MAP
    ↓
加载 QoS eBPF (EbpfLoader + map_pin_path)
    ├─ aya 检测到 maps 已 pin
    ├─ 复用已存在的 maps（共享）
    └─ attach 到 TC
    ↓
初始化 IdentityManager
    ├─ 从 ACL 获取 maps
    └─ 填充数据（同时影响两个程序）
    ↓
初始化 ACL/QoS Managers
    ↓
✅ 两个程序共享同一组 ID maps
```

### 2. Map 共享验证

```bash
# ACL 和 QoS 应该引用相同的 map IDs
$ sudo bpftool prog show

ID  NAME               TYPE     MAP IDS
100 xdp_ingress_acl    xdp      200,201,202,203,...
101 tc_egress_qos      tc       200,201,202,203,...

# map IDs 200-203 应该相同
$ sudo bpftool map show id 200
200: lpm_trie  name SRC_IPV4_ID_MAP  flags 0x1
    key 8B  value 4B  max_entries 10000
    pinned /sys/fs/bpf/aria/SRC_IPV4_ID_MAP
```

## 测试清单

### 在 Linux 环境中：

- [ ] 编译 eBPF 程序
- [ ] 编译用户态程序
- [ ] 启动 daemon
- [ ] 验证 pinned maps 存在
- [ ] 验证 ACL 和 QoS 共享 maps
- [ ] 测试 ACL 功能
- [ ] 测试 QoS 功能（现在应该能工作）
- [ ] 测试清理逻辑
- [ ] 性能测试

## 关键改进

### Before (不工作)
```
IdentityManager
    ↓ (仅写入)
ACL Maps (独立实例)
    
QoS Maps (独立实例，永远为空)
    ↓
QoS 查询 → ID_WILDCARD → ❌ 无法匹配
```

### After (工作)
```
IdentityManager
    ↓ (写入)
Pinned Maps (共享实例)
    ↑         ↑
    |         |
  ACL      QoS
(读/写)   (读)
    
QoS 查询 → 正确的 ID → ✅ 可以匹配
```

## 预期结果

1. **QoS 功能正常**：能正确查询身份 ID
2. **限速生效**：基于正确的 ID 进行限速
3. **性能无损**：共享 map 不影响性能
4. **资源节省**：只维护一份 ID 映射

## 下一步

1. ✅ 代码修改完成
2. ⏳ Linux 环境编译
3. ⏳ 功能测试
4. ⏳ 性能测试
5. ⏳ 生产部署

---

**完成日期**：2026-03-01
**状态**：✅ 代码准备就绪，等待 Linux 测试
