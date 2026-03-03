# gRPC 双向流心跳 + 配置热加载

## TL;DR

> **Quick Summary**: 为 Rust Agent 实现 gRPC 双向流心跳（支持 Controller 下发命令）和 SIGHUP 配置热加载，实现 Agent 与 Controller 的实时通信和动态配置更新。
> 
> **Deliverables**:
> - 更新的 protobuf 定义（新增 Heartbeat 双向流 RPC + 17 种命令类型）
> - Rust Agent 心跳模块（双向流 + 断线重连 + 命令处理）
> - Rust Agent 热加载模块（SIGHUP 信号 + broadcast 通知）
> - Go Controller gRPC 服务端（双向流支持）
> - TDD 测试套件（单元测试 + 集成测试）
> 
> **Estimated Effort**: Medium
> **Parallel Execution**: YES - 4 waves
> **Critical Path**: protobuf → Agent Heartbeat → Controller gRPC → Agent Hot Reload → Tests

---

## Context

### Original Request
用户提供了 Rust Agent 的设计方案：
1. gRPC 心跳：双向流 + 断线重连 + Controller 下发命令
2. 配置热加载：SIGHUP 信号 + broadcast 通知

### Interview Summary

**Key Discussions**:
- **架构设计**：整合成一个进程（Agent 模块集成到 Daemon）
- **gRPC 心跳**：新增 Heartbeat 双向流 RPC（间隔 10s，超时 5s）
- **断线重连**：指数退避（1s → 60s），重连后重新注册
- **命令体系**：17 种命令（RELOAD_CONFIG, UPDATE_POLICIES, WG_*, ROUTE_*, GET_STATUS 等）
- **配置热加载**：全部配置支持热加载（SIGHUP）
- **测试策略**：TDD（测试驱动开发）
- **实施范围**：Agent（Rust）+ Controller（Go）
- **优先级**：Phase 1（心跳）→ Phase 2（热加载）

**Research Findings**:
- 现有 Agent 已有单向心跳（每 10 秒）
- 现有 gRPC 客户端支持 Register、Sync、Heartbeat
- 现有 Controller gRPC 服务端支持 Register、Sync
- 现有 protobuf 定义只有单向 RPC
- 现有 Daemon 监听 SIGINT，未监听 SIGHUP

### Metis Review (Self-Analysis)

**Identified Gaps** (addressed):
- **未问的问题**：Controller 并发心跳支持 → 假设支持，测试时验证
- **防护栏**：不能破坏现有 API 兼容性 → 新增 RPC，不改现有
- **范围蔓延**：可能实现所有 17 种命令 → Phase 1 只实现核心命令（RELOAD_CONFIG, UPDATE_POLICIES, GET_STATUS）
- **边界情况**：心跳超时但网络恢复 → 重新注册；配置文件损坏 → 错误处理

---

## Work Objectives

### Core Objective
为 Aria Rust Agent 实现实时通信和动态配置更新能力，支持 Controller 通过 gRPC 双向流下发命令，并支持通过 SIGHUP 信号热加载配置。

### Concrete Deliverables
- `pkg/grpc/agentpb/aria-agent.proto`：新增 Heartbeat 双向流 RPC + 命令枚举
- `agent-rust/agent/src/grpc_client.rs`：实现双向流心跳 + 断线重连
- `agent-rust/agent/src/agent.rs`：集成心跳模块 + 命令处理器
- `agent-rust/agent/src/main.rs`：添加 SIGHUP 信号处理 + broadcast
- `internal/controller/grpc/server.go`：实现双向流 Heartbeat RPC
- `agent-rust/tests/`：单元测试 + 集成测试

### Definition of Done
- [ ] protobuf 定义更新并编译通过
- [ ] Rust Agent 心跳模块实现并通过测试
- [ ] Rust Agent 热加载实现并通过测试
- [ ] Go Controller 双向流实现并通过测试
- [ ] 端到端测试通过（Agent ↔ Controller 双向通信）
- [ ] 文档更新（README + CHANGELOG）

### Must Have
- 双向流 Heartbeat RPC（Agent ↔ Controller）
- 17 种命令类型定义
- 断线重连（指数退避 1s → 60s）
- SIGHUP 信号处理
- 全部配置支持热加载
- TDD 测试套件

### Must NOT Have (Guardrails)
- ❌ 不能破坏现有 Register 和 Sync API
- ❌ 不能影响现有 Agent 的正常运行
- ❌ 心跳失败不能导致 Agent 崩溃
- ❌ 配置热加载失败不能导致状态不一致
- ❌ Phase 1 不实现所有 17 种命令的处理器（只实现核心 3 种）

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES（Rust cargo test, Go go test）
- **Automated tests**: TDD（测试驱动开发）
- **Framework**: cargo test (Rust) + go test (Go)
- **TDD Flow**: RED (failing test) → GREEN (minimal impl) → REFACTOR

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **gRPC 通信**: Use Bash (grpcurl/grpc_client) — Send request, assert response
- **Rust 测试**: Use Bash (cargo test) — Run tests, assert pass
- **Go 测试**: Use Bash (go test) — Run tests, assert pass
- **集成测试**: Use Bash (scripts) — Start services, test communication, cleanup

---

## Execution Strategy

### Parallel Execution Waves

> Maximize throughput by grouping independent tasks into parallel waves.
> Target: 4-7 tasks per wave.

