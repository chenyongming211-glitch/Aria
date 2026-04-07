# 实施计划

- [x] 1. 编写 bug condition 探索性测试
  - **Property 1: Bug Condition** - Mutex 中毒导致级联失败
  - **关键**: 此测试必须在未修复的代码上失败 - 失败确认 bug 存在
  - **不要在测试失败时尝试修复测试或代码**
  - **注意**: 此测试编码了预期行为 - 在实施后通过时将验证修复
  - **目标**: 展示证明 bug 存在的反例
  - **作用域 PBT 方法**: 对于确定性 bug，将属性作用域限定为具体的失败案例以确保可重现性
  - 测试实施细节来自设计文档中的 Bug Condition
  - 测试断言应匹配设计文档中的 Expected Behavior Properties
  - 在未修复的代码上运行测试
  - **预期结果**: 测试失败（这是正确的 - 证明 bug 存在）
  - 记录发现的反例以理解根本原因
  - 当测试编写、运行并记录失败时标记任务完成
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9_

- [x] 2. 编写保留属性测试（在实施修复之前）
  - **Property 2: Preservation** - 正常操作行为保持不变
  - **重要**: 遵循观察优先方法
  - 在未修复的代码上观察非 bug 输入的行为
  - 编写基于属性的测试捕获来自 Preservation Requirements 的观察行为模式
  - 基于属性的测试生成许多测试用例以提供更强的保证
  - 在未修复的代码上运行测试
  - **预期结果**: 测试通过（这确认了要保留的基线行为）
  - 当测试编写、运行并在未修复的代码上通过时标记任务完成
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9, 3.10_

- [x] 3. 修复 Mutex 中毒漏洞

  - [x] 3.1 替换 Mutex 类型声明
    - 将 `last_sync_peers: Arc<StdMutex<Vec<GrpcPeerInfo>>>` 替换为 `last_sync_peers: Arc<tokio::sync::Mutex<Vec<GrpcPeerInfo>>>`
    - 将 `log_handle: Arc<StdMutex<Option<reload::Handle<EnvFilter, Registry>>>>` 替换为 `log_handle: Arc<tokio::sync::Mutex<Option<reload::Handle<EnvFilter, Registry>>>>`
    - 将 `current_log_level: Arc<StdMutex<String>>` 替换为 `current_log_level: Arc<tokio::sync::Mutex<String>>`
    - _Bug_Condition: isBugCondition(input) where input.mutex_type == "std::sync::Mutex" AND input.lock_method == ".lock().unwrap()" AND input.context == "async" AND mutex_is_poisoned(input.mutex)_
    - _Expected_Behavior: 使用 tokio::sync::Mutex 和 .lock().await，即使发生 panic 也不会导致 Mutex 中毒和级联失败_
    - _Preservation: 所有现有功能（同步、命令处理、metrics 收集、日志管理）必须保持完全相同的行为_
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_

  - [x] 3.2 更新 Mutex 初始化
    - 将 `Arc::new(StdMutex::new(Vec::new()))` 替换为 `Arc::new(tokio::sync::Mutex::new(Vec::new()))`
    - 将 `Arc::new(StdMutex::new("info".to_string()))` 替换为 `Arc::new(tokio::sync::Mutex::new("info".to_string()))`
    - _Requirements: 2.1, 2.2, 2.3_

  - [x] 3.3 替换主同步循环中的锁获取（Line 415）
    - 将 `self.last_sync_peers.lock().unwrap().len()` 替换为 `self.last_sync_peers.lock().await.len()`
    - _Requirements: 2.1, 2.6_

  - [x] 3.4 替换 Unix socket 任务中的锁获取（Line 618）
    - 将 `last_sync_peers.lock().unwrap().clone()` 替换为 `last_sync_peers.lock().await.clone()`
    - _Requirements: 2.1, 2.7_

  - [x] 3.5 替换日志级别管理中的锁获取（Line 1159, 1172, 1199）
    - 将 `log_handle.lock().unwrap()` 替换为 `log_handle.lock().await`
    - 将 `current_log_level.lock().unwrap()` 替换为 `current_log_level.lock().await`
    - _Requirements: 2.2, 2.3, 2.10_

  - [x] 3.6 替换远程命令执行中的锁获取（Line 1235, 1320, 1349）
    - 将 `self.last_sync_peers.lock().unwrap().len()` 替换为 `self.last_sync_peers.lock().await.len()`
    - 将 `self.last_sync_peers.lock().unwrap().clone()` 替换为 `self.last_sync_peers.lock().await.clone()`
    - _Requirements: 2.1, 2.8_

  - [x] 3.7 替换 sync 函数中的锁获取（Line 1470）
    - 将 `*self.last_sync_peers.lock().unwrap() = peers` 替换为 `*self.last_sync_peers.lock().await = peers`
    - _Requirements: 2.1, 2.9_

  - [x] 3.8 移除不必要的 import
    - 移除 `use std::sync::{Arc, Mutex as StdMutex};` 中的 `Mutex as StdMutex`
    - 保留 `use tokio::sync::{Mutex, mpsc, oneshot};` 中的 `Mutex`
    - _Requirements: 2.4_

  - [x] 3.9 验证 bug condition 探索性测试现在通过
    - **Property 1: Expected Behavior** - Mutex 中毒不导致级联失败
    - **重要**: 重新运行任务 1 中的相同测试 - 不要编写新测试
    - 任务 1 中的测试编码了预期行为
    - 当此测试通过时，确认满足预期行为
    - 重新运行步骤 1 中的 bug condition 探索性测试
    - **预期结果**: 测试通过（确认 bug 已修复）
    - _Requirements: Expected Behavior Properties from design_

  - [x] 3.10 验证保留测试仍然通过
    - **Property 2: Preservation** - 正常操作行为保持不变
    - **重要**: 重新运行任务 2 中的相同测试 - 不要编写新测试
    - 重新运行步骤 2 中的保留属性测试
    - **预期结果**: 测试通过（确认没有回归）
    - 确认修复后所有测试仍然通过（没有回归）

- [ ] 4. Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。
