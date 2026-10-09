<template>
  <section id="daily-ip-token-quota" class="card" aria-labelledby="ip-quota-settings-title">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="ip-quota-settings-title" class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.ipTokenQuota.settingsTitle') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.ipTokenQuota.settingsDescription') }}</p>
    </div>
    <div class="space-y-5 p-6">
      <p v-if="loading" role="status" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
      <div v-else-if="loadError" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
        <p>{{ loadError }}</p>
        <button type="button" class="btn btn-secondary mt-3" @click="load">{{ t('common.tryAgain') }}</button>
      </div>
      <template v-else>
        <div class="flex items-center justify-between gap-4">
          <div>
            <label for="daily-ip-quota-enabled" class="font-medium text-gray-900 dark:text-gray-100">{{ t('admin.ipTokenQuota.enabledLabel') }}</label>
            <p class="mt-1 text-sm text-gray-500">{{ t('admin.ipTokenQuota.sharedHint') }}</p>
          </div>
          <Toggle id="daily-ip-quota-enabled" v-model="form.enabled" :aria-label="t('admin.ipTokenQuota.enabledLabel')" :disabled="saving" />
        </div>
        <div class="grid gap-5 md:grid-cols-2">
          <div>
            <label for="daily-ip-token-limit" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ipTokenQuota.limitLabel') }}</label>
            <input id="daily-ip-token-limit" v-model.number="form.daily_token_limit" type="number" min="0" max="9007199254740991" step="1" class="input" :disabled="saving" />
            <p class="mt-2 text-xs text-gray-500">{{ t('admin.ipTokenQuota.limitHint') }}</p>
          </div>
          <div>
            <label for="daily-ip-token-timezone" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ipTokenQuota.timezoneLabel') }}</label>
            <input id="daily-ip-token-timezone" v-model.trim="form.timezone" list="ip-quota-timezones" class="input" placeholder="Asia/Shanghai" :disabled="saving" />
            <datalist id="ip-quota-timezones"><option value="Asia/Shanghai" /><option value="UTC" /><option value="Asia/Tokyo" /><option value="America/New_York" /><option value="Europe/London" /></datalist>
            <p class="mt-2 text-xs text-gray-500">{{ t('admin.ipTokenQuota.timezoneHint') }}</p>
          </div>
        </div>
        <div class="border-t border-gray-100 pt-5 dark:border-dark-700">
          <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 class="font-medium text-gray-900 dark:text-white">{{ t('admin.ipTokenQuota.whitelistTitle') }}</h3>
              <p class="mt-1 text-sm text-gray-500">{{ t('admin.ipTokenQuota.whitelistHint') }}</p>
            </div>
            <button type="button" data-testid="add-whitelist-ip" class="btn btn-secondary" :disabled="saving || form.whitelist.length >= MAX_WHITELIST" @click="addIP">+ {{ t('admin.ipTokenQuota.addIP') }}</button>
          </div>
          <div class="mb-5">
            <label for="whitelist-daily-token-limit" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ipTokenQuota.whitelistLimit') }}</label>
            <input id="whitelist-daily-token-limit" v-model.number="form.whitelist_daily_token_limit" type="number" min="0" max="9007199254740991" step="1" class="input md:max-w-md" :disabled="saving" />
            <p class="mt-2 text-xs text-gray-500">{{ t('admin.ipTokenQuota.whitelistLimitHint') }}</p>
          </div>
          <div class="space-y-3">
            <div v-for="(entry, index) in form.whitelist" :key="entry.id" data-testid="whitelist-ip-row" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
              <div class="mb-3 flex items-center justify-between gap-3">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ipTokenQuota.whitelistNumber', { number: index + 1 }) }}</span>
                <button type="button" data-testid="remove-whitelist-ip" class="text-sm text-red-600 disabled:opacity-50 dark:text-red-400" :disabled="saving" :aria-label="t('admin.ipTokenQuota.removeIPLabel', { number: index + 1 })" @click="removeIP(entry.id)">{{ t('admin.ipTokenQuota.removeIP') }}</button>
              </div>
              <div>
                <label :for="`whitelist-ip-${entry.id}`" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ipTokenQuota.ipAddress') }}</label>
                <input :id="`whitelist-ip-${entry.id}`" v-model.trim="entry.ip_address" data-field="ip-address" type="text" maxlength="45" class="input font-mono" placeholder="203.0.113.8" :disabled="saving" spellcheck="false" />
              </div>
            </div>
            <p v-if="!form.whitelist.length" class="rounded-lg bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-900/40">{{ t('admin.ipTokenQuota.emptyWhitelist') }}</p>
          </div>
          <p class="mt-3 text-xs text-gray-500">{{ t('admin.ipTokenQuota.exactIPHint') }}</p>
        </div>
        <p class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/15 dark:text-amber-200">{{ t('admin.ipTokenQuota.inflightHint') }}</p>
        <p v-if="saveError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ saveError }}</p>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <RouterLink class="text-sm font-medium text-primary-600 hover:underline dark:text-primary-400" to="/admin/ip-token-limits">{{ t('admin.ipTokenQuota.viewLimitedIPs') }} →</RouterLink>
          <button type="button" class="btn btn-primary" :disabled="saving || !valid" @click="save">{{ saving ? t('common.submitting') : t('admin.ipTokenQuota.saveSettings') }}</button>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { getDailyIPTokenQuotaSettings, updateDailyIPTokenQuotaSettings, type DailyIPTokenQuotaSettings } from '@/api/admin/ipTokenQuota'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')
