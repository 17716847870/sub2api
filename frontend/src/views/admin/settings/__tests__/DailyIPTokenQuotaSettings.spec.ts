import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import component from '../DailyIPTokenQuotaSettings.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const { ref } = await import('vue')
  const { default: messages } = await import('@/i18n/locales/en/admin/ipTokenQuota')
  const original = await importOriginal<typeof import('vue-i18n')>()
  return { ...original, useI18n: () => ({ locale: ref('en'), t: (key: string, params: Record<string, unknown> = {}) => {
    let value: unknown = { admin: messages, common: { loading: 'Loading', tryAgain: 'Retry', submitting: 'Saving', search: 'Search', status: 'Status' } }
    for (const segment of key.split('.')) value = (value as Record<string, unknown>)?.[segment]
    return String(value ?? key).replace(/\{(\w+)\}/g, (_, name) => String(params[name] ?? ''))
  } }) }
})
const api = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), success: vi.fn() }))
vi.mock('@/api/admin/ipTokenQuota', () => ({ getDailyIPTokenQuotaSettings: api.get, updateDailyIPTokenQuotaSettings: api.update }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess: api.success }) }))
const mountComponent = () => mount(component, {
  global: {
    stubs: { RouterLink: { template: '<a><slot /></a>' } },
  },
})

describe('Daily IP quota settings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockResolvedValue({ enabled: true, daily_token_limit: 100000000, timezone: 'Asia/Shanghai', whitelist_daily_token_limit: 0, whitelist: [] })
    api.update.mockImplementation(async data => data)
  })
  it('loads and saves database values including timezone and disable toggle', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    await wrapper.get('#daily-ip-token-limit').setValue('250000000')
    await wrapper.get('#daily-ip-token-timezone').setValue('UTC')
    await wrapper.get('[role="switch"]').trigger('click')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith({ enabled: false, daily_token_limit: 250000000, timezone: 'UTC', whitelist_daily_token_limit: 0, whitelist: [] })
    expect(api.success).toHaveBeenCalled()
    wrapper.unmount()
  })
  it('does not let load failures overwrite stored settings with defaults', async () => {
    api.get.mockRejectedValue(new Error('offline'))
    const wrapper = mountComponent()
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.find('button.btn-primary').exists()).toBe(false)
    expect(api.update).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('accepts zero, rejects fractional limits and shows save errors', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    await wrapper.get('#daily-ip-token-limit').setValue('0')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeUndefined()
    await wrapper.get('#daily-ip-token-limit').setValue('1.5')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    await wrapper.get('#daily-ip-token-limit').setValue('100000000')
    api.update.mockRejectedValue(new Error('save failed'))
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(api.success).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})

describe('Unified IP whitelist quota', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockResolvedValue({ enabled: true, daily_token_limit: 100000000, timezone: 'Asia/Shanghai', whitelist_daily_token_limit: 0, whitelist: [] })
    api.update.mockImplementation(async data => data)
  })
  it('saves one shared quota setting and an IP-only list', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    await wrapper.get('#daily-ip-token-limit').setValue('0')
    await wrapper.get('#whitelist-daily-token-limit').setValue('200000000')
    await wrapper.get('[data-testid="add-whitelist-ip"]').trigger('click')
    await wrapper.findAll('[data-testid="whitelist-ip-row"]')[0].get('[data-field="ip-address"]').setValue('203.0.113.8')
    await wrapper.get('[data-testid="add-whitelist-ip"]').trigger('click')
    await wrapper.findAll('[data-testid="whitelist-ip-row"]')[1].get('[data-field="ip-address"]').setValue('2001:db8::1')
    expect(wrapper.findAll('[data-field="ip-limit"]')).toHaveLength(0)
    expect(wrapper.findAll('#whitelist-daily-token-limit')).toHaveLength(1)
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith({ enabled: true, daily_token_limit: 0, timezone: 'Asia/Shanghai', whitelist_daily_token_limit: 200000000, whitelist: ['203.0.113.8', '2001:db8::1'] })
    wrapper.unmount()
  })
  it('accepts zero whitelist quota and rejects negative or fractional unified quotas', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    await wrapper.get('[data-testid="add-whitelist-ip"]').trigger('click')
    const row = wrapper.get('[data-testid="whitelist-ip-row"]')
    for (const value of ['not-an-ip', '999.1.1.1', '203.0.113.0/24', 'fe80::1%en0']) {
      await row.get('[data-field="ip-address"]').setValue(value)
      expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    }
    await row.get('[data-field="ip-address"]').setValue('2001:db8::1')
    for (const limit of ['-1', '1.5']) {
      await wrapper.get('#whitelist-daily-token-limit').setValue(limit)
      expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    }
    await wrapper.get('#whitelist-daily-token-limit').setValue('0')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-testid="add-whitelist-ip"]').trigger('click')
    await wrapper.findAll('[data-testid="whitelist-ip-row"]')[1].get('[data-field="ip-address"]').setValue('2001:0db8:0:0::1')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    expect(api.update).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('deletes an address without changing the quota for the remaining whitelist', async () => {
    api.get.mockResolvedValue({ enabled: true, daily_token_limit: 100000000, timezone: 'Asia/Shanghai', whitelist_daily_token_limit: 300000000, whitelist: ['203.0.113.8', '203.0.113.9'] })
    const wrapper = mountComponent()
    await flushPromises()
    await wrapper.findAll('[data-testid="whitelist-ip-row"]')[0].get('[data-testid="remove-whitelist-ip"]').trigger('click')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith({ enabled: true, daily_token_limit: 100000000, timezone: 'Asia/Shanghai', whitelist_daily_token_limit: 300000000, whitelist: ['203.0.113.9'] })
    wrapper.unmount()
  })
})
