import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getGatewayHeaderAuthSettings, updateGatewayHeaderAuthSettings } from '../gatewayHeaderAuth'

const api = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ default: api }))

describe('gateway header authentication API', () => {
  beforeEach(() => vi.clearAllMocks())
  it('preserves a legacy single rule during a rolling server upgrade', async () => {
    api.get.mockResolvedValue({ data: { enabled: false, header_name: 'X-Original', required_substring: 'keep-me' } })
    expect(await getGatewayHeaderAuthSettings()).toEqual({ enabled: false, rules: [{ header_name: 'X-Original', required_substring: 'keep-me' }] })
  })
  it('loads and sends all rules in the new array format', async () => {
    const settings = { enabled: true, rules: [{ header_name: 'User-Agent', required_substring: 'XundaAI' }, { header_name: 'X-App', required_substring: 'allowed' }] }
    api.get.mockResolvedValue({ data: settings })
    api.put.mockResolvedValue({ data: settings })
    expect(await getGatewayHeaderAuthSettings()).toEqual(settings)
    expect(await updateGatewayHeaderAuthSettings(settings)).toEqual(settings)
    expect(api.put).toHaveBeenCalledWith('/admin/settings/gateway-header-auth', settings)
  })
})
