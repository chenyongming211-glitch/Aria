# 节点接入闭环实施方案

本文档把 `init -> 注册 -> 获取身份/配置 -> Agent online` 拆成可落地的产品和工程步骤。目标不是再增加一个注册接口，而是让一台新机器从 0 到能被控制台稳定管理、同步、观测和审计。

## 1. 目标链路

最终接入链路应为：

```text
管理员生成 Enrollment Token
-> 节点执行 aria-agent init
-> Agent 生成本机身份
-> Controller 校验 token
-> Controller 创建或绑定 node_id、tenant_id、assigned_ip
-> Controller 返回 runtime credential 和初始配置
-> Agent 保存 runtime state
-> Agent 启动 WireGuard / gRPC / Sync
-> Controller 看到节点 online
-> 前端显示接入成功、最近同步成功
```

v0.1.0 的验收标准是“自助接入可演示、可回放、可排障”。自动证书签发、低风险自愈、多 Controller 编排不进入第一阶段的必做范围。

## v0.1.0 闭环终点

接入闭环第一阶段的终点是：

```text
管理员生成 Enrollment Token
-> 复制 init 命令
-> Agent 注册
-> Controller 创建 node + assigned_ip + runtime token
-> Agent 完成首次 Sync
-> 前端 30 秒内看到 online 或 degraded
-> 节点详情能解释 last_sync / desired / applied / error
-> 全流程有审计事件
```

到这里即视为 v0.1.0 接入闭环完成，可以停止扩展。第一阶段不继续追求完整证书生命周期、多 Controller 注册、复杂机器证明恢复、无人值守自愈或大而全安装器。

停止规则：一台新机器从 0 接入后，控制台能判断它是真 `online` 还是 `degraded`，并能展示原因和审计记录。

## 2. 接入状态机

前后端需要共享一套接入状态语义，避免把“注册成功”误显示成“运行正常”。

| 状态 | 含义 | 典型来源 |
| --- | --- | --- |
| `uninitialized` | 本机尚未执行 `aria-agent init` 或缺少本地状态 | Agent 本地判断 |
| `enrolling` | Agent 正在向 Controller 注册 | Agent init 过程 |
| `registered` | Controller 已建立节点身份并返回运行凭据 | 注册接口 |
| `syncing` | Agent 正在执行首次或恢复 Sync | Agent runtime |
| `online` | 心跳、runtime credential、Sync、命令流均正常 | Controller 节点状态 |
| `degraded` | 已注册，但 Sync、命令流或运行凭据存在异常 | Controller/Agent 回报 |
| `offline` | 超过阈值未收到心跳或 metrics | Controller cleanup |
| `revoked` | token、证书或节点运行资格被撤销 | Controller 生命周期 |
| `deleted` | 管理员删除节点 | Controller 生命周期 |

第一阶段可以不把所有状态都落库为新 enum，但 API 和前端展示必须能区分：

- 注册成功但首次 Sync 失败：显示 `degraded`，不能显示 `online`。
- 注册成功但还未收到心跳：显示 `syncing` 或 `registered`。
- 节点被 suspended/deleted/banned：不得继续接收策略和命令。

## 3. 租户侧生成 Enrollment Token

控制台应提供“接入节点”入口，而不是只提供 token 列表。

管理员流程：

1. 选择租户。
2. 创建 Enrollment Token。
3. 设置用途、最大使用次数、过期时间。
4. 只展示一次完整 token。
5. 生成可复制的安装命令。
6. 等待节点上线并跳转到节点详情。

示例命令：

```bash
aria-agent init \
  --controller https://controller.example.com \
  --token tk_xxx \
  --hostname edge-sh-01 \
  --region sh \
  --interface eth0
```

边界要求：

- Enrollment Token 只用于节点首次注册。
- Enrollment Token 不得和用户 JWT、runtime token 混用。
- Token 列表和节点详情默认只显示 preview、用途、使用次数、过期时间和使用节点，不返回完整 secret。

## 4. Agent init 本地初始化

`aria-agent init` 应完成以下动作：

1. 生成或读取 `machine_id`。
2. 生成 WireGuard keypair。
3. 采集 hostname、region、public IP、interface、kernel/eBPF 能力。
4. 调用 Controller 注册接口。
5. 保存 Controller 返回的 runtime state。
6. 立刻触发首次 Sync。

推荐拆分静态配置与运行状态：

```text
agent.yaml          controller 地址、region、interface、启动参数
agent-state.json    node_id、tenant_id、assigned_ip、runtime_token、last_sync 状态
```

这样用户修改 bootstrap 配置时，不会误删节点身份或运行凭据。

## 5. Controller 注册事务

注册接口需要尽量原子化，避免“token 已消耗但节点未保存”或“节点已保存但没有运行凭据”的半成品状态。

建议事务步骤：

1. 校验 Enrollment Token 存在、未过期、未耗尽、属于目标租户。
2. 校验 machine_id、public_key、hostname 是否允许注册或重注册。
3. 分配或复用稳定 `node_id`。
4. 分配或复用 `assigned_ip`。
5. 保存节点记录。
6. 保存成功后再消耗 Enrollment Token。
7. 生成 runtime token。
8. 写入审计事件 `node.registered` 或 `node.reregistered`。
9. 返回注册结果。