```
Wave 1 (Start Immediately — protobuf + 测试基础):
├── Task 1: 更新 protobuf 定义（Heartbeat RPC + 命令枚举）[quick]
├── Task 2: 编写 Rust 心跳模块测试用例（TDD - RED）[quick]
├── Task 3: 编写 Rust 热加载模块测试用例（TDD - RED）[quick]
└── Task 4: 编写 Go Controller gRPC 测试用例（TDD - RED）[quick]

Wave 2 (After Wave 1 — 核心实现):
├── Task 5: 实现 Rust 心跳模块（双向流 + 断线重连）[deep]
├── Task 6: 实现 Rust 热加载模块（SIGHUP + broadcast）[quick]
├── Task 7: 实现 Go Controller 双向流 Heartbeat RPC [unspecified-high]
└── Task 8: 集成心跳模块到 Agent 主循环 [deep]

Wave 3 (After Wave 2 — 测试通过 + 集成):
├── Task 9: Rust 心跳模块测试通过（TDD - GREEN）[quick]
├── Task 10: Rust 热加载模块测试通过（TDD - GREEN）[quick]
├── Task 11: Go Controller gRPC 测试通过（TDD - GREEN）[quick]
└── Task 12: 端到端集成测试（Agent ↔ Controller）[deep]

Wave 4 (After Wave 3 — 文档 + 清理):
├── Task 13: 更新 README（新功能说明）[quick]
├── Task 14: 更新 CHANGELOG（版本变更）[quick]
└── Task 15: 代码审查 + 优化 [unspecified-high]

Critical Path: Task 1 → Task 5 → Task 7 → Task 8 → Task 12 → Task 15
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 4 (Waves 1 & 2)
```

### Dependency Matrix

- **1**: — 2-4, 5, 7
- **2**: — 5, 9
- **3**: — 6, 10
- **4**: — 7, 11
- **5**: 1, 2 — 8, 9
- **6**: 3 — 8, 10
- **7**: 1, 4 — 12, 11
- **8**: 5, 6 — 12
- **9**: 2, 5 — 12
- **10**: 3, 6 — 12
- **11**: 4, 7 — 12
- **12**: 7, 8, 9, 10, 11 — 13, 14
- **13**: 12 — 15
- **14**: 12 — 15
- **15**: 13, 14 —

### Agent Dispatch Summary

- **1**: quick
- **2-4**: quick
- **5**: deep
- **6**: quick
- **7**: unspecified-high
- **8**: deep
- **9-11**: quick
- **12**: deep
- **13-14**: quick
- **15**: unspecified-high

---

## TODOs

> Implementation + Test = ONE Task. Never separate.
> EVERY task MUST have: Recommended Agent Profile + Parallelization info + QA Scenarios.

- [ ] 1. 更新 protobuf 定义（Heartbeat RPC + 命令枚举）

  **What to do**:
  - 在 `pkg/grpc/agentpb/aria-agent.proto` 中新增 `Heartbeat` 双向流 RPC
  - 定义 `AgentCommand` 枚举（17 种命令类型）
  - 定义 `HeartbeatRequest` 和 `HeartbeatResponse` 消息
  - 编译 protobuf（生成 Rust 和 Go 代码）

  **Must NOT do**:
  - 不要修改现有的 Register 和 Sync RPC
  - 不要删除现有消息定义

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单文件修改，语法清晰，快速完成
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3, 4)
  - **Blocks**: Tasks 2-4 (依赖 protobuf 定义), Task 5, Task 7
  - **Blocked By**: None (can start immediately)

  **References**:
  - `pkg/grpc/agentpb/aria-agent.proto:18-24` - 现有 ControllerService 定义（新增 Heartbeat RPC）
  - `pkg/grpc/agentpb/aria-agent.proto:96-102` - 现有 ACLRule 消息（参考消息结构）
  - External: https://protobuf.dev/programming-guides/proto3/ - Protobuf 语法

  **Acceptance Criteria**:
  - [ ] protobuf 文件包含 `rpc Heartbeat(stream HeartbeatRequest) returns (stream HeartbeatResponse)`
  - [ ] protobuf 文件包含 `enum AgentCommand`（17 种命令）
  - [ ] `protoc` 编译成功，生成 Rust 和 Go 代码
  - [ ] 生成的代码在 `agent-rust/agent/src/grpc_client.rs` 中可导入

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: protobuf 编译成功
    Tool: Bash (protoc)
    Preconditions: protobuf 文件已修改
    Steps:
      1. cd /Users/chen/Aria && protoc --rust_out=agent-rust/agent/src pkg/grpc/agentpb/aria-agent.proto
      2. cd /Users/chen/Aria && protoc --go_out=. pkg/grpc/agentpb/aria-agent.proto
    Expected Result: 编译成功，无错误
    Failure Indicators: protoc 报错，生成文件缺失
    Evidence: .sisyphus/evidence/task-1-protoc-compile.log
  ```

  **Evidence to Capture**:
  - [ ] protoc 编译日志
  - [ ] 生成的 Rust 文件（aria-agent.rs）
  - [ ] 生成的 Go 文件（aria-agent.pb.go）

  **Commit**: YES
  - Message: `feat(proto): add Heartbeat bidirectional stream RPC`
  - Files: `pkg/grpc/agentpb/aria-agent.proto`, generated files
  - Pre-commit: `protoc` compilation

- [ ] 2. 编写 Rust 心跳模块测试用例（TDD - RED）

  **What to do**:
  - 在 `agent-rust/tests/` 创建 `test_heartbeat.rs`
  - 编写测试用例：
    1. 心跳连接成功
    2. 心跳超时重试
    3. 断线重连（指数退避）
    4. Controller 下发命令（RELOAD_CONFIG）
  - **测试应该失败**（TDD - RED 阶段）

  **Must NOT do**:
  - 不要实现实际的心跳逻辑（只写测试）
  - 不要修改生产代码

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单文件测试代码，语法标准
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3, 4)
  - **Blocks**: Task 5 (测试驱动实现), Task 9
  - **Blocked By**: Task 1 (需要 protobuf 定义)

  **References**:
  - `agent-rust/tests/test_grpc_client.rs` - 现有 gRPC 测试（参考测试模式）
  - `agent-rust/agent/src/grpc_client.rs:119-131` - 现有 Heartbeat 方法（参考）

  **Acceptance Criteria**:
  - [ ] 测试文件创建：`agent-rust/tests/test_heartbeat.rs`
  - [ ] `cargo test` 运行失败（TDD - RED）
  - [ ] 测试覆盖：连接成功、超时、重连、命令处理

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 测试用例编译通过但运行失败
    Tool: Bash (cargo test)
    Preconditions: 测试文件已创建
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test --test test_heartbeat
    Expected Result: 测试编译通过，运行失败（RED 阶段）
    Failure Indicators: 编译失败，或测试全部通过（不符合 TDD）
    Evidence: .sisyphus/evidence/task-2-test-red.log
  ```

  **Evidence to Capture**:
  - [ ] cargo test 输出（RED 阶段）
  - [ ] 测试文件内容

  **Commit**: YES
  - Message: `test(agent): add heartbeat module test cases (TDD RED)`
  - Files: `agent-rust/tests/test_heartbeat.rs`
  - Pre-commit: none

