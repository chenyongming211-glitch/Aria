import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount, flushPromises } from '@vue/test-utils'

const {
  tokenApiMock,
  elMessageError
} = vi.hoisted(() => ({
  tokenApiMock: {
    getAllTokens: vi.fn(),
    getTokenDetail: vi.fn(),
    createToken: vi.fn(),
    revokeToken: vi.fn()
  },
  elMessageError: vi.fn()
}))

vi.mock('@/composables/useTokenApi', () => ({
  useTokenApi: tokenApiMock
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

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: {
      error: elMessageError,
      success: vi.fn(),
      warning: vi.fn()
    },
    ElNotification: vi.fn()
  }
})

import Tokens from '@/views/Tokens.vue'

const passthroughStub = {
  template: '<div><slot name="header" /><slot name="reference" /><slot name="footer" /><slot /></div>'
}

const mountTokens = () => shallowMount(Tokens, {
  global: {
    directives: {
      loading: {}
    },
    stubs: {
      'el-card': passthroughStub,
      'el-table': passthroughStub,
      'el-table-column': { template: '<div />' },
      'el-button': passthroughStub,
      'el-icon': passthroughStub,
      'el-input': { template: '<input />' },
      'el-dialog': passthroughStub,
      'el-form': passthroughStub,
      'el-form-item': passthroughStub,
      'el-input-number': { template: '<input />' },
      'el-select': passthroughStub,
      'el-option': true,
      'el-pagination': true,
      'el-popconfirm': passthroughStub,
      'el-tag': passthroughStub,
      Search: true,
      Plus: true,
      Refresh: true
    }
  }
})

describe('Tokens view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps existing token rows and shows an error when reload fails', async () => {
    tokenApiMock.getAllTokens.mockResolvedValueOnce([
      { id: 'token-1', token_preview: 'tk_1234', tag: 'initial', status: 'active' }
    ])
    const wrapper = mountTokens()
    await flushPromises()
    expect(wrapper.vm.tokens).toHaveLength(1)

    tokenApiMock.getAllTokens.mockRejectedValueOnce(new Error('network down'))
    await wrapper.vm.fetchTokens()

    expect(wrapper.vm.tokens).toHaveLength(1)
    expect(wrapper.vm.tokens[0].id).toBe('token-1')
    expect(elMessageError).toHaveBeenCalled()
  })

  it('uses the shared error helper when token creation rejects without an Error object', async () => {
    tokenApiMock.getAllTokens.mockResolvedValueOnce([])
    tokenApiMock.createToken.mockRejectedValueOnce(null)
    const wrapper = mountTokens()
    await flushPromises()

    wrapper.vm.createToken()
    wrapper.vm.currentToken.tag = 'node-onboarding'

    await wrapper.vm.saveToken()

    expect(elMessageError).toHaveBeenCalledWith('tokens.createFailed: policyTerms.unknownError')
  })

  it('uses the shared error helper when token revocation rejects without an Error object', async () => {
    tokenApiMock.getAllTokens.mockResolvedValueOnce([
      { id: 'token-1', token_preview: 'tk_1234', tag: 'initial', status: 'active' }
    ])
    tokenApiMock.revokeToken.mockRejectedValueOnce(null)
    const wrapper = mountTokens()
    await flushPromises()

    await wrapper.vm.revokeToken('token-1')

    expect(elMessageError).toHaveBeenCalledWith('tokens.revokeFailed: policyTerms.unknownError')
  })
})
