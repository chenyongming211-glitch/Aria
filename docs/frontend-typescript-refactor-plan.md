# Frontend TypeScript Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `frontend` 从纯 JS/Vue 逐步迁移到 TypeScript，优先类型化 API DTO、状态流、Pinia store 和高风险业务模块，降低字段漂移、状态字符串误用和权限上下文错误。

**Architecture:** 不重写 Vue 3/Element Plus/Pinia/Vite 技术栈，只在现有结构上增加类型层。第一阶段允许 JS 与 TS 共存，先把类型检查接入构建，再从 `types/`、`composables/useXxxApi`、`stores/` 逐步迁移，页面组件最后迁。

**Tech Stack:** Vue 3.4, Vite 4.5, Pinia 2.1, Vue Router 4.2, Element Plus 2.4, Axios, ECharts, Vitest, TypeScript 5.x, vue-tsc.

---

## Long-Term Frontend Target

长期目标不是更换前端框架，而是把现有 Vue 控制台升级成类型边界清晰、组件语义统一、CI 能拦截类型错误的后台管理前端：

- **Framework layer:** 继续使用 Vue 3，不迁移到 React、Next.js 或 Nuxt。
- **Language layer:** 从 JavaScript 渐进迁移到 TypeScript，允许 JS 与 TS 在第一阶段共存。
- **State layer:** 继续使用 Pinia，但将 `user`、`tenant`、`node` 等核心 store 类型化。
- **Router layer:** 继续使用 Vue Router，但将 route meta、权限要求、租户上下文等路由相关约束类型化。
- **UI layer:** 继续使用 Element Plus，但逐步沉淀内部业务组件，例如 page shell、KPI card、data panel、filter toolbar、status badge 和 action buttons。
- **Engineering layer:** 增加 `vue-tsc` 和 `npm run type-check`，并在迁移稳定后让 GitHub Actions 的 `frontend-build` job 执行 type-check。

## Current Baseline

- `frontend/package.json` 已安装 `typescript` 和 `vue-tsc`。
- `frontend/tsconfig.json`、`frontend/tsconfig.node.json`、`frontend/src/env.d.ts` 已建立 JS/Vue 与 TS 共存的类型检查基线。
- GitHub Actions `frontend-build` job 已在单测后执行 `npm run type-check`。
- 当前还没有业务 `.ts` / `.tsx` 源码文件；后续从共享类型和 API composables 开始迁移。
- 当前前端源码约为 `45` 个 `.js`、`20` 个 `.vue`。
- 业务高风险区集中在：
  - `frontend/src/composables/useApi.js`
  - `frontend/src/composables/useMonitorApi.js`
  - `frontend/src/composables/useAgentProxyApi.js`
  - `frontend/src/composables/usePolicyApi.js`
  - `frontend/src/composables/useAclApi.js`
  - `frontend/src/composables/useQosApi.js`
  - `frontend/src/stores/user.js`
  - `frontend/src/stores/tenant.js`
  - `frontend/src/stores/node.js`
- 现有 CI 的 `frontend-build` job 已执行 `npm ci`、`npm run test:run`、`npm run build`，但还没有 `npm run type-check`。

## Scope

本计划只处理前端 TypeScript 渐进迁移，不做以下事项：

- 不切 React、Next、Nuxt。
- 不升级 Vite/Vue/Element Plus 主版本。
- 不重写页面 UI。
- 不改变后端 API。
- 不把所有 `.vue` 页面一次性改成 `<script setup lang="ts">`。
- 不把测试框架从 Vitest 换掉。

## Target Shape

迁移完成后的前端应该保持当前目录结构，但增加类型边界：

```text
frontend/
  tsconfig.json
  tsconfig.node.json
  src/
    env.d.ts
    types/
      api.ts
      auth.ts
      tenant.ts
      node.ts
      policy.ts
      monitoring.ts
      agent.ts
      permission.ts
    composables/
      useApi.ts
      useMonitorApi.ts
      useAgentProxyApi.ts
      usePolicyApi.ts
      useAclApi.ts
      useQosApi.ts
    stores/
      user.ts
      tenant.ts
      node.ts
```

## Migration Rules

