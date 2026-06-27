# 测试覆盖状态（v0.1.0）

**更新时间**: 2026-06-28

## 1. 当前状态

- ✅ RBAC 三态（`off/audit/enforce`）已覆盖接口级权限矩阵测试
- ✅ 前端权限控制已覆盖 composable、路由守卫、页面可见性测试
- ✅ `nodes + monitoring` 已补齐 API 行为级测试（成功/参数错误/边界/错误码）
- ✅ 自动证书签发（`issue/renew`）已补齐核心 API 行为测试（鉴权/参数/续签链路/错误契约）
- ✅ Settings / Backup 已覆盖 `super_admin` 权限边界、备份生命周期、上传、下载与恢复行为
- ✅ 前端单测已接入 CI（`npm run test:run`）
- ✅ 前端工作流上下文测试已覆盖 `Nodes`、`Monitoring`、`Policy Center`、ACL、QoS、Route、IP Group 之间的 `nodeId`、`policyRef`、`policyDomain/kind`、`commandId` 保真与跳转聚焦

## 2. 已覆盖重点

### 后端（Go）

- RBAC 接口行为矩阵：
  - Roles / Users / Routes / ACL / QoS / Tokens / Monitoring 等核心 handler
  - 覆盖 `read/write/commands` 与 `off/audit/enforce` 组合
- Nodes 与 Monitoring 行为测试：
  - 成功路径：`list/get/update/delete`、`stats/health/events/alerts/topology/traffic/node detail/node metrics`
  - 参数错误：无效 UUID、无效 `since`、无效 `range`、无效 path
  - 边界行为：分页钳制、topology 0/1/2 节点、traffic 无节点/节点查询失败降级
  - 错误码契约：`400/404/405/500` 与业务 code 断言
  - 租户隔离：跨租户访问 `404` 行为断言
- 自动证书签发行为测试（`internal/cli/controller_certificates_test.go`）：
  - `issue` 鉴权路径：`runtime_token`（body + `Authorization: Bearer`）与 `enrollment token` 均已覆盖
  - `issue` 成功/失败：成功签发、非法 token、租户不匹配、CSR 非法、存储写入失败
  - `renew` 成功/失败：无历史证书 `404`、历史证书查询失败 `500`、节点不存在 `401`
  - 续签链路：断言 `renewed_from` 证书链路写入（续签 lineage 保持）
  - 协议守卫：`405 Method Not Allowed`、`503 Service Unavailable`、`400 csr_pem required`
- Settings / Backup 行为测试（`internal/api/v2/settings_test.go`）：
  - `super_admin` 专用访问边界
  - `create/list/download/delete/upload/restore` 的基础行为契约
  - restore 事务内清理、恢复表数据并写入审计事件

### 前端（Vue）

- `usePermission` 权限判断（包含 wildcard）
- 路由权限元信息与 guard 跳转
- 关键页面按钮和菜单可见性/禁用态
- Settings 下载通过鉴权 API 客户端获取 blob，避免裸浏览器跳转绕过请求拦截器
- `Monitoring` alert/event context 容错：缺失 `context` / `detail` 时仍能跳到节点详情的告警视图
- `Nodes` 最近命令和活跃告警行级跳转：能携带 `commandId`、`alertId`、`eventType`、`policyRef`、`policyDomain`
- 策略专页上下文回跳：ACL、QoS、Route、IP Group 页面回节点详情时根据命令或策略上下文聚焦 `commands` / `policies`

## 3. CI 状态

- 后端测试与前端单测均已纳入 GitHub Actions
- 最近多轮提交持续通过，测试基线稳定
- `codex/frontend-workflow-closure` 分支最近三批前端工作流修复均通过完整 Actions：`28297743229`、`28297930570`、`28298116308`

## 4. 可选增强（非阻塞 v0.1.0）

- Monitoring 与 VM 客户端交互的更深集成测试
- gRPC 端到端自动化与性能压测基线报告
- 覆盖率统计与阈值门禁（如 `go test -cover` + fail-under）
- 自动证书签发可继续增强：
  - `renew` 场景下证书内容字段（有效期/用途）更细粒度断言
  - `token` 鉴权链路的异常细分（如 token 解析成功但租户查询失败）
  - 与 `register/sync/unregister` 真实生命周期联动的端到端行为测试

## 5. 自动证书签发最终清单（阶段性）

- 已覆盖：
  - `issue`：`runtime_token`（body + bearer）/ `enrollment token` 成功路径
  - `issue`：非法 token、租户不匹配、缺少 node selector、非法 CSR、存储失败
  - `renew`：成功续签 + `renewed_from` lineage 断言
  - `renew`：无历史证书、历史证书查询失败、节点不存在
  - 协议与输入守卫：`405/503/400`
- 待覆盖（非阻塞）：
  - 证书内容级断言增强（`ClientAuth`、有效期边界）在 API 行为层的二次校验
  - 生命周期 E2E（注册触发签发、注销触发撤销、同步返回证书）自动化回归
