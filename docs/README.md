# Aria 文档索引

保留下来的文档以长期有用的内容为主：架构、部署、测试、监控和前端设计。一次性的实施计划、修复总结和部署回执已经从仓库中移除。

## 核心文档

- `README.md`：项目总览、核心能力、快速开始。
- `docs/ARCHITECTURE-REFACTOR.md`：Go Controller + Rust Agent 的职责拆分说明。
- `docs/DEPLOYMENT.md`：整体部署参考。
- `docs/RUST-AGENT-DEPLOYMENT.md`：Rust Agent 部署与运行说明。
- `docs/NGINX-GRPC-CONFIG.md`：gRPC 经过 Nginx 的配置方式。
- `docs/GRPC-TESTING.md`：gRPC 接口测试方法。

## 子模块文档

- `agent-rust/README.md`：Agent 架构、eBPF map、ACL/QoS 处理流程。
- `agent-rust/BUILD-GUIDE.md`：Agent 编译环境和构建步骤。
- `frontend-refactor/DESIGN-SYSTEM.md`：前端视觉和组件设计约束。

## 监控文档

- `deployments/monitoring/README.md`：监控栈整体说明。
- `deployments/monitoring/vmalert/README.md`：告警规则与 vmalert 说明。
- `deployments/monitoring/grafana/dashboards/README.md`：Grafana 面板说明。
