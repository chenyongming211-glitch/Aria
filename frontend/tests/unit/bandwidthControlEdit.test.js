import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

const {
  qosApiMock,
  tenantApiMock,
  ipGroupApiMock
} = vi.hoisted(() => ({
  qosApiMock: {
    getQoSRulesByNode: vi.fn(async () => [
      {
        id: 'qos-1',
        node_id: 'node-1',
        description: 'limit vpn peer',
        group_id: 'group-1',
        bandwidth_mbps: 1,
        direction: 'egress',
        rate_bps: 1000000,
        burst_bytes: 1500,
        priority: 10,
        mode: 'policing',
        enabled: true,
        policyStatus: 'applied',
        stats: {}
      }
    ]),
    createQoSRule: vi.fn(async () => ({})),
    updateQoSRule: vi.fn(async () => ({})),
    deleteQoSRule: vi.fn(async () => ({}))
  },
  tenantApiMock: {
    getTenantNodes: vi.fn(async () => [{ id: 'node-1', hostname: 'edge-1' }])
  },
  ipGroupApiMock: {
    listIPGroups: vi.fn(async () => [
      { id: 'group-1', name: 'vpn-peer', kind: 'custom', members: [{ cidr: '100.64.0.27/32' }] }
    ])
  }
}))

vi.mock('@/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (permission) => permission === 'qos:write'
  })
}))

vi.mock('@/composables/useTenantChangeReload', () => ({
  useTenantChangeReload: vi.fn()
}))

vi.mock('@/composables/useQosApi', () => ({
  useQosApi: qosApiMock
}))

vi.mock('@/composables/useTenantApi', () => ({
  useTenantApi: tenantApiMock
}))

vi.mock('@/composables/useIpGroupApi', () => ({
  useIpGroupApi: ipGroupApiMock
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    error: vi.fn()
  },
  ElMessageBox: {
    confirm: vi.fn(async () => {})
  }
}))

import BandwidthControl from '@/views/BandwidthControl.vue'

const stubs = {
  'el-card': { template: '<div><slot name="header" /><slot /></div>' },
  'el-select': {
    props: ['modelValue'],
    emits: ['update:modelValue', 'change'],
    template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\', $event.target.value)"><slot /></select>'
  },
  'el-option': {
    props: ['label', 'value'],
    template: '<option :value="value">{{ label }}</option>'
  },
  'el-button': {
    props: ['disabled', 'loading'],
    template: '<button :disabled="disabled || loading" @click="$emit(\'click\', $event)"><slot /></button>'
  },
  'el-icon': { template: '<i><slot /></i>' },
  'el-alert': { template: '<div></div>' },
  'el-table': {
    props: ['data'],
    template: '<div><slot /></div>'
  },
  'el-table-column': {
    props: ['label'],
    template: '<div></div>'
  },
  'el-tag': { template: '<span><slot /></span>' },
  'el-dialog': { template: '<div><slot /><slot name="footer" /></div>' },
  'el-form': {
    template: '<form><slot /></form>',
    methods: {
      validate: () => Promise.resolve(true),
      resetFields: () => {}
    }
  },
  'el-form-item': { template: '<div><slot /></div>' },
  'el-input': {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
  },
  'el-input-number': {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<input type="number" :value="modelValue" @input="$emit(\'update:modelValue\', Number($event.target.value))" />'
  },
  'el-row': { template: '<div><slot /></div>' },
  'el-col': { template: '<div><slot /></div>' }
}

describe('BandwidthControl edit behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('updates an existing QoS rule instead of creating a new one', async () => {
    const wrapper = mount(BandwidthControl, {
      global: {
        stubs,
        directives: {
          loading: {}
        }
      }
    })

    await flushPromises()

    const handleEdit = wrapper.vm.handleEdit || wrapper.vm.$.setupState.handleEdit
    const handleSave = wrapper.vm.handleSave || wrapper.vm.$.setupState.handleSave
    const form = wrapper.vm.form || wrapper.vm.$.setupState.form

    expect(handleEdit).toBeTypeOf('function')
    handleEdit({
      id: 'qos-1',
      description: 'limit vpn peer',
      group_id: 'group-1',
      bandwidth_mbps: 1,
      direction: 'egress',
      rate_bps: 1000000,
      burst_bytes: 1500,
      priority: 10,
      mode: 'policing',
      enabled: true
    })
    await nextTick()

    form.bandwidth_mbps = 2
    await handleSave()
    await flushPromises()

    expect(qosApiMock.updateQoSRule).toHaveBeenCalledWith('node-1', 'qos-1', expect.objectContaining({
      id: 'qos-1',
      bandwidth_mbps: 2,
      rate_bps: 0,
      burst_bytes: 0
    }))
    expect(qosApiMock.createQoSRule).not.toHaveBeenCalled()
  })
})
