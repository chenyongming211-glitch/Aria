# Rust eBPF 快速开始指南

## 最小化编译步骤（3 步搞定）

### 1️⃣ 环境准备（首次）

```bash
# 安装 Rust nightly
rustup install nightly
rustup component add rust-src --toolchain nightly

# 安装系统依赖
apt-get update && apt-get install -y build-essential clang llvm libelf-dev

# 安装 bpf-linker
cargo install bpf-linker
```

### 2️⃣ 编译

```bash
cd /root/agent-rust
cargo clean
cargo build --release
```

### 3️⃣ 验证

```bash
# 检查编译产物
ls -lh target/release/aria-agent

# 运行测试
./target/release/aria-agent --help
```

---

## 一键编译脚本

保存为 `build.sh`：

```bash
#!/bin/bash
set -e

echo "=== Aria Rust eBPF 编译脚本 ==="

# 检查 Rust
if ! command -v rustup &> /dev/null; then
    echo "❌ Rust 未安装"
    exit 1
fi

# 检查 nightly
if ! rustup show | grep -q "nightly"; then
    echo "📦 安装 Rust nightly..."
    rustup install nightly
    rustup component add rust-src --toolchain nightly
fi

# 检查 bpf-linker
if ! command -v bpf-linker &> /dev/null; then
    echo "📦 安装 bpf-linker..."
    cargo install bpf-linker
fi

# 清理旧产物
echo "🧹 清理旧编译产物..."
cargo clean

# 编译
echo "🔨 编译 Release 版本..."
cargo build --release

# 验证
if [ -f "target/release/aria-agent" ]; then
    echo "✅ 编译成功！"
    ls -lh target/release/aria-agent
else
    echo "❌ 编译失败"
    exit 1
fi
```

使用方法：
```bash
chmod +x build.sh
./build.sh
```

---

## 常见问题速查

| 错误 | 原因 | 解决方案 |
|------|------|----------|
| `unwinding panics are not supported` | 依赖污染 | `cargo clean && cargo build --release` |
| `unresolved symbol __udivti3` | u128 运算 | 已修复，确保代码最新 |
| `could not find native static library 'elf'` | 缺少 libelf | `apt-get install libelf-dev` |
| `failed to run custom build command` | eBPF 编译失败 | `rustup component add rust-src --toolchain nightly` |

---

## 编译时间参考

- **首次编译**: 5-10 分钟（下载依赖）
- **增量编译**: 30-60 秒
- **Clean 构建**: 2-3 分钟

---

## 产物大小

- **Release (未 strip)**: ~15MB
- **Release (strip 后)**: ~8MB
- **Release (压缩后)**: ~3MB

---

## 下一步

详细文档请查看：[BUILD-GUIDE.md](./BUILD-GUIDE.md)
