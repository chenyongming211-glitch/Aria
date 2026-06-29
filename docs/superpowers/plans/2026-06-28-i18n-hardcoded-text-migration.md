# 前端 i18n 硬编码文本迁移方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 27 个 `.vue` 文件中硬编码的中文文本迁移到 i18n 系统，使语言切换真正生效。

**Architecture:** i18n 基础设施已完备——`i18n/index.ts` 有中英文双语字典（~200 个 key）、`t()` 函数支持嵌套 key 解析（如 `t('status.policy.idle')`）、`useAppStore` 托管当前语言。问题是所有 `.vue` 组件直接写死了中文，没有任何 `$t()` 调用。

**Tech Stack:** Vue 3 + TypeScript, Element Plus, Vitest.

---

## 当前状态

```
i18n 字典:  ✅ 中英文双语，~200 key，结构完整
t() 函数:   ✅ 支持嵌套 key 解析
Vue 组件:   ❌ 27 个文件全部硬编码中文，0 次 $t()
```

## 迁移范围

按风险分级，分 4 批：

| 批次 | 文件 | 数量 | 风险 |
|------|------|:---:|:---:|
| B1 | UI 组件 + 布局 | 8 | 低 |
| B2 | 简单页面 | 8 | 中 |
| B3 | 复杂页面 | 10 | 高 |
| B4 | 跳过 | 1 | — |

### B1: UI 组件 + 布局（低风险）

这些组件逻辑简单，只做展示，不改业务。

- [x] `components/ui/ActionIconButton.vue`
- [x] `components/ui/DataPanel.vue`
- [x] `components/ui/FilterBar.vue`
- [x] `components/ui/MetricStrip.vue`
- [x] `components/ui/PageHeader.vue`
- [x] `components/ui/StatusBadge.vue`
- [x] `components/layout/Layout.vue`
- [x] `components/layout/TenantSelector.vue`

### B2: 简单页面（中风险）

页面结构固定，不涉及复杂交互。

- [x] `Login.vue`
- [x] `ChangePassword.vue`
- [x] `Dashboard.vue`
- [x] `Roles.vue`
- [x] `Settings.vue`
- [x] `TenantManagement.vue`
- [x] `Tokens.vue`
- [x] `PolicyContextBanner.vue`（如有硬编码）

### B3: 复杂页面（高风险）

包含大量状态映射、条件渲染、Element Plus 表格/表单配置。

- [x] `Nodes.vue`（1495 个中文字符，最大）
- [x] `NodeMonitorDetail.vue`
- [x] `Monitoring.vue`
- [x] `Routing.vue`
- [x] `IPGroups.vue`
- [x] `ACLRules.vue`
- [x] `BandwidthControl.vue`
- [x] `Policies.vue`
- [x] `VpnTopology.vue`
- [x] `App.vue`（无用户可见硬编码文本）

### B4: 跳过

- [ ] `AIAssistant.vue` — 等 Hermes Agent 重做时一起处理

---

## 每批执行流程

### Step 1: 扫描硬编码文本

对当前批次每个 `.vue` 文件，提取所有硬编码中文：

```bash
# 示例：找出模板中硬编码的 label/placeholder/title
rg 'label|placeholder|title|el-button|el-tag' --include='*.vue' frontend/src/views/Nodes.vue
```

记录：
- 哪些 key 已在 `i18n/index.ts` 中存在 → 直接替换为 `$t('xxx')`
- 哪些 key 缺失 → 进入 Step 2

### Step 2: 补充翻译 key

**文件:** `frontend/src/i18n/index.ts`

在对应 domain 下新增 key，同时补 `en` 和 `zh` 两种语言。命名规则：

```
domain.section.field

示例：
nodeManagement.editNode     → '编辑节点' / 'Edit Node'
nodeManagement.saveSuccess  → '保存成功' / 'Saved successfully'
nodeManagement.saveError    → '保存失败' / 'Save failed'
```

### Step 3: 替换硬编码文本

模板中使用 `$t()`：

```vue
<!-- 替换前 -->
<el-table-column label="主机名" />
<el-button>编辑</el-button>

<!-- 替换后 -->
<el-table-column :label="$t('nodeManagement.hostname')" />
<el-button>{{ $t('common.edit') }}</el-button>
```

脚本中使用 `t()`：

```ts
// 替换前
ElMessage.success('节点设置更新成功')

// 替换后
ElMessage.success(t('nodeManagement.updateSuccess'))
```

`ElMessage` 等调用需在 `<script setup>` 中 import `t`：

```ts
import { t } from '@/i18n'
```

### Step 4: Element Plus 组件文本也走 i18n

Element Plus 的分页器、表格空状态等支持传入文本参数：

```vue
<el-table :empty-text="$t('common.noData')" />
<el-pagination :total-text="$t('common.total')" />
```

### Step 5: 验证

```bash
# 运行全量测试
npm run test:run

# 构建验证
npm run build

# 手动切换语言，检查中文/英文渲染一致
```

### Step 6: 提交

每个批次一个 commit：