- [ ] 3. 编写 Rust 热加载模块测试用例（TDD - RED）

  **What to do**:
  - 在 `agent-rust/tests/` 创建 `test_hot_reload.rs`
  - 编写测试用例：
    1. SIGHUP 信号触发配置重载
    2. 配置文件解析成功
    3. broadcast 通知到各模块
    4. 配置文件损坏错误处理
  - **测试应该失败**（TDD - RED 阶段）

  **Must NOT do**:
  - 不要实现实际的热加载逻辑（只写测试）
  - 不要修改生产代码

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单文件测试代码，语法标准
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 4)
  - **Blocks**: Task 6 (测试驱动实现), Task 10
  - **Blocked By**: Task 1 (需要 protobuf 定义)

  **References**:
  - `agent-rust/agent/src/config.rs:40-93` - 现有配置管理（参考配置重载）
  - `agent-rust/agent/src/main.rs:391-400` - 现有信号处理（参考 SIGINT）

  **Acceptance Criteria**:
  - [ ] 测试文件创建：`agent-rust/tests/test_hot_reload.rs`
  - [ ] `cargo test` 运行失败（TDD - RED）
  - [ ] 测试覆盖：信号触发、配置解析、broadcast、错误处理

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 测试用例编译通过但运行失败
    Tool: Bash (cargo test)
    Preconditions: 测试文件已创建
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test --test test_hot_reload
    Expected Result: 测试编译通过，运行失败（RED 阶段）
    Failure Indicators: 编译失败，或测试全部通过（不符合 TDD）
    Evidence: .sisyphus/evidence/task-3-test-red.log
  ```

  **Evidence to Capture**:
  - [ ] cargo test 输出（RED 阶段）
  - [ ] 测试文件内容

  **Commit**: YES
  - Message: `test(agent): add hot reload module test cases (TDD RED)`
  - Files: `agent-rust/tests/test_hot_reload.rs`
  - Pre-commit: none

- [ ] 4. 编写 Go Controller gRPC 测试用例（TDD - RED）

  **What to do**:
  - 在 `internal/controller/grpc/` 创建 `server_test.go`
  - 编写测试用例：
    1. Heartbeat 双向流连接成功
    2. Controller 下发命令到 Agent
    3. Agent 心跳丢失超时检测
  - **测试应该失败**（TDD - RED 阶段）

  **Must NOT do**:
  - 不要实现实际的 Heartbeat RPC（只写测试）
  - 不要修改生产代码

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 单文件测试代码，语法标准
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 3)
  - **Blocks**: Task 7 (测试驱动实现), Task 11
  - **Blocked By**: Task 1 (需要 protobuf 定义)

  **References**:
  - `internal/controller/grpc/server.go:12-169` - 现有 gRPC 服务端（参考实现）
  - `pkg/grpc/agentpb/aria-agent.proto:18-24` - 现有 RPC 定义（参考）

  **Acceptance Criteria**:
  - [ ] 测试文件创建：`internal/controller/grpc/server_test.go`
  - [ ] `go test` 运行失败（TDD - RED）
  - [ ] 测试覆盖：双向流连接、命令下发、超时检测

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 测试用例编译通过但运行失败
    Tool: Bash (go test)
    Preconditions: 测试文件已创建
    Steps:
      1. cd /Users/chen/Aria && go test ./internal/controller/grpc/...
    Expected Result: 测试编译通过，运行失败（RED 阶段）
    Failure Indicators: 编译失败，或测试全部通过（不符合 TDD）
    Evidence: .sisyphus/evidence/task-4-test-red.log
  ```

  **Evidence to Capture**:
  - [ ] go test 输出（RED 阶段）
  - [ ] 测试文件内容

  **Commit**: YES
  - Message: `test(controller): add Heartbeat RPC test cases (TDD RED)`
  - Files: `internal/controller/grpc/server_test.go`
  - Pre-commit: none