- 每批只迁移一组边界，保持 PR 可 review。
- 每次重命名 `.js -> .ts` 后立刻运行对应单测和 `npm run type-check`。
- 所有 imports 保持不写扩展名，例如 `@/composables/useMonitorApi`，避免迁移期间反复改调用方。
- API 类型优先描述后端原始字段，页面展示字段可以单独定义 normalized view model。
- 状态字符串必须使用 union type，不再散落裸字符串。
- 允许早期 `unknown` 和窄范围类型断言，但不要使用全局 `any` 掩盖模型不清晰的问题。

## Type Model Baseline

第一批类型应覆盖控制台核心状态：

```ts
// frontend/src/types/api.ts
export interface ApiEnvelope<T> {
  success: boolean
  data?: T
  message?: string
  error?: string
  code?: string
}

export interface ListResult<T> {
  items: T[]
  total?: number
  limit?: number
  offset?: number
}

export type ISODateTimeString = string
export type TenantId = string
export type NodeId = string
export type PolicyId = string
export type CommandId = string
export type AlertId = string
```

```ts
// frontend/src/types/permission.ts
export type Permission =
  | '*'
  | 'nodes:read'
  | 'nodes:write'
  | 'routes:read'
  | 'routes:write'
  | 'acls:read'
  | 'acls:write'
  | 'ip-groups:read'
  | 'ip-groups:write'
  | 'qos:read'
  | 'qos:write'
  | 'blacklist:read'
  | 'blacklist:write'
  | 'monitoring:read'
  | 'commands:write'
  | 'tokens:read'
  | 'tokens:write'
  | 'users:read'
  | 'users:write'
  | 'roles:read'
  | 'roles:write'
  | 'ai:use'
  | 'policies:read'
  | 'settings:read'
  | 'settings:write'

export type RoleName = 'super_admin' | 'admin' | 'operator' | 'viewer'
```

```ts
// frontend/src/types/node.ts
import type { ISODateTimeString, NodeId, TenantId } from './api'

export type NodeAvailabilityStatus = 'online' | 'offline' | 'degraded' | 'unknown'
export type NodeRuntimeStatus = 'registered' | 'syncing' | 'online' | 'degraded' | 'offline' | 'revoked' | 'deleted'
export type StateConvergence = 'idle' | 'converged' | 'diverged' | 'pending'
export type ObservedState = 'idle' | 'pending' | 'applied' | 'failed' | 'stale' | 'degraded'

export interface NodeRecord {
  id: NodeId
  tenant_id?: TenantId
  hostname?: string
  assigned_ip?: string
  private_ip?: string
  public_ip?: string
  endpoint?: string
  region?: string
  status?: string
  availability_status?: NodeAvailabilityStatus
  runtime_mode?: string
  kernel_version?: string
  last_seen?: ISODateTimeString | number
  last_sync_at?: ISODateTimeString
  last_sync_error?: string
  desired_state_version?: string
  desired_state_updated_at?: ISODateTimeString
  applied_state_version?: string
  applied_state_updated_at?: ISODateTimeString
  observed_state?: ObservedState | string
  observed_message?: string
  observed_at?: ISODateTimeString
  state_convergence?: StateConvergence
  convergence_status?: StateConvergence
}
```

```ts
// frontend/src/types/policy.ts
import type { CommandId, ISODateTimeString, NodeId, PolicyId, TenantId } from './api'

export type PolicyKind = 'route' | 'acl' | 'qos' | 'blacklist' | 'unknown'
export type PolicyScope = 'node' | 'tenant'
export type DeliveryCommandStatus = 'pending' | 'sent' | 'acknowledged' | 'queued' | 'in_progress' | 'completed' | 'failed' | 'stale'
export type PolicyStatus = 'idle' | 'pending' | 'in_progress' | 'applied' | 'error' | 'stale'

export interface PolicyDelivery {
  id?: string
  tenant_id?: TenantId
  node_id?: NodeId
  policy_id?: PolicyId
  command_id?: CommandId
  action?: string
  command_status?: DeliveryCommandStatus | string
  last_error?: string
  created_at?: ISODateTimeString
  updated_at?: ISODateTimeString
}

export interface DispatchResult {
  desired_state_version?: string
  desired_state_updated_at?: ISODateTimeString
  command_id?: CommandId
  status?: DeliveryCommandStatus | string
  last_delivery?: PolicyDelivery | null
}
```