```
frontend: i18n migrate {batch-name}
```

---

## 处理规则

### 不需要 $t() 的场景

| 场景 | 原因 |
|------|------|
| CSS class 名、ID | 不是用户可见文本 |
| 变量名、key 名 | 代码内部使用 |
| `console.log` | 调试信息 |
| 纯数字 | 不需要翻译 |

### 必须 $t() 的场景

| 场景 | 示例 |
|------|------|
| `<el-button>` 文本 | `确定` → `$t('common.confirm')` |
| `<el-table-column label="...">` | `主机名` → `$t('nodeManagement.hostname')` |
| `<el-descriptions-item label="...">` | `区域` → `$t('nodeManagement.region')` |
| `<el-tag>` 内嵌文本 | 状态标签 |
| `ElMessage.success('...')` | 通知消息 |
| `ElMessageBox.confirm('...')` | 确认对话框 |
| 页面标题、说明文字 | 页面内容 |

---

## 最终效果

```
迁移前                              迁移后
──────                              ──────
中英文切换：无效                     中英文切换：生效
所有文本硬编码中文                    所有文本走 $t() 或 t()
i18n 字典：被架空                    i18n 字典：被使用
新增文本：直接写死中文                新增文本：先加 key 再用 $t()
```

### 验证标准

- [ ] 切换到英文后，所有页面 UI 文本变为英文（除 Element Plus 组件内部文本）
- [ ] 切换到中文后，所有页面 UI 文本变为中文
- [ ] `npm run build` 无错误
- [ ] `npm run test:run` 全量通过
- [ ] 至少抽查 3 个页面（一个简单 + 两个复杂），两种语言下文本均不出现 `key` 名泄露（如显示 `nodeManagement.hostname` 表示 key 未找到）

---

## 关联待办：去除「AI 味」UI

**状态:** 🔴 未开始

**目标:** 替换 SaaS 模板风格的装饰性 UI 元素为运维控制台风格。

**受影响文件:**

| 文件 | 问题 |
|------|------|
| `Dashboard.vue` | KPI 卡片渐变背景、发光阴影 |
| `Nodes.vue` | 彩色 KPI badge、hero 区域 |
| `Login.vue` | hero section、发光输入框 |
| `Layout.vue` | 渐变背景 |
| `global.css` | 全局 CSS 变量中 gradient/glow/shadow 定义 |

**设计原则:**
- 高密度、低装饰，状态颜色仅语义使用
- 去除渐变背景、发光阴影、彩色 KPI 卡片、大 Hero 区域
- 保持在运维控制台的朴素视觉风格内

**建议:** 与 i18n 迁移同批或在 `platform-backup-cert-closure` 完成后独立分支执行。`AIAssistant.vue` 跳过（等 Hermes）。

---

## 关联待办：文档与代码对齐

**状态:** 🔴 未开始

**目标:** 整理 `docs/` 下与当前代码不符的文档，消除误导。

**已知不符项:**

| 文档 | 问题 |
|------|------|
| `architecture-refactor.md` | Southbound 写成 HTTP register/unregister/network，Agent 实际全 gRPC |
| `control-plane-phase1.md` | B1-B5 checklist 大量未勾，代码实际已全部完成 |
| `remove-ai-look-ui.md` | 文件已不存在，但曾作为计划引用 |
| 各 plan 文件的 checklist | 多处 `[ ]` 未勾，但对应功能已上线 |

**整理策略:**
1. 已完成的功能 → 在文档顶部标注 `**状态: 已完成**`，不必逐项补勾
2. 已过时的文档 → 同步当前代码状态或归档
3. 新建文档时约定：完成后必须更新状态标记

**建议:** 在各功能分支收尾时顺带更新对应文档，避免堆积到最后一次性清理。

---

## 关联待办：IP Group 引用关系展示

**状态:** 🔴 未开始

**现状:** `IPGroups.vue` 表格只展示名称、类型、描述、CIDR 成员、重叠提示，不展示该 Group 被哪些节点的哪些规则引用。用户改动一个 Group 后无法预判影响范围。

**数据模型（正确，不改）:** IP Group 是租户级资源（无 `node_id`），通过 `acl_rules.src_group_id` / `dst_group_id` 和 `qos_rules.group_id` 外键关联到具体节点的规则。

**优化:**

1. **后端:** 新增 `GetIPGroupUsage(tenantID, groupID)` 查询，联查 `acl_rules` + `qos_rules`，返回引用该 Group 的规则列表和对应节点信息。

2. **前端:** `IPGroups.vue` 表格新增「引用节点」列，显示被哪些节点使用、共计多少条规则，点击节点名跳转详情。

3. **前端（可选）:** 删除 IP Group 时若被规则引用，弹出确认框告知影响范围。

---

## 关联待办：垃圾代码清理

**状态:** 🔴 未开始

### 可立即删除（无依赖）