- [ ] 5. 实现 Rust 心跳模块（双向流 + 断线重连）

  **What to do**:
  - 在 `agent-rust/agent/src/grpc_client.rs` 实现双向流心跳：
    1. `start_heartbeat_stream()` 方法：启动双向流
    2. `heartbeat_loop()` 方法：心跳循环（10s 间隔）
    3. `handle_command()` 方法：处理 Controller 下发的命令
    4. `reconnect_with_backoff()` 方法：断线重连（指数退避 1s → 60s）
  - 实现命令处理器（Phase 1 只处理 3 种核心命令）：
    - RELOAD_CONFIG
    - UPDATE_POLICIES
    - GET_STATUS

  **Must NOT do**:
  - 不要实现所有 17 种命令的处理器（Phase 1 只实现核心 3 种）
  - 不要破坏现有的 Register 和 Sync 方法

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 核心逻辑复杂，需要深入理解 gRPC 双向流和并发编程
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 6, 7, 8)
  - **Blocks**: Task 8 (需要心跳模块), Task 9 (测试通过)
  - **Blocked By**: Task 1 (protobuf), Task 2 (测试用例)

  **References**:
  - `agent-rust/agent/src/grpc_client.rs:119-131` - 现有 Heartbeat 方法（改造为双向流）
  - `agent-rust/agent/src/agent.rs:152-162` - 现有心跳发送（参考心跳循环）
  - External: https://tokio.rs/tokio/tutorial - Tokio 异步编程
  - External: https://docs.rs/tonic/latest/tonic/ - Tonic gRPC 文档

  **Acceptance Criteria**:
  - [ ] 双向流连接成功（Agent ↔ Controller）
  - [ ] 心跳间隔 10s，超时 5s
  - [ ] 断线重连：指数退避（1s → 60s），重连后重新注册
  - [ ] 支持处理 3 种核心命令
  - [ ] `cargo test` 测试通过（TDD - GREEN）

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 心跳连接成功并保持
    Tool: Bash (cargo test)
    Preconditions: 模拟 Controller gRPC 服务端运行
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test test_heartbeat_connection_success
    Expected Result: 测试通过，心跳连接成功
    Failure Indicators: 连接失败，或心跳超时
    Evidence: .sisyphus/evidence/task-5-heartbeat-success.log
  ```

  ```
  Scenario: 断线重连成功（指数退避）
    Tool: Bash (cargo test)
    Preconditions: 模拟 Controller 重启
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test test_heartbeat_reconnect
    Expected Result: 测试通过，重连成功，退避时间正确
    Failure Indicators: 重连失败，或退避时间错误
    Evidence: .sisyphus/evidence/task-5-reconnect.log
  ```

  ```
  Scenario: 命令处理成功（RELOAD_CONFIG）
    Tool: Bash (cargo test)
    Preconditions: 心跳连接已建立
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test test_heartbeat_command_reload
    Expected Result: 测试通过，命令处理成功
    Failure Indicators: 命令未处理，或处理失败
    Evidence: .sisyphus/evidence/task-5-command.log
  ```

  **Evidence to Capture**:
  - [ ] cargo test 输出（GREEN 阶段）
  - [ ] 心跳连接日志
  - [ ] 断线重连日志
  - [ ] 命令处理日志

  **Commit**: YES
  - Message: `feat(agent): implement bidirectional stream heartbeat with reconnection`
  - Files: `agent-rust/agent/src/grpc_client.rs`, `agent-rust/agent/src/agent.rs`
  - Pre-commit: `cargo test`

- [ ] 6. 实现 Rust 热加载模块（SIGHUP + broadcast）

  **What to do**:
  - 在 `agent-rust/agent/src/main.rs` 实现：
    1. SIGHUP 信号监听器
    2. `config_update_tx` broadcast channel
    3. `reload_config()` 方法：重新读取配置文件
    4. 通知各模块更新（WireGuard、eBPF、ACL/QoS）
  - 在各模块中订阅 `config_update_tx`：
    - `WireGuardManager`：更新密钥和 Peers
    - `AclManager`：更新 ACL 规则
    - `QoSManager`：更新 QoS 规则

  **Must NOT do**:
  - 不要破坏现有配置加载逻辑
  - 不要在热加载失败时导致 Agent 崩溃

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准信号处理 + broadcast 模式
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 5, 7, 8)
  - **Blocks**: Task 8 (需要热加载模块), Task 10 (测试通过)
  - **Blocked By**: Task 3 (测试用例)

  **References**:
  - `agent-rust/agent/src/config.rs:40-93` - 现有配置管理（参考配置重载）
  - `agent-rust/agent/src/main.rs:391-400` - 现有信号处理（参考 SIGINT）
  - External: https://docs.rs/tokio/latest/tokio/signal/index.html - Tokio 信号处理

  **Acceptance Criteria**:
  - [ ] SIGHUP 信号触发配置重载
  - [ ] 配置文件解析成功
  - [ ] broadcast 通知到所有订阅者
  - [ ] 配置文件损坏时错误处理（不崩溃）
  - [ ] `cargo test` 测试通过（TDD - GREEN）

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: SIGHUP 触发配置重载
    Tool: Bash (cargo test)
    Preconditions: 配置文件已修改
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test test_hot_reload_sighup
    Expected Result: 测试通过，配置重载成功
    Failure Indicators: 信号未触发，或重载失败
    Evidence: .sisyphus/evidence/task-6-sighup.log
  ```

  ```
  Scenario: 配置文件损坏错误处理
    Tool: Bash (cargo test)
    Preconditions: 配置文件损坏（无效 YAML）
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test test_hot_reload_error
    Expected Result: 测试通过，错误处理成功（不崩溃）
    Failure Indicators: Agent 崩溃，或未处理错误
    Evidence: .sisyphus/evidence/task-6-error.log
  ```

  **Evidence to Capture**:
  - [ ] cargo test 输出（GREEN 阶段）
  - [ ] SIGHUP 信号日志
  - [ ] 配置重载日志
  - [ ] 错误处理日志

  **Commit**: YES
  - Message: `feat(agent): implement SIGHUP hot reload with broadcast`
  - Files: `agent-rust/agent/src/main.rs`, `agent-rust/agent/src/agent.rs`
  - Pre-commit: `cargo test`

