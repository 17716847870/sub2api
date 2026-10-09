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
    api.get.mockResolvedValue({ enabled: true, daily_token_limit: 100000000, timezone: 'Asia/Shanghai' })
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
    expect(api.update).toHaveBeenCalledWith({ enabled: false, daily_token_limit: 250000000, timezone: 'UTC' })
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
  it('rejects zero or fractional limits and shows save errors', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    await wrapper.get('#daily-ip-token-limit').setValue('0')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
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