```ts
// frontend/src/types/monitoring.ts
import type { AlertId, ISODateTimeString, NodeId } from './api'
import type { PolicyDelivery, PolicyStatus } from './policy'

export type AlertStatus = 'active' | 'resolved'
export type AlertSeverity = 'info' | 'warning' | 'critical'
export type MonitoringRange = '1h' | '24h' | '7d' | '30d'

export interface MonitoringStats {
  total_nodes: number
  online_nodes: number
  offline_nodes: number
  sync_success_rate: number
  total_peers: number
  total_acl_rules: number
  total_qos_rules: number
  failed_commands_count: number
  active_alerts_count: number
}

export interface AlertRecord {
  id: AlertId
  node_id?: NodeId
  alert_type?: string
  severity: AlertSeverity | string
  status: AlertStatus | string
  message?: string
  created_at?: ISODateTimeString
  resolved_at?: ISODateTimeString
}

export interface MonitoringNodeDetail {
  id: NodeId
  hostname?: string
  desired_state_version?: string
  applied_state_version?: string
  observed_state?: PolicyStatus | string
  observed_message?: string
  last_sync_at?: ISODateTimeString
  last_sync_error?: string
  state_convergence?: string
  recent_policy_deliveries?: PolicyDelivery[]
  active_alerts?: AlertRecord[]
}
```

## Implementation Tasks

### Task 1: Add TypeScript Tooling Baseline

Status: completed in `codex/frontend-ts-baseline-b8`.

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Create: `frontend/tsconfig.json`
- Create: `frontend/tsconfig.node.json`
- Create: `frontend/src/env.d.ts`

- [ ] **Step 1: Install type checker**

Run:

```bash
cd frontend
npm install -D vue-tsc
```

Expected:

- `frontend/package.json` gains `vue-tsc` under `devDependencies`.
- `frontend/package-lock.json` is updated.

- [ ] **Step 2: Add package script**

Modify `frontend/package.json` scripts to include:

```json
{
  "type-check": "vue-tsc --noEmit -p tsconfig.json"
}
```

Expected scripts after change:

```json
{
  "dev": "vite",
  "build": "vite build",
  "preview": "vite preview",
  "test": "vitest",
  "test:run": "vitest run",
  "type-check": "vue-tsc --noEmit -p tsconfig.json"
}
```

- [ ] **Step 3: Create `frontend/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "allowJs": true,
    "checkJs": false,
    "skipLibCheck": true,
    "isolatedModules": true,
    "moduleResolution": "Bundler",
    "resolveJsonModule": true,
    "strict": true,
    "noFallthroughCasesInSwitch": true,
    "types": ["node", "vitest/globals"],
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"]
    }
  },
  "include": [
    "src/**/*.ts",
    "src/**/*.d.ts",
    "src/**/*.vue",
    "tests/**/*.ts",
    "vite.config.js",
    "vitest.config.js"
  ],
  "references": [
    {
      "path": "./tsconfig.node.json"
    }
  ]
}
```

- [ ] **Step 4: Create `frontend/tsconfig.node.json`**

```json
{
  "compilerOptions": {
    "composite": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "allowSyntheticDefaultImports": true,
    "types": ["node"]
  },
  "include": ["vite.config.js", "vitest.config.js"]
}
```

- [ ] **Step 5: Create `frontend/src/env.d.ts`**

```ts
/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'

  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}
```

- [ ] **Step 6: Verify baseline**

Run:

```bash
cd frontend
npm run type-check
npm run test:run
npm run build
```

Expected:

- `npm run type-check` passes before any source migration.
- `npm run test:run` keeps existing frontend test behavior.
- `npm run build` still emits `../temp-dist`.

- [ ] **Step 7: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/tsconfig.json frontend/tsconfig.node.json frontend/src/env.d.ts
git commit -m "chore: add frontend type-check baseline"
```

### Task 2: Add Shared Frontend Type Models

Status: completed in `codex/frontend-shared-types-b9`.

**Files:**
- Create: `frontend/src/types/api.ts`
- Create: `frontend/src/types/permission.ts`
- Create: `frontend/src/types/auth.ts`
- Create: `frontend/src/types/tenant.ts`
- Create: `frontend/src/types/node.ts`
- Create: `frontend/src/types/policy.ts`
- Create: `frontend/src/types/agent.ts`
- Create: `frontend/src/types/monitoring.ts`
- Create: `frontend/src/types/index.ts`

- [ ] **Step 1: Create core API and ID types**

Create `frontend/src/types/api.ts` using the `Type Model Baseline` content above.

- [ ] **Step 2: Create permission and auth types**

Create `frontend/src/types/permission.ts` using the `Permission` and `RoleName` union above.

Create `frontend/src/types/auth.ts`:

```ts
import type { Permission, RoleName } from './permission'
import type { TenantId } from './api'