- [ ] 7. 实现 Go Controller 双向流 Heartbeat RPC

  **What to do**:
  - 在 `internal/controller/grpc/server.go` 实现：
    1. `Heartbeat(stream HeartbeatRequest) returns (stream HeartbeatResponse)` 方法
    2. 心跳超时检测（5s 超时）
    3. 命令下发逻辑（从数据库或配置读取命令）
  - 实现命令队列管理：
    - 存储 Agent 的待执行命令
    - 心跳响应时下发命令
    - 记录命令执行结果

  **Must NOT do**:
  - 不要破坏现有 Register 和 Sync RPC
  - 不要阻塞其他 Agent 的心跳

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Go gRPC 双向流实现，需要处理并发和状态管理
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 5, 6, 8)
  - **Blocks**: Task 11 (测试通过), Task 12 (端到端测试)
  - **Blocked By**: Task 1 (protobuf), Task 4 (测试用例)

  **References**:
  - `internal/controller/grpc/server.go:12-169` - 现有 gRPC 服务端（参考实现）
  - `pkg/grpc/agentpb/aria-agent.proto:18-24` - 现有 RPC 定义（参考）
  - External: https://grpc.io/docs/languages/go/basics/ - Go gRPC 教程

  **Acceptance Criteria**:
  - [ ] 双向流 Heartbeat RPC 实现完成
  - [ ] 心跳超时检测（5s）
  - [ ] 命令下发成功（RELOAD_CONFIG, UPDATE_POLICIES, GET_STATUS）
  - [ ] `go test` 测试通过（TDD - GREEN）

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 双向流连接成功
    Tool: Bash (go test)
    Preconditions: Agent 模拟客户端连接
    Steps:
      1. cd /Users/chen/Aria && go test ./internal/controller/grpc/... -run TestHeartbeatStream
    Expected Result: 测试通过，双向流连接成功
    Failure Indicators: 连接失败，或流中断
    Evidence: .sisyphus/evidence/task-7-stream.log
  ```

  ```
  Scenario: 心跳超时检测
    Tool: Bash (go test)
    Preconditions: Agent 停止发送心跳
    Steps:
      1. cd /Users/chen/Aria && go test ./internal/controller/grpc/... -run TestHeartbeatTimeout
    Expected Result: 测试通过，超时检测成功
    Failure Indicators: 超时未检测，或检测延迟
    Evidence: .sisyphus/evidence/task-7-timeout.log
  ```

  ```
  Scenario: 命令下发成功
    Tool: Bash (go test)
    Preconditions: 命令队列中有待执行命令
    Steps:
      1. cd /Users/chen/Aria && go test ./internal/controller/grpc/... -run TestHeartbeatCommand
    Expected Result: 测试通过，命令下发成功
    Failure Indicators: 命令未下发，或下发失败
    Evidence: .sisyphus/evidence/task-7-command.log
  ```

  **Evidence to Capture**:
  - [ ] go test 输出（GREEN 阶段）
  - [ ] 双向流连接日志
  - [ ] 超时检测日志
  - [ ] 命令下发日志

  **Commit**: YES
  - Message: `feat(controller): implement bidirectional stream Heartbeat RPC`
  - Files: `internal/controller/grpc/server.go`, `internal/controller/grpc/command_queue.go`
  - Pre-commit: `go test`

- [ ] 8. 集成心跳模块到 Agent 主循环

  **What to do**:
  - 在 `agent-rust/agent/src/agent.rs` 的 `run_main_loop()` 中集成：
    1. 启动心跳双向流（替代现有单向心跳）
    2. 处理 Controller 下发的命令
    3. 与热加载模块协同工作
  - 在 `agent-rust/agent/src/main.rs` 的 Daemon 中：
    1. 初始化心跳模块
    2. 传递 `config_update_tx` 到心跳模块

  **Must NOT do**:
  - 不要破坏现有主循环逻辑
  - 不要阻塞主循环

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 核心集成逻辑，需要理解主循环和心跳模块的协同
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 5, 6, 7)
  - **Blocks**: Task 12 (端到端测试)
  - **Blocked By**: Task 5 (心跳模块), Task 6 (热加载模块)

  **References**:
  - `agent-rust/agent/src/agent.rs:119-150` - 现有主循环（改造）
  - `agent-rust/agent/src/main.rs:302-389` - Daemon 主循环（参考）

  **Acceptance Criteria**:
  - [ ] 心跳模块成功集成到主循环
  - [ ] 命令处理与热加载协同工作
  - [ ] 主循环不阻塞
  - [ ] `cargo test` 测试通过

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 主循环集成成功
    Tool: Bash (cargo test)
    Preconditions: 心跳模块和热加载模块已实现
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test test_agent_main_loop
    Expected Result: 测试通过，主循环运行正常
    Failure Indicators: 主循环阻塞，或模块冲突
    Evidence: .sisyphus/evidence/task-8-main-loop.log
  ```

  **Evidence to Capture**:
  - [ ] cargo test 输出
  - [ ] 主循环日志
  - [ ] 模块协同日志

  **Commit**: YES
  - Message: `feat(agent): integrate heartbeat module into main loop`
  - Files: `agent-rust/agent/src/agent.rs`, `agent-rust/agent/src/main.rs`
  - Pre-commit: `cargo test`

