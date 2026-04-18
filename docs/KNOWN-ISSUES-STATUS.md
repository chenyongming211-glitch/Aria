# 已知问题状态追踪 (UPDATED ✅)

**更新日期**: 2026-04-18
**基准文档**: `docs/V0.1.0-PRODUCT-BLUEPRINT.md` 第 13 节

---

## 1. API 命名空间不统一（v1/v2/裸路径并存）

**状态**: ✅ **已完全修复**
- 前端 100% 迁移到 `/api/v2/...`
- 后端彻底删除了 `internal/api/v1` 目录，所有业务逻辑已迁移到 `handlers` 包并由 v2 API 承载
- MCP 白名单已更新为 `/api/v2/auth/*`
- 冗余的裸路径 `/version` 等已清理

## 2. 节点身份与运行鉴权未分离

**状态**: 🛠️ **大部分修复**
- `node_id` (UUID) 已全面启用
- mTLS 配置已修复：现在强制要求并验证客户端证书 (BUG-4/B9)
- **遗留**: 运行期 gRPC 请求（Sync/Metrics）目前仍基于 `node_id` 信誉，尚未引入短期动态凭据

## 3. Sync 即时拼装，未版本化

**状态**: ✅ **已完全修复**
- 实现了完整的 Desired/Applied 版本控制系统
- 节点重连后立即触发同步，确保状态强一致性

## 4. 前端菜单与后端领域模型未对齐

**状态**: ✅ **已完全修复**
- 导航结构已与 v0.1.0 蓝图完美对齐
- 修复了菜单名称冲突和路由跳转失效问题

## 5. ACL / Policies / Bandwidth 概念重叠

**状态**: ✅ **已修复**
- **Schema 统一**: 已在 `acl_rules` 中添加 v2 所需列 (BUG-1/B3)
- **逻辑收拢**: 所有的 ACL/QoS 变更都会自动触发版本提升，并由统一的 v2 API 处理
- **前端对齐**: ACL 管理和带宽控制页面已完全适配 v2 node-scoped 模式

---

## 6. 代码审查发现的 Bug 修复状态

| ID | 描述 | 状态 |
|----|------|------|
| B1 | `node_control_states` UNIQUE 约束修复 | ✅ FIXED |
| B2 | RateLimiter Context 取消 Bug | ✅ FIXED |
| B3 | v2 ACL 数据库列缺失 | ✅ FIXED |
| B4 | SuperAdminOnly 越权检查修复 | ✅ FIXED |
| B5 | syncNode 租户隔离（防止跨租户规则泄漏） | ✅ FIXED |
| B6 | ConsumeToken TOCTOU 竞态修复 | ✅ FIXED |
| B7 | createTenantToken 零值 token 修复 | ✅ FIXED |
| B8 | HandleRegister 空公钥 panic 修复 | ✅ FIXED |
| B9 | mTLS 验证失效修复 | ✅ FIXED |
| B10 | MCP 白名单路径错误修复 | ✅ FIXED |
| B11 | Agent 硬编码接口名修复 | ✅ FIXED |
| B12 | gRPC 连接无超时修复 | ✅ FIXED |
| B13 | Endpoint 端口偏移丢失修复 | ✅ FIXED |
| B14 | JWTSecret 并发读写竞态修复 | ✅ FIXED |
| B15 | 吞掉数据库错误修复 | ✅ FIXED |
| B16 | 飞书事件处理器 goroutine 泄漏修复 | ✅ FIXED |
| B17 | Context 中 tenant_id 类型不一致修复 | ✅ FIXED |
| B18 | Config Reload 不更新定时器修复 | ✅ FIXED |
| B19 | QoS mbps=0 导致全拦截修复 | ✅ FIXED |
| B20 | Token 跨 Tab 丢失修复 | ✅ FIXED |
| B21 | 路由守卫过期检查修复 | ✅ FIXED |
| B22 | AI 助手 v-html XSS 风险修复 | ✅ FIXED |
| B23 | Settings 上传 Token 来源错误修复 | ✅ FIXED |
| B24 | updateNodeRemote 未导出修复 | ✅ FIXED |
| B25 | Dashboard Icon 未定义修复 | ✅ FIXED |
| B26 | Settings ElMessageBox 缺失修复 | ✅ FIXED |
| B27 | deleteRoute 传参错误修复 | ✅ FIXED |

---

## 总结
Aria SD-WAN 目前已完成 v0.1.0 阶段的所有重大重构与 Bug 修复任务。系统处于稳定状态，随时准备进行交付演示。