注册响应至少应包含：

```json
{
  "node_id": "uuid",
  "tenant_id": "uuid",
  "assigned_ip": "100.64.0.2",
  "runtime_token": "rt_xxx",
  "runtime_token_expires_at": 1234567890,
  "controller_grpc": "https://controller.example.com",
  "sync_interval": 5
}
```

关键约束：

- 必须先保存节点成功，再消耗 Enrollment Token。
- `tenant_id` 一旦建立，不允许跨租户漂移。
- `public_key` 可以轮换，但不能改变节点租户归属。
- 重注册必须有 runtime token、同租户 enrollment flow 或 machine proof 约束，不能只靠 hostname。

## 6. 首次 Sync 与上线判定

注册成功后，Agent 必须保存 runtime state，然后立刻执行首次 Sync：

```text
Agent -> gRPC Sync(node_id, runtime_token)
Controller -> peers/routes/acl/qos/ip_groups/certs/domain_versions
Agent -> apply
Agent -> report applied_state / observed_state
```

`online` 不应只等于“注册成功”。上线判定至少需要：

1. Controller 最近收到 heartbeat 或 metrics。
2. runtime token 有效。
3. command stream 或 Sync 通道正常。
4. 最近一次 Sync 没有硬失败。
5. 节点未处于 suspended、deleted、banned。

首次 Sync 失败时：

- Agent 保存错误原因。
- Controller 标记 `observed_state=degraded` 或等价状态。
- 前端节点详情显示 `last_sync_error`。
- 不得把节点展示为完全健康。

## 7. 前端接入向导

控制台建议提供“接入节点”弹窗或页面：

1. 选择租户。
2. 创建或选择 Enrollment Token。
3. 展示一次性 token 和安装命令。
4. 轮询 token 使用情况和节点注册结果。
5. 节点出现后进入 `registered/syncing` 状态。
6. 收到首次 Sync 成功后显示 `online`。
7. 提供跳转到节点详情的入口。

Nodes 页面和节点详情至少展示：

```text
接入状态
Node ID
Assigned IP
Runtime Mode
Last Seen
Last Sync
Desired Version
Applied Version
Observed State
Last Error
Enrollment Token preview 或来源标签
```

## 8. 生命周期和异常场景

| 场景 | 期望行为 |
| --- | --- |
| Agent 重启 | 读取本地 state，直接 runtime sync，不重新消耗 Enrollment Token |
| runtime token 过期 | 通过 Sync 或 refresh 流程轮换 runtime token |
| 本地 state 丢失 | 需要重新 init，或通过受控 machine proof 恢复 |
| hostname 重复 | 同租户内按明确规则处理，跨租户必须拒绝 |
| token 过期或耗尽 | 注册失败，前端显示明确失败原因 |
| 节点被删除 | runtime token 失效，Agent 不能继续 Sync 或 command stream |
| 节点被 suspended/banned | 拒绝策略变更和命令，但保留审计与可观测状态 |
| 首次 Sync 失败 | 节点进入 degraded，保留错误、允许重试 |
| Controller 重启 | Agent 使用本地 runtime state 恢复连接并重新 Sync |

## 9. 验收标准

### 2026-06-25 当前收口状态

- Controller / Agent 注册、runtime token、首次 Sync、节点状态和证书基础链路已经具备。
- Nodes 页面已经从静态提示升级为接入向导：可创建 Enrollment Token、生成真实 `aria-agent init --server ... --token ... --controller-api-url ...` 命令，并提供复制与验证清单。
- 本阶段剩余工作不再扩展安装器，而是在线上用一台新机器从 0 执行 init、启动 Agent，并按下列验收项记录证据。

接入闭环完成后，应能通过以下验收：

1. 新租户创建 Enrollment Token。
2. 新机器执行一条 `aria-agent init ...`。
3. Controller 创建节点并分配 `node_id + assigned_ip`。
4. Agent 保存 runtime state。
5. Agent 立刻完成第一次 Sync。
6. 前端 Nodes 列表在 30 秒内显示节点。
7. 节点详情显示 `desired/applied/observed/last_sync`。
8. 重启 Agent 后不消耗新 token，仍能上线。
9. token 过期、耗尽、租户不匹配时注册失败且错误可见。
10. 删除或禁用节点后，Agent 不能继续 command stream 或 Sync。
11. 全流程有审计事件：token created、node registered、sync applied、node offline/deleted。
12. 注册、重启、token 过期、节点删除至少有自动化回归测试。

## 10. 实施顺序

建议按以下顺序推进，避免把证书、自愈、多集群混进第一阶段：

1. 收口接入状态机和前端展示。
2. 拆分 Agent bootstrap config 与 runtime state。
3. 补齐注册接口事务边界和重注册规则。
4. 注册成功后强制首次 Sync，并把首次 Sync 结果纳入上线判定。
5. 做前端接入向导。
6. 补齐 E2E 或集成测试：注册、重启、token 过期、节点删除。
7. 后续再接自动证书签发阶段 B。

## 11. 非目标

第一阶段不做：

- 完整自愈闭环。
- 多 Controller / 多区域联邦。
- ACE 事件模型替换现有状态表。
- 所有证书生命周期自动化。
- Relay 或边缘中继架构。

这些内容应在接入闭环稳定后单独推进。