- [ ] 9. Rust 心跳模块测试通过（TDD - GREEN）

  **What to do**:
  - 运行 `cargo test --test test_heartbeat`
  - 确认所有测试通过（TDD - GREEN 阶段）
  - 修复任何失败的测试
  - 记录测试覆盖率

  **Must NOT do**:
  - 不要跳过失败的测试
  - 不要降低测试标准

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 测试验证，快速完成
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 10, 11, 12)
  - **Blocks**: Task 12 (端到端测试)
  - **Blocked By**: Task 2 (测试用例), Task 5 (实现)

  **References**:
  - `agent-rust/tests/test_heartbeat.rs` - 测试用例
  - `agent-rust/agent/src/grpc_client.rs` - 实现代码

  **Acceptance Criteria**:
  - [ ] `cargo test --test test_heartbeat` 全部通过
  - [ ] 测试覆盖率 ≥ 80%
  - [ ] 无测试失败或跳过

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 所有测试通过
    Tool: Bash (cargo test)
    Preconditions: 心跳模块已实现
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test --test test_heartbeat
    Expected Result: 所有测试通过，无失败
    Failure Indicators: 有测试失败或跳过
    Evidence: .sisyphus/evidence/task-9-test-green.log
  ```

  **Evidence to Capture**:
  - [ ] cargo test 输出（GREEN 阶段）
  - [ ] 测试覆盖率报告

  **Commit**: YES
  - Message: `test(agent): heartbeat module tests pass (TDD GREEN)`
  - Files: none
  - Pre-commit: none

- [ ] 10. Rust 热加载模块测试通过（TDD - GREEN）

  **What to do**:
  - 运行 `cargo test --test test_hot_reload`
  - 确认所有测试通过（TDD - GREEN 阶段）
  - 修复任何失败的测试
  - 记录测试覆盖率

  **Must NOT do**:
  - 不要跳过失败的测试
  - 不要降低测试标准

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 测试验证，快速完成
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 9, 11, 12)
  - **Blocks**: Task 12 (端到端测试)
  - **Blocked By**: Task 3 (测试用例), Task 6 (实现)

  **References**:
  - `agent-rust/tests/test_hot_reload.rs` - 测试用例
  - `agent-rust/agent/src/main.rs` - 实现代码

  **Acceptance Criteria**:
  - [ ] `cargo test --test test_hot_reload` 全部通过
  - [ ] 测试覆盖率 ≥ 80%
  - [ ] 无测试失败或跳过

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 所有测试通过
    Tool: Bash (cargo test)
    Preconditions: 热加载模块已实现
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo test --test test_hot_reload
    Expected Result: 所有测试通过，无失败
    Failure Indicators: 有测试失败或跳过
    Evidence: .sisyphus/evidence/task-10-test-green.log
  ```

  **Evidence to Capture**:
  - [ ] cargo test 输出（GREEN 阶段）
  - [ ] 测试覆盖率报告

  **Commit**: YES
  - Message: `test(agent): hot reload module tests pass (TDD GREEN)`
  - Files: none
  - Pre-commit: none