export interface UserProfile {
  id?: string
  username: string
  name?: string
  role: RoleName | string
  tenant_id?: TenantId
  initials?: string
}

export interface LoginResponse {
  token: string
  expires_in?: number
  user?: UserProfile
  permissions?: Permission[]
  must_change_password?: boolean
}
```

- [ ] **Step 3: Create tenant types**

Create `frontend/src/types/tenant.ts`:

```ts
import type { TenantId } from './api'

export type TenantStatus = 'active' | 'disabled' | 'deleted' | string

export interface Tenant {
  id: TenantId
  name?: string
  status?: TenantStatus
  created_at?: string
  updated_at?: string
}
```

- [ ] **Step 4: Create node, policy, monitoring types**

Create `frontend/src/types/node.ts`, `frontend/src/types/policy.ts`, and `frontend/src/types/monitoring.ts` using the `Type Model Baseline` content above.

- [ ] **Step 5: Create agent command types**

Create `frontend/src/types/agent.ts`:

```ts
import type { CommandId, ISODateTimeString, NodeId, TenantId } from './api'

export type AgentCommandStatus = 'pending' | 'sent' | 'acknowledged' | 'queued' | 'in_progress' | 'completed' | 'failed' | 'stale'
export type AgentCommandType = 'sync' | 'health_check' | 'apply_policy' | 'reload' | string

export interface AgentCommandPayload {
  type?: AgentCommandType
  action?: string
  payload?: Record<string, unknown>
  reason?: string
}

export interface AgentCommandRecord {
  id?: CommandId
  command_id?: CommandId
  tenant_id?: TenantId
  node_id?: NodeId
  type?: AgentCommandType
  action?: string
  status?: AgentCommandStatus | string
  last_error?: string
  created_at?: ISODateTimeString
  updated_at?: ISODateTimeString
}

export interface AgentStatus {
  node_id?: NodeId
  availability_status?: string
  configuration_status?: string
  pending_cmds?: number
  desired_state_version?: string
  applied_state_version?: string
  observed_state?: string
  observed_message?: string
  last_sync_at?: ISODateTimeString
  last_sync_error?: string
}
```

- [ ] **Step 6: Create barrel export**

Create `frontend/src/types/index.ts`:

```ts
export * from './api'
export * from './permission'
export * from './auth'
export * from './tenant'
export * from './node'
export * from './policy'
export * from './agent'
export * from './monitoring'
```

- [ ] **Step 7: Verify types compile**

Run:

```bash
cd frontend
npm run type-check
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/types
git commit -m "chore: add frontend domain types"
```

### Task 3: Add Typed API Response Helpers

Status: completed in `codex/frontend-api-response-b10`.

**Files:**
- Create: `frontend/src/composables/apiResponse.ts`
- Test: `frontend/tests/unit/apiResponse.test.ts`

- [ ] **Step 1: Add response helper tests**

Create `frontend/tests/unit/apiResponse.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { unwrapApiData, unwrapApiList } from '@/composables/apiResponse'

describe('apiResponse helpers', () => {
  it('unwraps unified API envelope data', () => {
    expect(unwrapApiData({ data: { success: true, data: { id: 'n1' } } })).toEqual({ id: 'n1' })
  })

  it('returns raw response data when no envelope exists', () => {
    expect(unwrapApiData({ data: { id: 'raw' } })).toEqual({ id: 'raw' })
  })

  it('unwraps array list responses', () => {
    expect(unwrapApiList({ data: { success: true, data: [{ id: 'a' }] } })).toEqual([{ id: 'a' }])
  })

  it('unwraps items list responses', () => {
    expect(unwrapApiList({ data: { success: true, data: { items: [{ id: 'a' }] } } })).toEqual([{ id: 'a' }])
  })

  it('returns empty list for null list payloads', () => {
    expect(unwrapApiList({ data: { success: true, data: null } })).toEqual([])
  })
})
```

- [ ] **Step 2: Run failing test**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/apiResponse.test.ts
```

Expected: FAIL because `@/composables/apiResponse` does not exist.

- [ ] **Step 3: Create helper implementation**

Create `frontend/src/composables/apiResponse.ts`:

