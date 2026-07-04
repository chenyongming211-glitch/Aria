import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount, flushPromises } from '@vue/test-utils'

const {
  routerPush,
  ipGroupApiMock,
  elMessageError
} = vi.hoisted(() => ({
  routerPush: vi.fn(),
  ipGroupApiMock: {
    listIPGroups: vi.fn(),
    listIPGroupReferences: vi.fn(),
    createIPGroup: vi.fn(),
    updateIPGroup: vi.fn(),
    deleteIPGroup: vi.fn()
  },
  elMessageError: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: routerPush })
}))

vi.mock('@/composables/useIpGroupApi', () => ({
  useIpGroupApi: ipGroupApiMock
}))

vi.mock('@/composables/usePermission', () => ({
  usePermission: () => ({
    hasPermission: () => true
  })
}))

vi.mock('@/composables/useTenantChangeReload', () => ({
  useTenantChangeReload: vi.fn()
}))

vi.mock('@/i18n', () => ({
  t: (key) => key
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    error: elMessageError,
    success: vi.fn(),
    warning: vi.fn()
  },
  ElMessageBox: {
    confirm: vi.fn(async () => true)
  }
}))

import IPGroups from '@/views/IPGroups.vue'

const passthroughStub = {
  template: '<div><slot name="header" /><slot name="reference" /><slot name="footer" /><slot /></div>'
}

const mountIPGroups = () => shallowMount(IPGroups, {
  global: {
    directives: {
      loading: {}
    },
    stubs: {
      PolicyContextBanner: true,
      'el-table': passthroughStub,
      'el-table-column': {
        template: '<div><slot :row="row" /></div>',
        data() {
          return {
            row: {
              id: 'group-1',
              name: 'office',
              kind: 'custom',
              description: '',
              members: [{ cidr: '10.10.0.0/16' }],
              warnings: []
            }
          }
        }
      },
      'el-button': passthroughStub,
      'el-icon': passthroughStub,
      'el-tag': passthroughStub,
      'el-dialog': passthroughStub,
      'el-form': passthroughStub,
      'el-form-item': passthroughStub,
      'el-input': { template: '<input />' },
      'el-drawer': passthroughStub,
      'el-alert': passthroughStub,
      'el-pagination': true,
      Delete: true,
      Plus: true,
      Refresh: true
    }
  }
})

describe('IPGroups view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('uses a fallback error message when loading groups rejects without an Error object', async () => {
    ipGroupApiMock.listIPGroups.mockRejectedValueOnce(null)

    mountIPGroups()
    await flushPromises()

    expect(elMessageError).toHaveBeenCalledWith('ipGroups.loadFailed: policyTerms.unknownError')
  })

  it('uses a fallback error message when saving groups rejects without an Error object', async () => {
    ipGroupApiMock.listIPGroups.mockResolvedValueOnce([])
    ipGroupApiMock.createIPGroup.mockRejectedValueOnce(null)
    const wrapper = mountIPGroups()
    await flushPromises()

    wrapper.vm.formRef = { validate: vi.fn(async () => true), resetFields: vi.fn() }
    wrapper.vm.form.name = 'office'
    wrapper.vm.form.members = [{ cidr: '10.10.0.0/16', note: '' }]

    await wrapper.vm.handleSubmit()

    expect(elMessageError).toHaveBeenCalledWith('ipGroups.saveFailed: policyTerms.unknownError')
  })
})