- [ ] 11. Go Controller gRPC 测试通过（TDD - GREEN）

  **What to do**:
  - 运行 `go test ./internal/controller/grpc/...`
  - 确认所有测试通过（TDD - GREEN 阶段）
  - 修复任何失败的测试
  - 记录测试覆盖率

  **Must NOT do**:
  - 不要跳过失败的测试
  - 不要降低测试标准

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 测试验证，快速完成
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 9, 10, 12)
  - **Blocks**: Task 12 (端到端测试)
  - **Blocked By**: Task 4 (测试用例), Task 7 (实现)

  **References**:
  - `internal/controller/grpc/server_test.go` - 测试用例
  - `internal/controller/grpc/server.go` - 实现代码

  **Acceptance Criteria**:
  - [ ] `go test ./internal/controller/grpc/...` 全部通过
  - [ ] 测试覆盖率 ≥ 80%
  - [ ] 无测试失败或跳过

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 所有测试通过
    Tool: Bash (go test)
    Preconditions: gRPC 服务端已实现
    Steps:
      1. cd /Users/chen/Aria && go test ./internal/controller/grpc/... -v
    Expected Result: 所有测试通过，无失败
    Failure Indicators: 有测试失败或跳过
    Evidence: .sisyphus/evidence/task-11-test-green.log
  ```

  **Evidence to Capture**:
  - [ ] go test 输出（GREEN 阶段）
  - [ ] 测试覆盖率报告

  **Commit**: YES
  - Message: `test(controller): gRPC Heartbeat tests pass (TDD GREEN)`
  - Files: none
  - Pre-commit: none

- [ ] 12. 端到端集成测试（Agent ↔ Controller）

  **What to do**:
  - 创建端到端测试脚本：`scripts/test_e2e_heartbeat.sh`
  - 测试场景：
    1. Agent 启动并连接到 Controller
    2. 双向流心跳建立成功
    3. Controller 下发 RELOAD_CONFIG 命令
    4. Agent 执行命令并返回结果
    5. Agent 断线重连成功
    6. SIGHUP 信号触发配置重载
  - 记录测试结果和日志

  **Must NOT do**:
  - 不要使用生产环境
  - 不要跳过任何测试场景

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 端到端测试需要协调多个组件，复杂度高
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (需要所有模块完成)
  - **Blocks**: Tasks 13, 14 (文档)
  - **Blocked By**: Tasks 7, 8, 9, 10, 11 (所有模块测试通过)

  **References**:
  - `agent-rust/tests/` - Agent 测试
  - `internal/controller/grpc/` - Controller 测试
  - `scripts/` - 现有脚本（参考）

  **Acceptance Criteria**:
  - [ ] 端到端测试脚本创建：`scripts/test_e2e_heartbeat.sh`
  - [ ] 所有 6 个测试场景通过
  - [ ] 测试日志保存到 `.sisyphus/evidence/`

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: 端到端心跳通信成功
    Tool: Bash (scripts)
    Preconditions: Agent 和 Controller 运行
    Steps:
      1. cd /Users/chen/Aria && ./scripts/test_e2e_heartbeat.sh
    Expected Result: 所有测试场景通过
    Failure Indicators: 任一场景失败
    Evidence: .sisyphus/evidence/task-12-e2e.log
  ```

  **Evidence to Capture**:
  - [ ] 端到端测试脚本
  - [ ] 测试日志
  - [ ] 测试结果摘要

  **Commit**: YES
  - Message: `test: add end-to-end heartbeat integration tests`
  - Files: `scripts/test_e2e_heartbeat.sh`
  - Pre-commit: none

- [ ] 13. 更新 README（新功能说明）

  **What to do**:
  - 在 `agent-rust/README.md` 中添加：
    1. gRPC 双向流心跳功能说明
    2. 断线重连机制说明
    3. SIGHUP 配置热加载说明
    4. 支持的命令列表（17 种）
  - 在根目录 `README.md` 中添加新功能章节

  **Must NOT do**:
  - 不要删除现有内容
  - 不要过度详细（保持简洁）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 文档更新，快速完成
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Task 14)
  - **Blocks**: Task 15 (代码审查)
  - **Blocked By**: Task 12 (端到端测试)

  **References**:
  - `agent-rust/README.md` - 现有 Agent README
  - `README.md` - 项目主 README

  **Acceptance Criteria**:
  - [ ] `agent-rust/README.md` 包含心跳和热加载说明
  - [ ] `README.md` 包含新功能章节
  - [ ] 文档清晰易懂

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: README 包含新功能说明
    Tool: Bash (grep)
    Preconditions: README 已更新
    Steps:
      1. cd /Users/chen/Aria && grep -q "双向流心跳" agent-rust/README.md
      2. cd /Users/chen/Aria && grep -q "SIGHUP" agent-rust/README.md
    Expected Result: grep 找到关键词
    Failure Indicators: 关键词缺失
    Evidence: .sisyphus/evidence/task-13-readme.log
  ```

  **Evidence to Capture**:
  - [ ] README 文件内容
  - [ ] grep 输出

  **Commit**: YES
  - Message: `docs: add heartbeat and hot reload documentation`
  - Files: `agent-rust/README.md`, `README.md`
  - Pre-commit: none

- [ ] 14. 更新 CHANGELOG（版本变更）

  **What to do**:
  - 在 `CHANGELOG.md` 中添加新版本：
    1. 版本号：0.3.0
    2. 新增功能：gRPC 双向流心跳、SIGHUP 热加载
    3. 文件变更列表
    4. 性能数据对比（可选）

  **Must NOT do**:
  - 不要删除现有版本记录
  - 不要创建单独的版本文件

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 文档更新，快速完成
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 4 (with Task 13)
  - **Blocks**: Task 15 (代码审查)
  - **Blocked By**: Task 12 (端到端测试)

  **References**:
  - `CHANGELOG.md` - 现有变更日志（参考格式）
  - `CLAUDE.md:187-248` - CHANGELOG 管理规范

  **Acceptance Criteria**:
  - [ ] `CHANGELOG.md` 包含版本 0.3.0 记录
  - [ ] 包含功能列表和文件变更
  - [ ] 格式符合规范

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: CHANGELOG 包含新版本
    Tool: Bash (grep)
    Preconditions: CHANGELOG 已更新
    Steps:
      1. cd /Users/chen/Aria && grep -q "## \[0.3.0\]" CHANGELOG.md
      2. cd /Users/chen/Aria && grep -q "双向流心跳" CHANGELOG.md
    Expected Result: grep 找到关键词
    Failure Indicators: 关键词缺失
    Evidence: .sisyphus/evidence/task-14-changelog.log
  ```

  **Evidence to Capture**:
  - [ ] CHANGELOG 文件内容
  - [ ] grep 输出

  **Commit**: YES
  - Message: `docs: update CHANGELOG for v0.3.0`
  - Files: `CHANGELOG.md`
  - Pre-commit: none

