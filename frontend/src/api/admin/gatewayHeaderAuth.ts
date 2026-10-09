import apiClient from '../client'

export interface GatewayHeaderAuthRule {
  header_name: string
  required_substring: string
}

export interface GatewayHeaderAuthSettings {
  enabled: boolean
  rules: GatewayHeaderAuthRule[]
}

// During a rolling upgrade, the previous server may still return one rule.
interface LegacyGatewayHeaderAuthSettings extends GatewayHeaderAuthRule {
  enabled: boolean
}

const endpoint = '/admin/settings/gateway-header-auth'

export async function getGatewayHeaderAuthSettings(): Promise<GatewayHeaderAuthSettings> {
  const { data } = await apiClient.get<GatewayHeaderAuthSettings | LegacyGatewayHeaderAuthSettings>(endpoint)
  if ('rules' in data) return data
  return { enabled: data.enabled, rules: [{ header_name: data.header_name, required_substring: data.required_substring }] }
}

export async function updateGatewayHeaderAuthSettings(settings: GatewayHeaderAuthSettings): Promise<GatewayHeaderAuthSettings> {
  const { data } = await apiClient.put<GatewayHeaderAuthSettings>(endpoint, settings)
  return data
}
