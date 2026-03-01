# Aria Agent eBPF 编译指南

## 版本信息

| 组件 | 版本 |
|------|------|
| aya | 0.13.1 |
| aya-ebpf | 0.1.1 |
| aya-build | 0.1.3 |
| aya-log | 0.2 |
| Rust | nightly |

## 常见问题与解决方案

### 1. duplicate lang item in crate `core`: `sized`

**错误信息：**
```
error[E0152]: duplicate lang item in crate `core`: `sized`
  = note: the lang item is first defined in crate `core` (which `aria_agent` depends on)
  = note: first definition in `core` loaded from target/release/deps/libcore-*.rlib
  = note: second definition in `core` loaded from rustlib/x86_64-unknown-linux-gnu/lib/libcore-*.rlib
```

**原因：**
`.cargo/config.toml` 中配置了 `build-std`，导致用户空间程序也使用 build-std 编译，与系统 core 库冲突。

**错误配置示例：**
```toml
# .cargo/config.toml - 这是错误的配置！
[unstable]
build-std = ["core", "alloc", "std"]
build-std-features = ["compiler-builtins-mem"]
```

**解决方案：**
删除 `.cargo/config.toml` 文件，或移除 `build-std` 配置。aya-build 会自动处理 eBPF 的 build-std。

```bash
rm -f .cargo/config.toml
```

### 2. aya-build API 变更

**错误信息：**
```
error[E0425]: cannot find function `build` in crate `aya_build`
```

**原因：**
aya-build 0.1.3 移除了 `build()` 函数，改用 `build_ebpf()` 函数。

**解决方案：**
```rust
// agent/build.rs
fn main() {
    println!("cargo:rerun-if-env-changed=SKIP_EBPF_BUILD");
    
    if std::env::var("SKIP_EBPF_BUILD").is_ok() {
        println!("cargo:warning=Skipping eBPF build");
        return;
    }
    
    let ebpf_package = aya_build::Package {
        name: "aria-ebpf",
        root_dir: "../ebpf",
        ..Default::default()
    };
    
    aya_build::build_ebpf([ebpf_package], aya_build::Toolchain::default()).unwrap();
}
```

### 3. aya API 变更：Ebpf vs Bpf

**错误信息：**
```
error[E0425]: cannot find value `Bpf` in crate `aya`
help: a similar name exists: `Ebpf`
```

**原因：**
aya 0.12 使用 `Bpf`，aya 0.13.1 回归使用 `Ebpf`。

**解决方案（aya 0.13.1）：**
```rust
use aya::{Ebpf, EbpfLoader, include_bytes_aligned};

let mut bpf = Ebpf::load(bytes)?;
let mut bpf = EbpfLoader::new()
    .map_pin_path("/sys/fs/bpf/aria")
    .load(bytes)?;
```

### 4. eBPF panic 配置

**错误信息：**
```
error: unwinding panics are not supported without std
  = help: using nightly cargo, use -Zbuild-std with panic="abort" to avoid unwinding
```

**原因：**
eBPF 程序必须使用 `panic = "abort"`，但 Cargo 默认使用 `panic = "unwind"`。

**解决方案：**
aya-build 会自动处理此问题，不需要在 Cargo.toml 中配置。但确保 eBPF 代码有 panic handler：

```rust
// ebpf/src/qos.rs
#![no_std]
#![no_main]

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
```

### 5. workspace default-members 配置

**问题：**
直接编译时，cargo 会尝试编译所有 workspace 成员，包括 eBPF 程序（需要特殊编译）。

**解决方案：**
在 workspace Cargo.toml 中设置 `default-members`，排除 eBPF 包：

```toml
[workspace]
resolver = "2"
members = ["agent", "ebpf", "shared"]
default-members = ["agent", "shared"]  # 不包含 "ebpf"
```

### 6. eBPF 的 unsafe 使用规范

**aya-ebpf 0.1.x API 安全性：**

| 操作 | 需要 unsafe | 说明 |
|------|------------|------|
| `HashMap::get` | ✅ 需要 | 返回引用可能有别名问题 |
| `HashMap::insert` | ❌ 不需要 | 安全操作 |
| `HashMap::remove` | ❌ 不需要 | 安全操作 |
| `LpmTrie::get` | ❌ 不需要 | 安全操作 |
| `LpmTrie::insert` | ❌ 不需要 | 安全操作 |
| `bpf_ktime_get_ns()` | ✅ 需要 | BPF helper 函数 |
| 指针解引用 | ✅ 需要 | 如 `ptr_at` 函数 |

**正确示例：**
```rust
// HashMap::get 需要 unsafe
if let Some(bucket) = unsafe { SERVICE_QOS_MAP.get(&service_key) } {
    let bucket = *bucket;
    // HashMap::insert 不需要 unsafe
    let _ = SERVICE_QOS_MAP.insert(&service_key, &new_bucket, 0);
}

// LpmTrie::get 不需要 unsafe
if let Some(id) = map.get(&key) {
    return Ok(*id);
}
```

### 7. TC 程序 attach API 变更

**旧 API：**
```rust
let tc_builder = tc::_tc_qdisc_add_clsact(interface)?;
tc_builder.attach(program.id(), TcAttachType::Egress)?;
```

**新 API（aya 0.13.1）：**
```rust
tc::qdisc_add_clsact(interface)?;
program.attach(interface, TcAttachType::Egress)?;
```

## 正确的项目结构

```
aria-agent/
├── Cargo.toml              # workspace 配置
├── agent/
│   ├── Cargo.toml
│   ├── build.rs            # aya-build 配置
│   └── src/
│       └── main.rs         # 用户空间程序
├── ebpf/
│   ├── Cargo.toml
│   └── src/
│       ├── qos.rs          # eBPF QoS 程序
│       └── acl.rs          # eBPF ACL 程序
└── shared/
    ├── Cargo.toml
    └── src/
        └── lib.rs          # 共享数据结构
```

## 编译命令

```bash
# 完整清理后编译
rm -rf target && cargo build --release

# 跳过 eBPF 编译（用于调试用户空间代码）
SKIP_EBPF_BUILD=1 cargo build --release
```

## 依赖版本参考

```toml
# Cargo.toml (workspace)
[workspace.dependencies]
aya = { version = "0.13.1" }
aya-log = { version = "0.2" }
network-types = "0.1"

# agent/Cargo.toml
[dependencies]
aya.workspace = true

[build-dependencies]
aya-build = "0.1"

# ebpf/Cargo.toml
[dependencies]
aya-ebpf = { version = "0.1" }
network-types = "0.1"
```

## 关键要点

1. **不要在 `.cargo/config.toml` 中配置 `build-std`** - aya-build 会自动处理
2. **使用 `default-members` 排除 eBPF 包** - 避免直接编译 eBPF 代码
3. **检查 API 版本** - aya 0.12 和 0.13 API 有差异
4. **清理 target 目录** - 遇到奇怪错误时，先清理再编译
5. **查看官方文档** - https://docs.rs/aya/0.13.1/aya/

---

最后更新：2026-03-01
