import apiClient from '../client'

export interface DailyIPTokenQuotaSettings {
  enabled: boolean
  daily_token_limit: number
  timezone: string
  whitelist_daily_token_limit: number
  whitelist: string[]
}

export interface LimitedIP {
  ip_address: string
  daily_token_limit: number
  whitelisted: boolean
  used_tokens: number
  request_count: number
  last_used_at: string
  reset_at: string
}

export interface LimitedIPList {
  items: LimitedIP[]
  total: number
  page: number
  page_size: number
  settings: DailyIPTokenQuotaSettings
  day_start: string
  reset_at: string
  server_time: string
}

const endpoint = '/admin/settings/daily-ip-token-quota'

export async function getDailyIPTokenQuotaSettings(): Promise<DailyIPTokenQuotaSettings> {
  const { data } = await apiClient.get<DailyIPTokenQuotaSettings>(endpoint)
  return { ...data, whitelist_daily_token_limit: data.whitelist_daily_token_limit ?? 0, whitelist: data.whitelist ?? [] }
}

export async function updateDailyIPTokenQuotaSettings(settings: DailyIPTokenQuotaSettings): Promise<DailyIPTokenQuotaSettings> {
  const { data } = await apiClient.put<DailyIPTokenQuotaSettings>(endpoint, settings)
  return data
}

export async function listLimitedIPs(params: { page: number; page_size: number; search: string }, signal?: AbortSignal): Promise<LimitedIPList> {
  const { data } = await apiClient.get<LimitedIPList>(`${endpoint}/limited-ips`, { params, signal })
  return data
}