```ts
import type { ApiEnvelope } from '@/types'

interface ResponseLike<T> {
  data: ApiEnvelope<T> | T
}

function isEnvelope<T>(value: ApiEnvelope<T> | T): value is ApiEnvelope<T> {
  return Boolean(value && typeof value === 'object' && 'success' in value)
}

export function unwrapApiData<T>(response: ResponseLike<T>): T {
  const body = response.data
  if (isEnvelope<T>(body)) {
    return body.data as T
  }
  return body
}

export function unwrapApiList<T>(response: ResponseLike<T[] | { items?: T[] } | null | undefined>): T[] {
  const data = unwrapApiData<T[] | { items?: T[] } | null | undefined>(response)
  if (Array.isArray(data)) return data
  if (Array.isArray(data?.items)) return data.items
  return []
}
```

- [ ] **Step 4: Verify**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/apiResponse.test.ts
npm run type-check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/composables/apiResponse.ts frontend/tests/unit/apiResponse.test.ts
git commit -m "chore: add typed API response helpers"
```

### Task 4: Migrate Monitoring API to TypeScript

Status: completed in `codex/frontend-monitoring-api-b11`.

**Files:**
- Rename: `frontend/src/composables/useMonitorApi.js` -> `frontend/src/composables/useMonitorApi.ts`
- Modify: `frontend/tests/unit/monitoringWorkflow.test.js`

- [ ] **Step 1: Rename file**

```bash
git mv frontend/src/composables/useMonitorApi.js frontend/src/composables/useMonitorApi.ts
```

- [ ] **Step 2: Type monitoring API params and return values**

Use `MonitoringStats`, `MonitoringNodeDetail`, `AlertRecord`, `MonitoringRange`, and `ListResult` from `@/types`.

The function signatures should become:

```ts
getStats: async (): Promise<MonitoringStats>
getNodeDetail: async (nodeId: string): Promise<MonitoringNodeDetail>
getEvents: async (params: MonitoringEventParams = {}): Promise<ListResult<MonitoringEvent>>
getAlerts: async (params: MonitoringAlertParams = {}): Promise<ListResult<AlertRecord>>
resolveAlert: async (alertId: string): Promise<AlertRecord | { id?: string; status?: string }>
getTraffic: async (range: MonitoringRange = '24h'): Promise<MonitoringTraffic>
getHealth: async (): Promise<MonitoringHealth>
getNodeMetrics: async (nodeId: string): Promise<MonitoringNodeMetrics>
getTopology: async (): Promise<MonitoringTopology>
getVersion: async (): Promise<unknown>
```

- [ ] **Step 3: Keep runtime behavior unchanged**

For each API method, preserve:

```ts
const tenantId = requireCurrentTenantId()
const response = await api.get(...)
return unwrapApiData(response)
```

Do not change endpoint names, query params, or error messages in this task.

- [ ] **Step 4: Verify**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/monitoringWorkflow.test.js
npm run type-check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/composables/useMonitorApi.ts frontend/tests/unit/monitoringWorkflow.test.js
git commit -m "refactor: type monitoring API composable"
```

### Task 5: Migrate Agent Command API to TypeScript

Status: completed in `codex/frontend-agent-api-b12`.

**Files:**
- Rename: `frontend/src/composables/useAgentProxyApi.js` -> `frontend/src/composables/useAgentProxyApi.ts`
- Test: `frontend/tests/unit/useAgentProxyApi.test.js`

- [ ] **Step 1: Rename file**

```bash
git mv frontend/src/composables/useAgentProxyApi.js frontend/src/composables/useAgentProxyApi.ts
```

- [ ] **Step 2: Type command payloads**

Use `AgentCommandPayload`, `AgentCommandRecord`, and `AgentStatus` from `@/types`.

Target signatures:

```ts
sendAgentCommand: async (nodeId: string, command: AgentCommandPayload): Promise<AgentCommandRecord>
getAgentStatus: async (nodeId: string): Promise<AgentStatus>
getAgentCommands: async (nodeId: string, limit = 10): Promise<{ items?: AgentCommandRecord[] } | AgentCommandRecord[]>
sendBatchCommand: async (batchCommand: { node_ids?: string[]; command?: AgentCommandPayload; payload?: Record<string, unknown> }): Promise<unknown>
```