interface WhitelistRow { id: number; ip_address: string }
const MAX_WHITELIST = 1000
let nextIPID = 1
const form = reactive<{ enabled: boolean; daily_token_limit: number; timezone: string; whitelist_daily_token_limit: number; whitelist: WhitelistRow[] }>({ enabled: true, daily_token_limit: 100000000, timezone: 'Asia/Shanghai', whitelist_daily_token_limit: 0, whitelist: [] })
const validLimit = (limit: number) => Number.isSafeInteger(limit) && limit >= 0
function normalizedIP(value: string): string | null {
  if (/^(\d{1,3}\.){3}\d{1,3}$/.test(value)) {
    const parts = value.split('.')
    return parts.every(part => Number(part) <= 255 && String(Number(part)) === part) ? value : null
  }
  if (!value.includes(':') || !/^[0-9a-fA-F:.]+$/.test(value)) return null
  try { return new URL(`http://[${value}]/`).hostname } catch { return null }
}
const valid = computed(() => {
  if (!validLimit(form.daily_token_limit) || !validLimit(form.whitelist_daily_token_limit) || !form.timezone.trim() || form.whitelist.length > MAX_WHITELIST) return false
  const seen = new Set<string>()
  return form.whitelist.every(entry => {
    const ip = normalizedIP(entry.ip_address)
    if (!ip || seen.has(ip)) return false
    seen.add(ip)
    return true
  })
})
function applySettings(settings: DailyIPTokenQuotaSettings) {
  form.enabled = settings.enabled
  form.daily_token_limit = settings.daily_token_limit
  form.timezone = settings.timezone
  form.whitelist_daily_token_limit = settings.whitelist_daily_token_limit ?? 0
  form.whitelist = (settings.whitelist ?? []).map(ip => ({ ip_address: ip, id: nextIPID++ }))
}
function addIP() {
  if (saving.value || form.whitelist.length >= MAX_WHITELIST) return
  form.whitelist.push({ id: nextIPID++, ip_address: '' })
}
function removeIP(id: number) {
  if (!saving.value) form.whitelist = form.whitelist.filter(entry => entry.id !== id)
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    applySettings(await getDailyIPTokenQuotaSettings())
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.ipTokenQuota.loadFailed'))
  } finally { loading.value = false }
}

async function save() {
  if (!valid.value || saving.value) return
  saving.value = true
  saveError.value = ''
  try {
    const settings = { enabled: form.enabled, daily_token_limit: form.daily_token_limit, timezone: form.timezone, whitelist_daily_token_limit: form.whitelist_daily_token_limit, whitelist: form.whitelist.map(entry => entry.ip_address) }
    applySettings(await updateDailyIPTokenQuotaSettings(settings))
    appStore.showSuccess(t('admin.ipTokenQuota.saved'))
  } catch (error) {
    saveError.value = extractApiErrorMessage(error, t('admin.ipTokenQuota.saveFailed'))
  } finally { saving.value = false }
}

onMounted(load)
</script>
