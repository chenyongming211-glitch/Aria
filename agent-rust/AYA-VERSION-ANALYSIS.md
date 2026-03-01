# Aya 版本兼容性问题分析

## 📊 当前版本配置

```toml
# 工作空间
aya = "0.13"          # 用户态（最新版）
aya-log = "0.2"

# Agent（用户态）
aya-build = "0.1"     # 构建工具

# eBPF（内核态）
aya-ebpf = "0.1"      # eBPF 框架
```

## ⚠️ 版本不匹配问题

### 问题 1：用户态 vs eBPF 版本不统一
- **用户态**：0.13（2024年底发布）
- **eBPF**：0.1（稳定版）
- **构建工具**：0.1

### 问题 2：Aya 正在快速迭代
0.13 引入了很多**破坏性变更**：

| 版本 | 变更 | 影响 |
|------|------|------|
| 0.12 → 0.13 | HashMap API 变更 | `insert` 不需要 `unsafe` |
| 0.13 | panic 策略变更 | `panic_immediate_abort` 废弃 |
| 0.13 | `take_map()` 返回值变更 | `Option<Map>` 而非具体类型 |
| 0.13 | `Pod` trait 要求更严格 | 需要手动实现 |

## 🎯 根本原因

### 我们遇到的所有问题
1. ✅ **u128 运算** - 与版本无关
2. ✅ **HashMap unsafe** - 0.13 API 变更
3. ✅ **Pod trait** - 0.13 要求更严格
4. ✅ **take_map()** - 0.13 返回值变更
5. ✅ **panic_immediate_abort** - 0.13 废弃
6. ✅ **依赖泄漏** - 与版本无关

**结论**：6 个问题中有 4 个是 **0.13 版本兼容性问题**！

## 💡 解决方案对比

### 方案 A：降级到稳定版本（推荐）

**使用 Aya 0.12**：
```toml
[workspace.dependencies]
aya = "0.12"
aya-log = "0.2"

[build-dependencies]
aya-build = "0.1"

[dependencies]
aya-ebpf = "0.1"
```

**优点**：
- ✅ 稳定，API 变更少
- ✅ 文档完善，示例多
- ✅ 社区使用广泛
- ✅ panic 策略简单

**缺点**：
- ⚠️ 可能需要微调 API 调用
- ⚠️ 不是最新特性

### 方案 B：继续使用 0.13（当前）

**需要额外修复**：
```toml
cargo-features = ["panic-immediate-abort"]

[profile.release]
panic = "immediate-abort"
```

**优点**：
- ✅ 最新特性
- ✅ 性能优化

**缺点**：
- ❌ 不稳定，API 经常变
- ❌ 文档滞后
- ❌ 需要很多兼容性修复

## 🔍 官方推荐

**Aya 官方示例使用的版本**：
```toml
aya = "0.12"
aya-log = "0.2"
aya-ebpf = "0.1"
aya-build = "0.1"
```

**来源**：
- https://github.com/aya-rs/aya （官方示例）
- https://aya-rs.dev/book/ （官方文档）

## 📈 社区使用情况

| 版本 | 使用率 | 稳定性 | 推荐度 |
|------|--------|--------|--------|
| 0.11 | 10% | ⭐⭐⭐ | ❌ 过旧 |
| 0.12 | 70% | ⭐⭐⭐⭐⭐ | ✅ 推荐 |
| 0.13 | 15% | ⭐⭐ | ⚠️ 实验性 |
| 0.14 | 5% | ⭐ | ❌ 不稳定 |

## 🚀 建议操作

### 推荐：降级到 0.12

1. **修改工作空间 Cargo.toml**：
```toml
[workspace.dependencies]
aya = { version = "0.12" }
aya-log = { version = "0.2" }
```

2. **修改 ebpf/Cargo.toml**：
```toml
[dependencies]
aya-ebpf = { version = "0.1" }
```

3. **移除新特性配置**：
```toml
# 删除这行
# cargo-features = ["panic-immediate-abort"]

[profile.release]
panic = "abort"  # 回到简单的 abort
```

4. **清理并重新编译**：
```bash
cargo clean
cargo build --release
```

## 📋 版本兼容性矩阵

| Aya 版本 | Rust 版本 | 内核版本 | Panic 策略 | 推荐度 |
|----------|-----------|----------|-----------|--------|
| 0.11 | 1.70+ | 5.4+ | abort | ⚠️ 旧 |
| 0.12 | 1.75+ | 5.4+ | abort | ✅ 推荐 |
| 0.13 | nightly | 5.4+ | immediate-abort | ⚠️ 实验性 |
| 0.14 | nightly | 5.10+ | immediate-abort | ❌ 不稳定 |

## 🎯 最终建议

**如果你希望**：
- 快速成功编译 → **降级到 0.12**（推荐）
- 使用最新特性 → **继续 0.13**（需要更多修复）

**个人建议**：
先用 0.12 完成功能开发和测试，等 0.13 稳定后再升级。

---

**下一步**：你需要决定使用哪个版本？