- [ ] **Step 3: Verify**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/useAgentProxyApi.test.js
npm run type-check
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/composables/useAgentProxyApi.ts
git commit -m "refactor: type agent proxy API composable"
```

### Task 6: Migrate Policy, ACL, and QoS API Boundaries

Status: completed in `codex/frontend-policy-api-b13`.

**Files:**
- Rename: `frontend/src/composables/usePolicyApi.js` -> `frontend/src/composables/usePolicyApi.ts`
- Rename: `frontend/src/composables/useAclApi.js` -> `frontend/src/composables/useAclApi.ts`
- Rename: `frontend/src/composables/useQosApi.js` -> `frontend/src/composables/useQosApi.ts`
- Test: `frontend/tests/unit/useAclApi.test.js`
- Test: `frontend/tests/unit/useQosApi.test.js`
- Test: `frontend/tests/unit/bandwidthControlEdit.test.js`

- [ ] **Step 1: Migrate `usePolicyApi` first**

Target signatures:

```ts
listPolicies: async (filters: { kind?: string; nodeId?: string; enabled?: boolean | string } = {}): Promise<NormalizedPolicy[]>
```

Create `NormalizedPolicy` in `frontend/src/types/policy.ts` with fields already returned by `normalizePolicy`, including:

```ts
export interface NormalizedPolicy {
  id: string
  policyId: string
  policyRef?: string
  tenantId?: string
  nodeId?: string
  nodeName?: string
  targetNodes: string[]
  scope: PolicyScope | string
  kind: PolicyKind | string
  name: string
  enabled: boolean
  priority: number
  version: string
  status: PolicyStatus | string
  desiredStateVersion: string
  appliedStateVersion: string
  observedState: string
  observedMessage: string
  stateConvergence: string
  spec: Record<string, unknown>
  pendingCmds: number
  lastDelivery: PolicyDelivery | null
  deliveryHistory: PolicyDelivery[]
}
```

- [ ] **Step 2: Migrate ACL normalizers**

Keep existing normalizer behavior and add explicit types for:

```ts
type ACLAction = 'allow' | 'deny'
type ACLDirection = 'ingress' | 'egress' | 'both'

interface ACLRulePayload {
  name?: string
  src_cidr?: string
  dst_cidr?: string
  src_group_id?: string
  dst_group_id?: string
  protocol?: number
  dst_port?: number
  ports?: string
  direction?: ACLDirection | string
  action?: ACLAction | string
  enabled?: boolean
  priority?: number
  description?: string
}
```

- [ ] **Step 3: Migrate QoS normalizers**

Keep the unified node-scoped QoS model. Do not reintroduce old `service / peers / ip` terminology.

Add:

```ts
type QoSDirection = 'ingress' | 'egress' | 'both'
type QoSMode = 'auto' | 'policing' | 'shaping'

interface QoSRulePayload {
  src_cidr?: string
  dst_cidr?: string
  group_id?: string
  group_cidr?: string
  bandwidth_mbps?: number
  direction?: QoSDirection | string
  rate_bps?: number
  burst_bytes?: number
  priority?: number
  mode?: QoSMode | string
  description?: string
  enabled?: boolean
}
```

- [ ] **Step 4: Verify targeted tests**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/useAclApi.test.js tests/unit/useQosApi.test.js tests/unit/bandwidthControlEdit.test.js
npm run type-check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/composables/usePolicyApi.ts frontend/src/composables/useAclApi.ts frontend/src/composables/useQosApi.ts frontend/src/types/policy.ts
git commit -m "refactor: type policy delivery API boundaries"
```

### Task 7: Migrate User, Tenant, and Node Stores

Status: completed in `codex/frontend-stores-b14`.

**Files:**
- Rename: `frontend/src/stores/user.js` -> `frontend/src/stores/user.ts`
- Rename: `frontend/src/stores/tenant.js` -> `frontend/src/stores/tenant.ts`
- Rename: `frontend/src/stores/node.js` -> `frontend/src/stores/node.ts`
- Test: `frontend/tests/unit/userSession.test.js`
- Test: `frontend/tests/unit/tenantStore.test.js`
- Test: `frontend/tests/unit/nodeStore.test.js`
- Test: `frontend/tests/unit/nodesWorkbench.test.js`

- [ ] **Step 1: Migrate user store**

Type these refs:

```ts
const user = ref<UserProfile | null>(null)
const isAuthenticated = ref(false)
const mustChangePassword = ref(false)
const permissions = ref<Permission[]>([])
```

Type these exported functions:

