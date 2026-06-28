# Aria 文档索引

保留下来的文档以长期有用的内容为主：架构、部署、测试、监控和前端设计。过时的一次性实施计划、修复总结和部署回执已经从仓库中移除；当前仍保留并维护可执行的产品蓝图与阶段优先级基线。

## 核心文档

- `README.md`：项目总览、核心能力、快速开始。
- `docs/api-v2-whitepaper.md`：API v2 当前基线 + 目标接口白皮书，定义域边界、端点清单与统一响应格式。
- `docs/api-version-audit.md`：API 版本收敛审计，当前基线为 v2-only。
- `docs/known-issues-status.md`：按当前代码复核后的结构问题状态，区分“底层已收敛”和“产品闭环未完成”的部分。
- `docs/confirmed-bugs.md`：当前真实 bug 的闭合记录，以及本轮已重新验证关闭的历史问题。
- `docs/superpowers/plans/2026-06-27-confirmed-bugfix-closure.md`：BUG-25 到 BUG-35 的分批修复计划，覆盖旧 AI 写入口封禁、租户/节点生命周期、路由下发、Agent 命令状态和监控口径。
- `docs/test-coverage-status.md`：测试覆盖现状与 CI 接入状态。
- `docs/cert-auto-issuance-design.md`：自动证书签发方案（第一阶段设计）。
- `docs/v0.1.0-product-blueprint.md`：v0.1.0 产品蓝图与后续 6 周实施基线，定义前端导航、后端分层、节点接入、策略与运维闭环。
- `docs/node-onboarding-closure-plan.md`：节点接入闭环实施方案，定义 `init -> 注册 -> 获取身份/配置 -> Agent online` 的状态机、实施步骤和验收标准。
- `docs/control-loop-closure-plan.md`：控制闭环实施方案，定义 `前端下发 -> Controller 校验/存储 -> Agent 执行 -> 前端回显` 的状态模型、实施步骤和验收标准。
- `docs/operations-loop-closure-plan.md`：运维闭环实施方案，定义 `监控发现 -> 人工确认 -> 执行结果留痕` 的事件模型、确认流和验收标准；AI 建议暂缓到 Hermes Agent 阶段。
- `docs/frontend-typescript-refactor-plan.md`：前端 TypeScript 渐进重构计划，定义从类型检查、DTO、API composables、Pinia store 到高风险页面迁移的分批实施路径。
- `docs/superpowers/plans/2026-06-28-i18n-hardcoded-text-migration.md`：前端 i18n 硬编码文本迁移方案，将 27 个 `.vue` 文件中硬编码的中文迁移到 i18n 系统，使中英文语言切换真正生效。
- `docs/qos-product-decision.md`：QoS/ACL 产品决策；明确取消旧的“三级 QoS”/`service / peers / ip` 分类模型，统一为节点级策略，并确立 IP Group 为主的产品模型。
- `docs/rbac-design.md`：高级 RBAC 设计与落地进度（含 audit/enforce 模式）。
- `docs/architecture-refactor.md`：Go Controller + Rust Agent 的当前职责拆分与接口连接方式。
- `docs/deployment.md`：整体部署参考。
- `docs/rust-agent-deployment.md`：Rust Agent 部署与运行说明。
- `docs/nginx-grpc-config.md`：gRPC 经过 Nginx 的配置方式。
- `docs/grpc-testing.md`：gRPC 接口测试方法。
- `docs/file-naming-conventions.md`：仓库文件命名规则，说明哪些路径应统一小写、哪些语言约定应保留。

## 子模块文档

- `frontend/design-system.md`：前端视觉和组件设计约束。

## 监控文档

- `deployments/monitoring/README.md`：监控栈整体说明。
- `deployments/monitoring/vmalert/README.md`：告警规则与 vmalert 说明。
- `deployments/monitoring/grafana/dashboards/README.md`：Grafana 面板说明。
