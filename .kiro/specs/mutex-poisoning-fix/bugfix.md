# Bugfix Requirements Document

## Introduction

本文档定义了修复 Aria SD-WAN Rust Agent 中 Mutex 中毒漏洞的需求。该漏洞存在于 `agent-rust/agent/src/unified_agent.rs` 中，由于在异步上下文中使用 `std::sync::Mutex` 并配合 `.unwrap()` 处理锁获取，当任何线程在持有锁时发生 panic，会导致 Mutex 进入 "poisoned" 状态，后续所有 `.lock().unwrap()` 调用都会 panic，造成级联失败，最终导致 Agent 完全不可用。

受影响的功能包括：
- 主同步循环
- Unix socket CLI 命令
- 远程 gRPC 命令
- 日志级别管理
- 健康检查

## Bug Analysis

### Current Behavior (Defect)

1.1 WHEN 任何线程在持有 `last_sync_peers` 的 `StdMutex` 锁时发生 panic THEN Mutex 进入 poisoned 状态，后续所有 9 处 `.lock().unwrap()` 调用都会 panic

1.2 WHEN 任何线程在持有 `log_handle` 的 `StdMutex` 锁时发生 panic THEN Mutex 进入 poisoned 状态，日志级别管理功能完全失效

1.3 WHEN 任何线程在持有 `current_log_level` 的 `StdMutex` 锁时发生 panic THEN Mutex 进入 poisoned 状态，日志级别查询和更新功能完全失效

1.4 WHEN Mutex 中毒后，主同步循环（Line 415）尝试获取 `last_sync_peers` 锁 THEN 主循环 panic 导致 Agent 完全崩溃

1.5 WHEN Mutex 中毒后，Unix socket 任务（Line 618）尝试获取 `last_sync_peers` 锁 THEN Unix socket CLI 命令处理失败

1.6 WHEN Mutex 中毒后，远程命令执行（Line 1235, 1320, 1349）尝试获取 `last_sync_peers` 锁 THEN 远程 gRPC 命令执行失败

1.7 WHEN Mutex 中毒后，sync 函数（Line 1470）尝试更新 `last_sync_peers` THEN 同步功能完全失效

1.8 WHEN 在异步上下文中使用 `std::sync::Mutex` THEN 可能导致线程阻塞，影响 tokio 运行时性能

1.9 WHEN Agent 因 Mutex 中毒崩溃 THEN Agent 无法自动恢复，需要手动重启

### Expected Behavior (Correct)

2.1 WHEN 任何线程在持有 `last_sync_peers` 锁时发生 panic THEN 其他线程 SHALL 能够检测到错误并记录日志，但不应导致级联崩溃

2.2 WHEN 任何线程在持有 `log_handle` 锁时发生 panic THEN 其他线程 SHALL 能够检测到错误并记录日志，但不应导致日志管理功能完全失效

2.3 WHEN 任何线程在持有 `current_log_level` 锁时发生 panic THEN 其他线程 SHALL 能够检测到错误并记录日志，但不应导致日志级别管理功能完全失效

2.4 WHEN 在异步上下文中需要获取锁 THEN 系统 SHALL 使用 `tokio::sync::Mutex` 并使用 `.lock().await` 获取锁

2.5 WHEN 锁获取失败或检测到 Mutex 中毒 THEN 系统 SHALL 记录错误日志并返回错误结果，而不是 panic

2.6 WHEN 主同步循环需要访问 `last_sync_peers` THEN 系统 SHALL 使用 `tokio::sync::Mutex` 并优雅处理锁获取失败

2.7 WHEN Unix socket 任务需要访问 `last_sync_peers` THEN 系统 SHALL 使用 `tokio::sync::Mutex` 并优雅处理锁获取失败

2.8 WHEN 远程命令执行需要访问 `last_sync_peers` THEN 系统 SHALL 使用 `tokio::sync::Mutex` 并优雅处理锁获取失败

2.9 WHEN sync 函数需要更新 `last_sync_peers` THEN 系统 SHALL 使用 `tokio::sync::Mutex` 并优雅处理锁获取失败

2.10 WHEN 日志级别管理需要访问 `log_handle` 或 `current_log_level` THEN 系统 SHALL 使用 `tokio::sync::Mutex` 并优雅处理锁获取失败

### Unchanged Behavior (Regression Prevention)

3.1 WHEN Agent 正常运行且没有发生 panic THEN 系统 SHALL CONTINUE TO 正常执行主同步循环、Unix socket 命令处理、远程 gRPC 命令执行和日志级别管理

3.2 WHEN 多个线程并发访问共享状态 THEN 系统 SHALL CONTINUE TO 通过 Mutex 保证数据一致性和线程安全

3.3 WHEN Agent 收集和报告 metrics THEN 系统 SHALL CONTINUE TO 正确记录 peer 数量、同步状态等统计信息

3.4 WHEN Agent 执行 sync 操作 THEN 系统 SHALL CONTINUE TO 正确更新 peer 列表、ACL 规则、QoS 规则等配置

3.5 WHEN Unix socket CLI 命令查询 peer 信息 THEN 系统 SHALL CONTINUE TO 返回正确的 peer 列表和状态信息

3.6 WHEN 远程 gRPC 命令执行健康检查 THEN 系统 SHALL CONTINUE TO 返回正确的 Agent 状态和 peer 数量

3.7 WHEN 用户通过 Unix socket 或远程命令设置日志级别 THEN 系统 SHALL CONTINUE TO 正确更新日志级别并应用到运行时

3.8 WHEN 用户查询当前日志级别 THEN 系统 SHALL CONTINUE TO 返回正确的日志级别信息

3.9 WHEN Agent 启动和初始化 THEN 系统 SHALL CONTINUE TO 正确创建和初始化所有共享状态和 Mutex

3.10 WHEN Agent 关闭和清理 THEN 系统 SHALL CONTINUE TO 正确释放所有资源和锁