```ts
export const normalizeRoleName = (role: unknown): RoleName | string
export const permissionsForRole = (role: unknown): Permission[]
export const tokenRequiresPasswordChange = (token: string | null | undefined): boolean
```

- [ ] **Step 2: Migrate tenant store**

Type these refs:

```ts
const currentTenant = ref<Tenant | null>(null)
const tenants = ref<Tenant[]>([])
const loading = ref(false)
```

Type function signatures:

```ts
async function loadTenants(): Promise<void>
async function switchTenant(tenant: Tenant | null): Promise<void>
```

- [ ] **Step 3: Migrate node store**

Split raw and UI models:

```ts
interface NodeViewModel {
  id: string
  hostname: string
  ip: string
  publicIp: string
  vpnIp: string
  endpoint: string
  region: string
  status: string
  rawStatus: string
  version: string
  mode: string
  lastSeen: string
  uptime: string
  routes: string[]
  pendingCmds: number
  configurationStatus: string
  lastSyncAt: string
  desiredStateVersion: string
  appliedStateVersion: string
  observedState: string
  observedMessage: string
  stateConvergence: string
  recentCommands: unknown[]
  bandwidth: { upload: number; download: number }
  latency: number
}
```

Type these refs:

```ts
const nodes = ref<NodeViewModel[]>([])
const currentNode = ref<NodeViewModel | null>(null)
const loading = ref(false)
```

- [ ] **Step 4: Verify store tests**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/userSession.test.js tests/unit/tenantStore.test.js tests/unit/nodeStore.test.js tests/unit/nodesWorkbench.test.js
npm run type-check
```

Expected: PASS. If `nodesWorkbench.test.js` exposes the known `sent` vs `acknowledged` baseline mismatch, do not hide it with loose types; either fix the expectation as a separate behavior commit or run the remaining store tests and record the known failure in the PR.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/stores/user.ts frontend/src/stores/tenant.ts frontend/src/stores/node.ts
git commit -m "refactor: type core frontend stores"
```

### Task 8: Migrate Low-Risk Utilities and Config

**Files:**
- Rename: `frontend/src/config/api.js` -> `frontend/src/config/api.ts`
- Rename: `frontend/src/utils/session.js` -> `frontend/src/utils/session.ts`
- Rename: `frontend/src/utils/topologyLayout.js` -> `frontend/src/utils/topologyLayout.ts`
- Test: `frontend/tests/unit/useApiSession.test.js`
- Test: `frontend/tests/unit/topologyLayout.test.js`

- [ ] **Step 1: Type API config helpers**

Target signatures:

```ts
export const getCurrentTenant = (): { id?: string; [key: string]: unknown } | null
export const getCurrentTenantId = (): string | null
export const requireCurrentTenantId = (): string
export const buildTenantPath = (tenantId: string, suffix?: string): string
```

- [ ] **Step 2: Type session helpers**

Preserve localStorage keys:

- `aria_token`
- `aria_token_expire_time`
- `aria-current-tenant`
- `aria_last_activity`

Do not rename keys during TypeScript migration.

Status: completed in `codex/frontend-utils-b15`.

- [ ] **Step 3: Type topology layout**

Use explicit node/link geometry models:

```ts
interface TopologyNode {
  id: string
  x?: number
  y?: number
  [key: string]: unknown
}

interface TopologyLink {
  source: string
  target: string
  [key: string]: unknown
}
```

- [ ] **Step 4: Verify**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/useApiSession.test.js tests/unit/topologyLayout.test.js
npm run type-check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/config/api.ts frontend/src/utils/session.ts frontend/src/utils/topologyLayout.ts
git commit -m "refactor: type frontend config and utilities"
```

### Task 9: Migrate Selected Vue Pages Last

**Files:**
- Modify: `frontend/src/views/Monitoring.vue`
- Modify: `frontend/src/views/NodeMonitorDetail.vue`
- Modify: `frontend/src/views/Nodes.vue`
- Modify: `frontend/src/views/Policies.vue`
- Modify: `frontend/src/views/ACLRules.vue`
- Modify: `frontend/src/views/BandwidthControl.vue`

- [ ] **Step 1: Migrate monitoring pages**

Convert only the script blocks:

```vue
<script setup lang="ts">
```

Use imported types from `@/types` for refs and computed data. Keep template markup unchanged unless TypeScript exposes a real null-safety issue.

- [ ] **Step 2: Migrate node and policy pages**

Apply the same rule: script-only migration, no visual redesign, no table column rewrite.

- [ ] **Step 3: Verify page-focused tests**

Run:

```bash
cd frontend
npm run test:run -- tests/unit/monitoringWorkflow.test.js tests/unit/nodesWorkbench.test.js tests/unit/bandwidthControlEdit.test.js
npm run type-check
npm run build
```

Expected:

- Monitoring and bandwidth tests pass.
- `nodesWorkbench.test.js` either passes or shows the known baseline mismatch, which must be handled as a separate explicit behavior fix.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/Monitoring.vue frontend/src/views/NodeMonitorDetail.vue frontend/src/views/Nodes.vue frontend/src/views/Policies.vue frontend/src/views/ACLRules.vue frontend/src/views/BandwidthControl.vue
git commit -m "refactor: type high-risk frontend views"
```

