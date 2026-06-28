# 自动证书签发设计（v0.1.x 第一阶段）

**版本**: v0.2
**日期**: 2026-06-28
**优先级**: P2（生产就绪）  
**范围**: Controller 侧自动签发/续签/撤销基础能力

---

## 1. 目标与边界

### 1.1 目标

在现有 Runtime Token 基础上，补齐 mTLS 证书生命周期能力：

1. 自动签发：支持基于 CSR 的节点证书签发
2. 自动续签：支持到期前阈值续签
3. 自动撤销：节点删除/禁用后证书失效
4. 全链路可追踪：证书元数据可审计

### 1.2 本阶段不做

- 多级 CA / 跨集群 CA 联邦
- 前端完整证书管理 UI
- OCSP/CRL 在线分发服务（先做数据库状态撤销）

---

## 2. 架构与数据模型

### 2.1 核心组件

- `internal/security/certissuance`: 证书签发服务
  - 解析并校验 CSR
  - 按策略签发客户端证书
- `pkg/controllerstorage`: 证书元数据持久化
  - 记录 issued/revoked/expired 状态
  - 支持续签扫描（按到期时间）

### 2.2 数据表（node_certificates）

新增 `node_certificates`：

- `tenant_id`, `node_id`
- `serial_number`
- `cert_pem`, `ca_pem`
- `not_before`, `not_after`
- `status`（`issued` / `revoked` / `expired`）
- `issued_at`, `revoked_at`, `revoke_reason`
- `renewed_from`

说明：

- `node_id` 唯一，确保每节点当前仅一条活动证书记录
- 续签时保留历史链路（`renewed_from`）

---

## 3. 生命周期流程

### 3.1 首次签发

1. Agent 生成本地私钥与 CSR
2. Agent 在 gRPC Register 请求中带上 `csr_pem`
3. Controller 先完成 enrollment/runtime token 绑定，确认请求节点身份
4. Controller 校验 CSR 签名，签发客户端证书并返回证书链
5. 写入 `node_certificates`（`issued`）
6. Agent 将 CA、客户端证书和私钥原子写入 `--ca-cert`、`--client-cert`、`--client-key` 指定路径

### 3.2 自动续签

1. 定时任务扫描即将过期证书（如 `< 72h`）
2. Agent 提交新 CSR
3. Controller 签发新证书并更新元数据（记录 `renewed_from`）
4. Agent 写入新证书后继续使用当前 runtime token 和 mTLS 配置
5. 续签失败时，Agent 将 `certificate renew failed: ...` 写入同步观测状态；Nodes 与 Monitoring 展示最近失败原因

### 3.3 撤销失效

节点删除/禁用时：

1. 将当前 `issued` 证书状态置为 `revoked`
2. 写入 `cert.revoked` 审计事件，记录节点状态、原因和撤销数量
3. Nodes 与 Monitoring 展示当前证书状态、撤销时间和撤销原因
4. 后续连接按证书状态拒绝（策略由鉴权层执行）

---

## 4. 安全策略

- 仅接受 CSR 签名合法请求（`CheckSignature`）
- 客户端证书用途限定为 `ClientAuth`
- 证书有效期默认短周期（建议 7~30 天）
- 租户隔离：证书元数据与节点/租户绑定
- CA 私钥仅在 Controller 侧持有，不下发
- 注册期签发绑定 enrollment/runtime token 解析出的节点身份，不信任请求体里的跨节点身份
- 生命周期撤销只更新当前 `issued` 证书，不覆盖历史 `expired/revoked` 状态

---

## 5. 测试与验收

### 5.1 单元测试

- CSR 合法 -> 签发成功
- CSR 非法 -> 拒绝
- 证书内容断言：
  - Subject / SAN
  - `ClientAuth` 扩展用途
  - 有效期边界

### 5.2 行为测试（后续）

- [x] 节点删除后证书状态变为 `revoked`
- [x] 节点暂停、封禁后证书状态变为 `revoked`
- [x] 即将过期证书可续签
- [x] 跨租户证书请求拒绝
- [x] 注册期 gRPC Register 可以返回 Controller 签发的证书链

---

## 6. 分阶段落地计划

### 阶段 A

- [x] 设计文档
- [x] 证书元数据表与存储方法
- [x] CSR 签发服务与基础单测

### 阶段 B

- [x] 接入注册/续签 API（REST/gRPC）
- [x] 接入节点禁用/删除/封禁撤销流程
- [x] 加入签发、续签失败和撤销审计事件
- [x] 接入 Nodes / Monitoring 可见性

### 阶段 C（增强）

- [ ] 管理端证书可观测性（列表/过期预警）
- [ ] 覆盖率门禁与续签压测
- [ ] gRPC mTLS 鉴权强制检查证书状态
- [ ] 证书轮换演练 runbook
