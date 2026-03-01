# Rust eBPF 手动编译指南

## 目录

- [环境要求](#环境要求)
- [依赖安装](#依赖安装)
- [编译步骤](#编译步骤)
- [验证方法](#验证方法)
- [常见问题](#常见问题)
- [开发调试](#开发调试)

---

## 环境要求

### 系统要求

- **操作系统**: Linux (推荐 Ubuntu 20.04+)
- **内核版本**: 5.4+ (支持 eBPF 和 bpf_spin_lock)
- **架构**: x86_64 (amd64)
- **权限**: root 或具有 CAP_BPF、CAP_NET_ADMIN 权限

### Rust 工具链

```bash
# 查看当前 Rust 版本
rustc --version
cargo --version

# 推荐版本
# rustc 1.75+ (nightly)
# cargo 1.75+
```

---

## 依赖安装

### 1. 安装 Rust Nightly 工具链

```bash
# 安装 nightly 工具链
rustup install nightly

# 设置为默认（可选）
rustup default nightly

# 安装必要的组件
rustup component add rust-src --toolchain nightly
rustup component add clippy --toolchain nightly
rustup component add rustfmt --toolchain nightly
```

### 2. 安装系统依赖

```bash
# Ubuntu/Debian
apt-get update
apt-get install -y \
    build-essential \
    clang \
    llvm \
    libelf-dev \
    libbpf-dev \
    pkg-config

# CentOS/RHEL
yum install -y \
    gcc \
    make \
    clang \
    llvm \
    elfutils-libelf-devel \
    libbpf-devel \
    pkgconfig
```

### 3. 安装 bpf-linker（eBPF 链接器）

```bash
# 安装 bpf-linker
cargo install bpf-linker
```

### 4. 验证环境

```bash
# 检查 Rust 工具链
rustup show

# 检查组件
rustup component list --toolchain nightly | grep -E "(rust-src|clippy|rustfmt)"

# 检查系统工具
clang --version
llc --version
```

---

## 编译步骤

### 方法一：完整编译（推荐）

```bash
# 1. 进入项目目录
cd /root/agent-rust

# 2. 清理旧的编译产物（重要！）
cargo clean

# 3. 编译 Release 版本
cargo build --release
```

**编译产物位置**：
- 用户态程序：`target/release/aria-agent`
- eBPF 程序：已嵌入到用户态程序中（通过 build.rs）

### 方法二：分步编译（调试用）

#### 步骤 1: 仅编译 eBPF 程序

```bash
# 编译 eBPF 程序（bpfel-unknown-none 目标）
cargo +nightly build \
    --package aria-ebpf \
    -Z build-std=core \
    --release \
    --target bpfel-unknown-none \
    --manifest-path ebpf/Cargo.toml

# 产物位置
ls -lh target/bpfel-unknown-none/release/
# 应该看到：acl  qos  (两个 eBPF 字节码文件)
```

#### 步骤 2: 编译用户态程序

```bash
# 跳过 eBPF 编译（假设已经编译好）
SKIP_EBPF_BUILD=1 cargo build --release

# 或正常编译（会自动触发 eBPF 编译）
cargo build --release
```

### 方法三：仅检查语法（快速验证）

```bash
# 快速检查语法和类型错误（不生成二进制）
cargo check

# 只检查 eBPF 程序
cargo check --package aria-ebpf --target bpfel-unknown-none
```

---

## 验证方法

### 1. 验证编译产物

```bash
# 检查二进制文件
ls -lh target/release/aria-agent

# 查看文件类型
file target/release/aria-agent
# 应该显示：ELF 64-bit LSB executable, x86-64

# 查看大小（通常 10-20MB）
du -h target/release/aria-agent
```

### 2. 验证 eBPF 程序嵌入

```bash
# 查看 eBPF 字节码是否嵌入
readelf -S target/release/aria-agent | grep -E "(qos|acl)"

# 或使用 objdump
objdump -h target/release/aria-agent | grep -E "ebpf"
```

### 3. 运行程序测试

```bash
# 查看帮助信息
./target/release/aria-agent --help

# 查看版本
./target/release/aria-agent --version

# 测试运行（需要 root 权限）
sudo ./target/release/aria-agent --help
```

### 4. 检查 eBPF 功能

```bash
# 检查 eBPF 支持状态
sudo ./target/release/aria-agent ebpf status

# 如果成功，应该看到类似输出：
# eBPF Status: Supported
# Kernel Version: 5.15.0
```

---

## 常见问题

### 问题 1: `unwinding panics are not supported without std`

**原因**: eBPF 编译时污染了用户态依赖（如 clap）

**解决方案**:
```bash
# 1. 清理所有编译产物
cargo clean

# 2. 确保 build.rs 中有隔离环境配置
# 已在 build.rs 中添加：
# .env_remove("RUSTFLAGS")
# .env_remove("CARGO_ENCODED_RUSTFLAGS")
# .args(["--package", "aria-ebpf"])

# 3. 重新编译
cargo build --release
```

### 问题 2: `unresolved symbol __udivti3`

**原因**: eBPF 中使用了 u128 运算

**解决方案**:
- 已在 `ebpf/src/qos.rs` 中修复，使用纯 u64 算法
- 确保 `mul_div` 函数不使用 u128

### 问题 3: `error: could not find native static library 'elf'`

**原因**: 缺少 libelf 开发库

**解决方案**:
```bash
# Ubuntu/Debian
apt-get install libelf-dev

# CentOS/RHEL
yum install elfutils-libelf-devel
```

### 问题 4: `error: failed to run custom build command for aria-agent`

**原因**: eBPF 编译失败

**解决方案**:
```bash
# 1. 查看详细错误日志
cargo build --release --verbose 2>&1 | tee build.log

# 2. 检查是否安装了 rust-src
rustup component list --toolchain nightly | grep rust-src

# 3. 如果没有，安装它
rustup component add rust-src --toolchain nightly

# 4. 手动编译 eBPF 测试
cargo +nightly build \
    --package aria-ebpf \
    -Z build-std=core \
    --release \
    --target bpfel-unknown-none \
    --manifest-path ebpf/Cargo.toml
```

### 问题 5: `unused unsafe` 警告

**原因**: Aya 框架已将部分操作标记为安全

**解决方案**:
- HashMap::insert 不需要 unsafe（已在代码中移除）
- HashMap::get 需要 unsafe（保留）
- LpmTrie::get 不需要 unsafe（已在代码中移除）

### 问题 6: 编译速度慢

**解决方案**:
```bash
# 1. 使用更快的链接器（lld）
rustup component add lld --toolchain nightly

# 在 .cargo/config.toml 中添加：
[target.x86_64-unknown-linux-gnu]
linker = "rust-lld"

# 2. 使用 sccache 缓存
cargo install sccache
export RUSTC_WRAPPER=sccache

# 3. 增加并行编译任务
export CARGO_BUILD_JOBS=4
```

---

## 开发调试

### 1. 查看详细编译日志

```bash
# 显示每个编译命令
cargo build --release --verbose

# 保存日志到文件
cargo build --release --verbose 2>&1 | tee build.log
```

### 2. 检查依赖树

```bash
# 查看依赖关系
cargo tree

# 只看 eBPF 包的依赖
cargo tree --package aria-ebpf

# 检查重复依赖
cargo tree --duplicates
```

### 3. 代码格式化和检查

```bash
# 格式化代码
cargo fmt

# 静态检查
cargo clippy

# 只检查 eBPF 代码
cargo clippy --package aria-ebpf
```

### 4. 运行测试

```bash
# 运行所有测试
cargo test

# 运行特定测试
cargo test --package aria-agent

# 查看 eBPF 程序大小
ls -lh target/bpfel-unknown-none/release/
```

### 5. Release 优化

```bash
# 查看编译时间
cargo build --release --timings

# 查看二进制大小
bloaty target/release/aria-agent

# 剥离调试符号（减小体积）
strip target/release/aria-agent
```

---

## 生产部署

### 1. 交叉编译

```bash
# 编译为 Linux amd64（在 macOS 上）
cargo build --release --target x86_64-unknown-linux-gnu

# 产物位置
ls -lh target/x86_64-unknown-linux-gnu/release/aria-agent
```

### 2. 打包发布

```bash
# 1. 编译
cargo build --release

# 2. 剥离调试符号
strip target/release/aria-agent

# 3. 压缩
upx --best target/release/aria-agent

# 4. 生成 SHA256
sha256sum target/release/aria-agent > aria-agent.sha256

# 5. 打包
tar -czf aria-agent-$(date +%Y%m%d)-linux-amd64.tar.gz \
    target/release/aria-agent \
    aria-agent.sha256 \
    README.md
```

### 3. 系统服务部署

```bash
# 复制到系统路径
cp target/release/aria-agent /usr/local/bin/

# 创建 systemd 服务
cat > /etc/systemd/system/aria-agent.service << 'EOF'
[Unit]
Description=Aria Agent (eBPF)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/aria-agent daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
systemctl daemon-reload
systemctl enable aria-agent
systemctl start aria-agent
systemctl status aria-agent
```

---

## 性能调优

### 1. 编译优化选项

在 `Cargo.toml` 中：

```toml
[profile.release]
opt-level = 3          # 最高优化级别
lto = true            # 链接时优化
codegen-units = 1     # 单个代码生成单元（更好的优化）
strip = true          # 剥离调试符号
panic = "abort"       # panic 时直接中止
```

### 2. eBPF 优化

在 `ebpf/Cargo.toml` 中：

```toml
[profile.release]
opt-level = 3
lto = true
panic = "abort"
```

---

## 故障排查清单

- [ ] Rust nightly 工具链已安装
- [ ] rust-src 组件已安装
- [ ] clang/llvm 已安装
- [ ] libelf-dev 已安装
- [ ] bpf-linker 已安装
- [ ] `cargo clean` 已执行
- [ ] build.rs 中环境隔离已配置
- [ ] 不存在 u128 运算
- [ ] unsafe 使用正确
- [ ] 内核版本 >= 5.4
- [ ] 有 root 权限

---

## 参考资源

- [Aya Book](https://aya-rs.dev/book/)
- [eBPF Documentation](https://ebpf.io/)
- [Rust embedded Book](https://docs.rust-embedded.org/book/)
- [Linux BPF Documentation](https://www.kernel.org/doc/html/latest/bpf/index.html)

---

**最后更新**: 2026-02-25  
**版本**: 1.0.0  
**维护者**: Aria Team