- [ ] 15. 代码审查 + 优化

  **What to do**:
  - 运行代码质量检查：
    1. `cargo clippy` (Rust)
    2. `go fmt` + `golangci-lint` (Go)
  - 优化建议：
    1. 检查并发安全问题（Arc, Mutex, broadcast）
    2. 检查错误处理（所有 Result 都要处理）
    3. 检查日志级别（debug/info/warn/error）
  - 代码审查清单：
    1. 无 `unwrap()` 或 `expect()`（生产代码）
    2. 无硬编码配置（使用配置文件）
    3. 无过度注释（代码应自解释）

  **Must NOT do**:
  - 不要破坏测试
  - 不要过度优化（保持可读性）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 需要深入理解代码质量和最佳实践
  - **Skills**: []
    - 无特殊技能需求

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (最终审查)
  - **Blocks**: None (最终任务)
  - **Blocked By**: Tasks 13, 14 (文档完成)

  **References**:
  - `agent-rust/agent/src/` - Rust 代码
  - `internal/controller/grpc/` - Go 代码
  - `CLAUDE.md` - 开发规范

  **Acceptance Criteria**:
  - [ ] `cargo clippy` 无警告
  - [ ] `go fmt` 格式化完成
  - [ ] `golangci-lint` 无错误
  - [ ] 代码审查清单全部通过

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Rust 代码质量检查通过
    Tool: Bash (cargo clippy)
    Preconditions: 代码已编写
    Steps:
      1. cd /Users/chen/Aria/agent-rust && cargo clippy -- -D warnings
    Expected Result: clippy 无警告
    Failure Indicators: clippy 报告警告
    Evidence: .sisyphus/evidence/task-15-clippy.log
  ```

  ```
  Scenario: Go 代码质量检查通过
    Tool: Bash (golangci-lint)
    Preconditions: 代码已编写
    Steps:
      1. cd /Users/chen/Aria && golangci-lint run ./internal/controller/grpc/...
    Expected Result: linter 无错误
    Failure Indicators: linter 报告错误
    Evidence: .sisyphus/evidence/task-15-lint.log
  ```

  **Evidence to Capture**:
  - [ ] cargo clippy 输出
  - [ ] golangci-lint 输出
  - [ ] 代码审查清单

  **Commit**: YES
  - Message: `refactor: code review and optimization`
  - Files: changed files
  - Pre-commit: `cargo clippy`, `golangci-lint`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Rejection → fix → re-run.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [6/6] | Must NOT Have [4/4] | Tasks [15/15] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `cargo clippy` + `golangci-lint` + `cargo test` + `go test`. Review all changed files for: `unwrap()`/`expect()` in prod, empty catches, console.log, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names (data/result/item/temp).
  Output: `Build [PASS/FAIL] | Lint [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [ ] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration (features working together, not isolation). Test edge cases: config file corruption, Controller restart, network partition. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination: Task N touching Task M's files. Flag unaccounted changes.
  Output: `Tasks [15/15 compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **1**: `feat(proto): add Heartbeat bidirectional stream RPC` — protobuf files
- **2**: `test(agent): add heartbeat module test cases (TDD RED)` — test_heartbeat.rs
- **3**: `test(agent): add hot reload module test cases (TDD RED)` — test_hot_reload.rs
- **4**: `test(controller): add Heartbeat RPC test cases (TDD RED)` — server_test.go
- **5**: `feat(agent): implement bidirectional stream heartbeat with reconnection` — grpc_client.rs, agent.rs
- **6**: `feat(agent): implement SIGHUP hot reload with broadcast` — main.rs, agent.rs
- **7**: `feat(controller): implement bidirectional stream Heartbeat RPC` — server.go, command_queue.go
- **8**: `feat(agent): integrate heartbeat module into main loop` — agent.rs, main.rs
- **9**: `test(agent): heartbeat module tests pass (TDD GREEN)` — none
- **10**: `test(agent): hot reload module tests pass (TDD GREEN)` — none
- **11**: `test(controller): gRPC Heartbeat tests pass (TDD GREEN)` — none
- **12**: `test: add end-to-end heartbeat integration tests` — test_e2e_heartbeat.sh
- **13**: `docs: add heartbeat and hot reload documentation` — README.md
- **14**: `docs: update CHANGELOG for v0.3.0` — CHANGELOG.md
- **15**: `refactor: code review and optimization` — changed files

---

## Success Criteria

### Verification Commands
```bash
# Protobuf 编译
protoc --rust_out=agent-rust/agent/src pkg/grpc/agentpb/aria-agent.proto
protoc --go_out=. pkg/grpc/agentpb/aria-agent.proto

# Rust 测试
cd agent-rust && cargo test

# Go 测试
cd /Users/chen/Aria && go test ./internal/controller/grpc/...

# 端到端测试
./scripts/test_e2e_heartbeat.sh

# 代码质量检查
cd agent-rust && cargo clippy -- -D warnings
cd /Users/chen/Aria && golangci-lint run ./internal/controller/grpc/...
```

### Final Checklist
- [ ] All "Must Have" present（6/6）
- [ ] All "Must NOT Have" absent（4/4）
- [ ] All tests pass（Rust + Go）
- [ ] Protobuf 编译成功
- [ ] 端到端测试通过
- [ ] 代码质量检查通过
- [ ] 文档更新完成
- [ ] CHANGELOG 更新完成
- [ ] 证据文件完整（.sisyphus/evidence/）

