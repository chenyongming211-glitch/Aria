type QueryValue = string | number | boolean | null | undefined | Array<string | null>
export type RouteQueryLike = Record<string, QueryValue>

export interface PolicyContext {
  nodeId?: string
  policyRef?: string
  policyDomain?: string
  commandId?: string
}

export interface DeliveryEvidence {
  status: string
  commandId: string
  action: string
  lastError: string
  updatedAt: string
}

export interface PolicyRouteTarget {
  name: 'ACLRules' | 'BandwidthControl' | 'Routing' | 'Policies' | 'NodeMonitorDetail' | 'IPGroups'
  params?: Record<string, string>
  query?: Record<string, string>
}

type PolicyRowLike = Record<string, any>

const routeNameByDomain: Record<string, PolicyRouteTarget['name']> = {
  acl: 'ACLRules',
  qos: 'BandwidthControl',
  route: 'Routing'
}

const firstString = (query: RouteQueryLike, keys: string[]): string => {
  for (const key of keys) {
    const raw = query[key]
    const value = Array.isArray(raw) ? raw[0] : raw
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  }
  return ''
}

const clean = (value: unknown): string => String(value || '').trim()

const lower = (value: unknown): string => clean(value).toLowerCase()

export const hasPolicyContext = (context: PolicyContext = {}): boolean =>
  Boolean(context.nodeId || context.policyRef || context.policyDomain || context.commandId)

export const policyContextFromQuery = (query: RouteQueryLike = {}): PolicyContext => ({
  nodeId: firstString(query, ['nodeId', 'node_id']),
  policyRef: firstString(query, ['ruleId', 'rule_id', 'policyRef', 'policy_ref']),
  policyDomain: firstString(query, ['kind', 'policyDomain', 'policy_domain']).toLowerCase(),
  commandId: firstString(query, ['commandId', 'command_id'])
})

export const policyCenterQueryFromContext = (context: PolicyContext = {}): Record<string, string> => ({
  ...(context.nodeId ? { nodeId: context.nodeId } : {}),
  ...(context.policyRef ? { policyRef: context.policyRef } : {}),
  ...(context.policyDomain ? { kind: context.policyDomain } : {}),
  ...(context.commandId ? { commandId: context.commandId } : {})
})

export const policySpecialPageQueryFromContext = (context: PolicyContext = {}): Record<string, string> => ({
  ...(context.nodeId ? { nodeId: context.nodeId } : {}),
  ...(context.policyRef ? { policyRef: context.policyRef } : {}),
  ...(context.commandId ? { commandId: context.commandId } : {})
})

export const policyPageRouteForDomain = (
  domain: string | undefined,
  context: PolicyContext = {}
): PolicyRouteTarget => ({
  name: routeNameByDomain[lower(domain)] || 'Policies',
  query: lower(domain) in routeNameByDomain
    ? policySpecialPageQueryFromContext(context)
    : policyCenterQueryFromContext(context)
})

export const policyCenterRouteFromContext = (context: PolicyContext = {}): PolicyRouteTarget => ({
  name: 'Policies',
  query: policyCenterQueryFromContext(context)
})

export const ipGroupsRouteFromContext = (context: PolicyContext = {}): PolicyRouteTarget => ({
  name: 'IPGroups',
  query: policyCenterQueryFromContext(context)
})

export const nodeDetailFocusFromContext = (context: PolicyContext = {}): string => {
  if (context.commandId) return 'commands'
  if (context.policyRef || context.policyDomain) return 'policies'
  return ''
}

export const nodeDetailRouteFromContext = (context: PolicyContext = {}): PolicyRouteTarget | null => {
  if (!context.nodeId) return null
  const focus = nodeDetailFocusFromContext(context)
  return {
    name: 'NodeMonitorDetail',
    params: { nodeId: context.nodeId },
    query: {
      ...(focus ? { focus } : {}),
      ...(context.commandId ? { commandId: context.commandId } : {}),
      ...(context.policyRef ? { policyRef: context.policyRef } : {}),
      ...(context.policyDomain ? { policyDomain: context.policyDomain } : {})
    }
  }
}

export const policyRefFromAnyRow = (row: PolicyRowLike = {}): string => (
  clean(row.policyRef) ||
  clean(row.policy_ref) ||
  clean(row.rule_id) ||
  clean(row.id) ||
  clean(row.cidr)
)

export const commandIdFromAnyRow = (row: PolicyRowLike = {}): string => (
  clean(row.lastDeliveryCommandId) ||
  clean(row.last_delivery_command_id) ||
  clean(row.commandId) ||
  clean(row.command_id) ||
  clean(row.lastDelivery?.command_id) ||
  clean(row.last_delivery?.command_id) ||
  clean(row.deliveryHistory?.find?.((item: PolicyRowLike) => item?.command_id || item?.commandId)?.command_id) ||
  clean(row.deliveryHistory?.find?.((item: PolicyRowLike) => item?.command_id || item?.commandId)?.commandId) ||
  clean(row.delivery_history?.find?.((item: PolicyRowLike) => item?.command_id || item?.commandId)?.command_id) ||
  clean(row.delivery_history?.find?.((item: PolicyRowLike) => item?.command_id || item?.commandId)?.commandId)
)

export const policyContextFromRow = (
  row: PolicyRowLike = {},
  defaults: PolicyContext = {}
): PolicyContext => ({
  nodeId: clean(row.nodeId) || clean(row.node_id) || defaults.nodeId,
  policyRef: policyRefFromAnyRow(row) || defaults.policyRef,
  policyDomain: lower(row.kind || row.policyDomain || row.policy_domain || defaults.policyDomain),
  commandId: commandIdFromAnyRow(row) || defaults.commandId
})

export const policyRowMatchesContext = (row: PolicyRowLike = {}, context: PolicyContext = {}): boolean => {
  const policyRef = lower(context.policyRef)
  const commandId = lower(context.commandId)
  if (!policyRef && !commandId) return true

  const policyHaystack = [
    row.policyRef,
    row.policy_ref,
    row.rule_id,
    row.id,
    row.name,
    row.description,
    row.cidr,
    row.group_id,
    row.group_cidr,
    row.runtime_group
  ].map(lower).filter(Boolean)
  if (policyRef) return policyHaystack.includes(policyRef)

  const commandHaystack = [
    row.lastDeliveryCommandId,
    row.last_delivery_command_id,
    row.commandId,
    row.command_id,
    row.lastDelivery?.command_id,
    row.last_delivery?.command_id
  ].map(lower).filter(Boolean)
  return commandId ? commandHaystack.includes(commandId) : true
}

export const normalizeDeliveryEvidence = (delivery: PolicyRowLike = {}): DeliveryEvidence => ({
  status: clean(delivery.status) || clean(delivery.command_status),
  commandId: clean(delivery.commandId) || clean(delivery.command_id),
  action: clean(delivery.action),
  lastError: clean(delivery.lastError) || clean(delivery.last_error),
  updatedAt: clean(delivery.updatedAt) || clean(delivery.updated_at) || clean(delivery.created_at)
})