| # | 项目 | 行数/体积 | 说明 |
|:---:|------|:---:|------|
| 1 | `frontend-refactor/` | 226 MB | 废弃重构分支残留，只剩 `node_modules` |
| 2 | `temp-dist/` | 3.4 MB | 构建残留 |
| 3 | `internal/api/handlers/tenant_mgmt.go` | 16 行 | 空壳 struct，无人引用 |
| 4 | `internal/agent/tools/tools.go:25` | 1 函数 | `NewListNodesTool()`，返硬编码假数据，注释已标记废弃 |
| 5 | `pkg/controllerstorage/redis.go:360` | 1 函数 | `NewRateLimiter()`，无调用方 |
| 6 | `internal/cli/controller_serve.go:10` | 1 import | `io/ioutil` 已废弃 (Go 1.16+)，改为 `os.ReadFile` |

### Rust 死代码（~720 行）

| # | 文件 | 内容 | 行数 |
|:---:|------|------|:---:|
| 7 | `wireguard.rs` | `check_dependencies()` | 19 |
| 8 | `agent_runtime.rs` | `load_ebpf_programs()` 单接口版 + `ensure_interface()` | 181 |
| 9 | `grpc_client.rs` | `new()` + `register()` 简化版 | 39 |
| 10 | `metrics.rs` | `MetricsCounters` 整结构 + 4 统计函数 | 84 |
| 11 | `system_optimization.rs` | `verify_optimizations()` | 38 |
| 12 | `routing.rs` | `add_route/remove_route/cleanup` 等 6 方法 | 120 |
| 13 | `shared/src/lib.rs` | `Acl5TupleKey`、`BucketState` 等 7 旧结构 | 70 |
| 14 | `proto/aria_agent.proto` | `AgentService` + 8 message | 80 |
| 15 | `ebpf/` | 死配置字段 (`conntrack_enabled` 等) | 12 |
| 16 | grpc_client.rs 等多处 | 过时 `#[allow(dead_code)]` 注解 | 4 处 |

### 前端死代码

| # | 文件 | 内容 | 行数 |
|:---:|------|------|:---:|
| 17 | `i18n/index.ts` | `settings.*` 37 个 key (× 2 语言) | ~74 |
| 18 | `i18n/index.ts` | `monitoring.*` 12 个 key (× 2 语言) | ~24 |
| 19 | `config/api.ts` | 10 个死 API 常量 | ~10 |
| 20 | `styles/global.css` | `.kpi-grid/.kpi-card/.kpi-label/.kpi-value/.kpi-meta` | ~50 |
| 21 | `styles/global.css` | `.animate-fade-in/.slide-up/.scale-in` + 配套 keyframes | ~50 |
| 22 | `styles/global.css` | `@keyframes glow/float/pulse-dot/slideDown` | ~50 |

### 过时注释

| # | 文件 | 内容 |
|:---:|------|------|
| 23 | `internal/controller/grpc/server.go:138` | 注释引用不存在的 `HandleSync`，应为 `processSync` |
| 24 | `internal/cli/controller_serve.go:1824-1828` | 同上 |

### 清理优先级

```
第1批：删除 frontend-refactor/ + temp-dist/ (229MB 释放)
第2批：删除 Go 死代码 6 处 (20 行)
第3批：删除前端死 CSS/i18n/API 常量 (~250 行)
第4批：删除 Rust 死代码 (~720 行)
第5批：修正过时注释 2 处
```

---

## 关联待办：策略下发状态按需聚焦轮询

**状态:** ✅ 已完成

**落地版本:** `0.2.88`

**正式方案文档:** `docs/superpowers/plans/2026-06-28-focused-status-polling.md`

**说明:** 原草案中的 `GET ?refs=` 已调整为正式实现里的 `POST` JSON body，避免 CIDR、混合 domain 和 URL 编码问题。

1. **后端:** 已新增轻量端点：
   ```
   POST /api/v2/tenants/:tid/policy-deliveries/status
   POST /api/v2/tenants/:tid/nodes/status
   ```
   只查询调用方提交的 policy refs 或 node ids，不全表刷新。

2. **前端:** 已新增 `composables/useFocusedPolling.ts`
   - 首次加载后扫描所有 `pending` / `in_progress` 的规则
   - 对这些规则启动 3 秒轮询，只调轻量端点
   - 局部更新对应行的状态 badge（不重渲染整个表格）
   - 全部达到终端状态（`applied` / `error` / `stale`）后自动停止
   - 用户执行 retry / create 后重新启动
   - 页面隐藏时暂停（`visibilitychange`）

3. **已接入页面：**
   ```ts
   // ACLRules.vue / BandwidthControl.vue / Routing.vue / Policies.vue
   useFocusedPolling(...)
   ```
   同时接入 `Nodes.vue` 和 `NodeMonitorDetail.vue` 的节点状态聚焦刷新。

| | 10s 全量轮询 | 按需聚焦轮询 |
|------|:---:|:---:|
| 请求数据量 | 全表 | 几条 delivery |
| settled 后 | 仍在刷 | 自动停 |
| 后端压力 | O(规则总数) | O(未完成数，通常 0~3) |
| 前端渲染 | 表格重绘 | 局部 badge 更新 |
| 延迟 | ≤10s | ≤3s |
