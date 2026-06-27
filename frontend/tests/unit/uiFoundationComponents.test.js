import { describe, expect, it, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

import PageHeader from '@/components/ui/PageHeader.vue'
import MetricStrip from '@/components/ui/MetricStrip.vue'
import DataPanel from '@/components/ui/DataPanel.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ActionIconButton from '@/components/ui/ActionIconButton.vue'
import FilterBar from '@/components/ui/FilterBar.vue'

describe('UI foundation components', () => {
  beforeEach(() => {
    localStorage.setItem('aria-lang', 'en')
  })

  it('renders a compact page header with translated actions outside cards', () => {
    const wrapper = mount(PageHeader, {
      props: {
        title: 'Nodes',
        subtitle: 'Operate node runtime and sync state'
      },
      slots: {
        actions: '<button class="refresh-action">Refresh</button>'
      }
    })

    expect(wrapper.find('.ui-page-header').exists()).toBe(true)
    expect(wrapper.find('h1').text()).toBe('Nodes')
    expect(wrapper.text()).toContain('Operate node runtime and sync state')
    expect(wrapper.find('.ui-page-header__actions .refresh-action').exists()).toBe(true)
  })

  it('renders dense operational metrics without decorative card chrome', () => {
    const wrapper = mount(MetricStrip, {
      props: {
        metrics: [
          { label: 'Online', value: 3, meta: 'of 4 nodes', status: 'success' },
          { label: 'Failed', value: 1, meta: 'needs attention', status: 'danger' }
        ]
      }
    })

    const items = wrapper.findAll('.ui-metric-strip__item')
    expect(items).toHaveLength(2)
    expect(items[0].text()).toContain('Online')
    expect(items[0].text()).toContain('3')
    expect(items[0].classes()).toContain('ui-metric-strip__item--success')
    expect(items[1].classes()).toContain('ui-metric-strip__item--danger')
    expect(wrapper.find('.stat-icon').exists()).toBe(false)
  })

  it('wraps table and filter sections in a neutral data panel', () => {
    const wrapper = mount(DataPanel, {
      props: {
        title: 'Policy delivery',
        subtitle: 'Latest sync evidence'
      },
      slots: {
        actions: '<button class="retry-action">Retry</button>',
        default: '<table><tbody><tr><td>applied</td></tr></tbody></table>'
      }
    })

    expect(wrapper.find('.ui-data-panel').exists()).toBe(true)
    expect(wrapper.find('.ui-data-panel__title').text()).toBe('Policy delivery')
    expect(wrapper.find('.ui-data-panel__actions .retry-action').exists()).toBe(true)
    expect(wrapper.find('table').text()).toBe('applied')
  })

  it('translates status labels and applies semantic state classes', () => {
    const queued = mount(StatusBadge, {
      props: {
        domain: 'command',
        status: 'queued'
      }
    })
    expect(queued.text()).toBe('Queued')
    expect(queued.classes()).toContain('ui-status-badge--info')

    const failed = mount(StatusBadge, {
      props: {
        domain: 'policy',
        status: 'failed'
      }
    })
    expect(failed.text()).toBe('Failed')
    expect(failed.classes()).toContain('ui-status-badge--danger')
  })

  it('renders accessible table action buttons with neutral and danger states', async () => {
    const wrapper = mount(ActionIconButton, {
      props: {
        label: 'Delete rule',
        tone: 'danger'
      },
      slots: {
        default: 'D'
      }
    })

    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('click')).toHaveLength(1)
    expect(wrapper.find('button').attributes('aria-label')).toBe('Delete rule')
    expect(wrapper.find('button').classes()).toContain('ui-action-icon-button--danger')

    await wrapper.setProps({ disabled: true })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('click')).toHaveLength(1)
    expect(wrapper.find('button').attributes()).toHaveProperty('disabled')
  })

  it('keeps filters and actions in stable left and right slots', () => {
    const wrapper = mount(FilterBar, {
      slots: {
        filters: '<input class="node-filter" />',
        actions: '<button class="add-action">Add</button>'
      }
    })

    expect(wrapper.find('.ui-filter-bar__filters .node-filter').exists()).toBe(true)
    expect(wrapper.find('.ui-filter-bar__actions .add-action').exists()).toBe(true)
  })
})
