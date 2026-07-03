import { describe, expect, it } from 'vitest'

import {
  nodeDetailRouteFromContext,
  normalizeDeliveryEvidence,
  policyCenterQueryFromContext,
  policyContextFromQuery,
  policyPageRouteForDomain,
  policyRowMatchesContext
} from '@/utils/policyContext'

describe('policyContext utilities', () => {
  it('normalizes snake_case and camelCase route query aliases', () => {
    expect(policyContextFromQuery({
      node_id: 'node-1',
      rule_id: 'acl-1',
      policy_domain: 'acl',
      command_id: 'cmd-1'
    })).toEqual({
      nodeId: 'node-1',
      policyRef: 'acl-1',
      policyDomain: 'acl',
      commandId: 'cmd-1'
    })

    expect(policyContextFromQuery({
      nodeId: 'node-2',
      policyRef: 'qos-1',
      kind: 'qos',
      commandId: 'cmd-2'
    })).toEqual({
      nodeId: 'node-2',
      policyRef: 'qos-1',
      policyDomain: 'qos',
      commandId: 'cmd-2'
    })
  })

  it('builds stable policy center and special page routes from one context shape', () => {
    const context = {
      nodeId: 'node-1',
      policyRef: 'office-acl',
      policyDomain: 'acl',
      commandId: 'cmd-1'
    }

    expect(policyCenterQueryFromContext(context)).toEqual({
      nodeId: 'node-1',
      policyRef: 'office-acl',
      kind: 'acl',
      commandId: 'cmd-1'
    })

    expect(policyPageRouteForDomain('acl', context)).toEqual({
      name: 'ACLRules',
      query: {
        nodeId: 'node-1',
        policyRef: 'office-acl',
        commandId: 'cmd-1'
      }
    })
    expect(policyPageRouteForDomain('qos', { ...context, policyDomain: 'qos' }).name).toBe('BandwidthControl')
    expect(policyPageRouteForDomain('route', { ...context, policyDomain: 'route' }).name).toBe('Routing')
  })

  it('builds node detail routes with command-first focus', () => {
    expect(nodeDetailRouteFromContext({
      nodeId: 'node-1',
      policyRef: 'acl-1',
      policyDomain: 'acl',
      commandId: 'cmd-1'
    })).toEqual({
      name: 'NodeMonitorDetail',
      params: { nodeId: 'node-1' },
      query: {
        focus: 'commands',
        commandId: 'cmd-1',
        policyRef: 'acl-1',
        policyDomain: 'acl'
      }
    })

    expect(nodeDetailRouteFromContext({
      nodeId: 'node-1',
      policyRef: 'acl-1',
      policyDomain: 'acl'
    })?.query).toEqual({
      focus: 'policies',
      policyRef: 'acl-1',
      policyDomain: 'acl'
    })

    expect(nodeDetailRouteFromContext({ policyRef: 'acl-1' })).toBeNull()
  })

  it('matches policy rows by policy reference before command id fallback', () => {
    const row = {
      id: 'row-id',
      policy_ref: 'acl-1',
      name: 'office rule',
      last_delivery_command_id: 'cmd-1'
    }

    expect(policyRowMatchesContext(row, { policyRef: 'acl-1', commandId: 'wrong-cmd' })).toBe(true)
    expect(policyRowMatchesContext(row, { commandId: 'cmd-1' })).toBe(true)
    expect(policyRowMatchesContext(row, { policyRef: 'missing' })).toBe(false)
  })

  it('normalizes latest delivery evidence from command_status aliases', () => {
    expect(normalizeDeliveryEvidence({
      command_status: 'failed',
      command_id: 'cmd-1',
      action: 'sync',
      last_error: 'apply failed',
      updated_at: '2026-07-03T10:00:00Z'
    })).toEqual({
      status: 'failed',
      commandId: 'cmd-1',
      action: 'sync',
      lastError: 'apply failed',
      updatedAt: '2026-07-03T10:00:00Z'
    })
  })
})
