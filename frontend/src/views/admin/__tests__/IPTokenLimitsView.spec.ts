import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import view from '../IPTokenLimitsView.vue'

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
const api = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('@/api/admin/ipTokenQuota', () => ({ listLimitedIPs: api.list }))
const fixture = () => ({
  items: [{ ip_address: '203.0.113.8', used_tokens: 120000000, daily_token_limit: 100000000, whitelisted: false, request_count: 24, last_used_at: '2026-10-09T10:00:00+08:00', reset_at: '2026-10-10T00:00:00+08:00' }],
  total: 1, page: 1, page_size: 20,
  settings: { enabled: true, daily_token_limit: 100000000, timezone: 'Asia/Shanghai', whitelist_daily_token_limit: 0, whitelist: [] },
  day_start: '2026-10-09T00:00:00+08:00', reset_at: '2026-10-10T00:00:00+08:00', server_time: '2026-10-09T10:00:00+08:00',
})
const mountView = () => mount(view, { global: {
  stubs: { AppLayout: { template: '<div><slot /></div>' }, Pagination: true, RouterLink: { template: '<a><slot /></a>' } },
} })

describe('limited IP admin list', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-09T02:00:00Z'))
    api.list.mockReset()
    api.list.mockResolvedValue(fixture())
  })
  afterEach(() => vi.useRealTimers())
  it('shows usage and countdown in the configured timezone and searches', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('203.0.113.8')
    expect(wrapper.text()).toContain('120,000,000')
    expect(wrapper.text()).toContain('14h 0m 0s')
    expect(wrapper.text()).toContain('Asia/Shanghai')
    await wrapper.get('input[type="search"]').setValue('203.0')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.list).toHaveBeenLastCalledWith({ page: 1, page_size: 20, search: '203.0' }, expect.any(AbortSignal))
    wrapper.unmount()
  })
  it('clears old blocked rows at midnight even with automatic refresh disabled', async () => {
    const data = fixture()
    data.server_time = '2026-10-09T23:59:59+08:00'
    api.list.mockResolvedValueOnce(data).mockResolvedValue({ ...data, items: [], total: 0, server_time: '2026-10-10T00:00:00+08:00', reset_at: '2026-10-11T00:00:00+08:00' })
    vi.setSystemTime(new Date('2026-10-09T15:59:59Z'))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('input[type="checkbox"]').setValue(false)
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    expect(wrapper.text()).not.toContain('203.0.113.8')
    expect(api.list).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
  it('distinguishes list failures from an empty successful response', async () => {
    api.list.mockRejectedValue(new Error('offline'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('unavailable')
    expect(wrapper.text()).not.toContain('No IPs have reached')
    wrapper.unmount()
  })
})

it('shows each limited IP’s own quota and a whitelist badge when the default is unlimited', async () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-09T02:00:00Z'))
  const data = fixture()
  data.settings.daily_token_limit = 0
  data.items[0].daily_token_limit = 50000000
  data.items[0].whitelisted = true
  api.list.mockResolvedValue(data)
  const wrapper = mountView()
  await flushPromises()
  expect(wrapper.text()).toContain('Unlimited')
  expect(wrapper.text()).toContain('Whitelist quota')
  expect(wrapper.text()).toContain('Limit 50,000,000')
  expect(wrapper.text()).not.toContain('Limit 0')
  wrapper.unmount()
  vi.useRealTimers()
})