### Task 10: Add Type Check to CI

**Files:**
- Modify: `.github/workflows/build.yml`

- [ ] **Step 1: Add CI type-check step**

In `frontend-build`, insert after `npm ci` and before unit tests:

```yaml
      - name: Run frontend type check
        run: npm run type-check
```

Target order:

```yaml
      - name: Install frontend dependencies
        run: npm ci

      - name: Run frontend type check
        run: npm run type-check

      - name: Run frontend unit tests
        run: npm run test:run

      - name: Build frontend
        run: npm run build
```

- [ ] **Step 2: Verify locally**

Run:

```bash
cd frontend
npm run type-check
npm run test:run
npm run build
```

Expected: PASS before pushing.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/build.yml
git commit -m "ci: run frontend type check"
```

## Rollout Strategy

推荐按 4 个 PR 执行，而不是一个大 PR：

1. **PR 1: Type-check baseline**
   - Task 1, Task 2, Task 3
   - 目标：不改业务行为，只让 TS 基础设施和核心类型落地。

2. **PR 2: API composables**
   - Task 4, Task 5, Task 6
   - 目标：监控、Agent 命令、策略投递、ACL、QoS 这些闭环相关接口先类型化。

3. **PR 3: Stores and utilities**
   - Task 7, Task 8
   - 目标：用户、权限、租户、节点状态和工具函数类型化。

4. **PR 4: High-risk views and CI gate**
   - Task 9, Task 10
   - 目标：只迁移高风险页面脚本，并把 type-check 固化进 CI。

## Validation Matrix

每个 PR 至少运行：

```bash
cd frontend
npm run type-check
npm run test:run
npm run build
```

涉及监控时加跑：

```bash
cd frontend
npm run test:run -- tests/unit/monitoringWorkflow.test.js
```

涉及节点时加跑：

```bash
cd frontend
npm run test:run -- tests/unit/nodeStore.test.js tests/unit/nodesWorkbench.test.js
```

涉及 ACL/QoS/策略时加跑：

```bash
cd frontend
npm run test:run -- tests/unit/useAclApi.test.js tests/unit/useQosApi.test.js tests/unit/bandwidthControlEdit.test.js
```

## Acceptance Criteria

- `frontend` 支持 `npm run type-check`。
- CI 的 `frontend-build` job 执行 type-check、unit tests、build。
- 监控 API、Agent 命令 API、策略投递 API、ACL API、QoS API 均有明确入参和返回类型。
- 用户、租户、节点 Pinia store 不再依赖隐式 `any` 状态。
- 核心状态字符串由 union type 表达：
  - 节点在线状态。
  - 控制投递状态。
  - 策略回显状态。
  - 告警状态。
  - 权限和角色。
- 不改变现有用户可见行为。
- 不引入框架迁移、UI 重写或依赖大升级。

## Risks and Controls

- **风险：一次性改太多页面。**
  控制：页面迁移放到最后，只迁移闭环相关页面的 script。

- **风险：后端字段实际不稳定。**
  控制：先定义原始 DTO，再定义 normalized view model，避免页面直接依赖后端临时字段。

- **风险：为了过 type-check 滥用 `any`。**
  控制：仅允许局部 `unknown`，必须在 normalizer 内收窄。

- **风险：类型迁移顺手改业务逻辑。**
  控制：每个任务只允许类型、文件扩展名、helper 抽取和必要 null-safety 修复；业务行为修复单独提交。

- **风险：CI 过早启用导致开发阻塞。**
  控制：先在 PR 1 本地通过 `npm run type-check`，PR 4 再接入 GitHub Actions。
