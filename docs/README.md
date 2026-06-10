# Aria 文档索引

保留下来的文档以长期有用的内容为主：架构、部署、测试、监控和前端设计。过时的一次性实施计划、修复总结和部署回执已经从仓库中移除；当前仍保留并维护可执行的产品蓝图与阶段优先级基线。

## 核心文档

- `README.md`：项目总览、核心能力、快速开始。
- `docs/API-V2-WHITEPAPER.md`：API v2 当前基线 + 目标接口白皮书，定义域边界、端点清单与统一响应格式。
- `docs/API-VERSION-AUDIT.md`：API 版本收敛审计，当前基线为 v2-only。
- `docs/KNOWN-ISSUES-STATUS.md`：按当前代码复核后的结构问题状态，区分“底层已收敛”和“产品闭环未完成”的部分。
- `docs/CONFIRMED-BUGS.md`：当前仍开放的真实 bug，以及本轮已重新验证关闭的历史问题。
- `docs/TEST-COVERAGE-STATUS.md`：测试覆盖现状与 CI 接入状态。
- `docs/CERT-AUTO-ISSUANCE-DESIGN.md`：自动证书签发方案（第一阶段设计）。
- `docs/V0.1.0-PRODUCT-BLUEPRINT.md`：v0.1.0 产品蓝图与后续 6 周实施基线，定义前端导航、后端分层、节点接入、策略与运维闭环。
- `docs/QOS-PRODUCT-DECISION.md`：QoS 产品决策；明确取消旧的“三级 QoS”/`service / peers / ip` 分类模型，统一为节点级匹配规则。
- `docs/RBAC-DESIGN.md`：高级 RBAC 设计与落地进度（含 audit/enforce 模式）。
- `docs/ARCHITECTURE-REFACTOR.md`：Go Controller + Rust Agent 的当前职责拆分与接口连接方式。
- `docs/DEPLOYMENT.md`：整体部署参考。
- `docs/RUST-AGENT-DEPLOYMENT.md`：Rust Agent 部署与运行说明。
- `docs/NGINX-GRPC-CONFIG.md`：gRPC 经过 Nginx 的配置方式。
- `docs/GRPC-TESTING.md`：gRPC 接口测试方法。

## 子模块文档

- `frontend/DESIGN-SYSTEM.md`：前端视觉和组件设计约束。

## 监控文档

- `deployments/monitoring/README.md`：监控栈整体说明。
- `deployments/monitoring/vmalert/README.md`：告警规则与 vmalert 说明。
- `deployments/monitoring/grafana/dashboards/README.md`：Grafana 面板说明。
