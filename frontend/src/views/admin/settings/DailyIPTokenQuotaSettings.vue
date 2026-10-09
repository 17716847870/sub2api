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
            <input id="daily-ip-token-limit" v-model.number="form.daily_token_limit" type="number" min="1" max="9007199254740991" step="1" class="input" :disabled="saving" />
            <p class="mt-2 text-xs text-gray-500">{{ t('admin.ipTokenQuota.limitHint') }}</p>
          </div>
          <div>
            <label for="daily-ip-token-timezone" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ipTokenQuota.timezoneLabel') }}</label>
            <input id="daily-ip-token-timezone" v-model.trim="form.timezone" list="ip-quota-timezones" class="input" placeholder="Asia/Shanghai" :disabled="saving" />
            <datalist id="ip-quota-timezones"><option value="Asia/Shanghai" /><option value="UTC" /><option value="Asia/Tokyo" /><option value="America/New_York" /><option value="Europe/London" /></datalist>
            <p class="mt-2 text-xs text-gray-500">{{ t('admin.ipTokenQuota.timezoneHint') }}</p>
          </div>
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
import { getDailyIPTokenQuotaSettings, updateDailyIPTokenQuotaSettings } from '@/api/admin/ipTokenQuota'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')
const form = reactive({ enabled: true, daily_token_limit: 100000000, timezone: 'Asia/Shanghai' })
const valid = computed(() => Number.isSafeInteger(form.daily_token_limit) && form.daily_token_limit > 0 && !!form.timezone.trim())

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    Object.assign(form, await getDailyIPTokenQuotaSettings())
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.ipTokenQuota.loadFailed'))
  } finally { loading.value = false }
}

async function save() {
  if (!valid.value || saving.value) return
  saving.value = true
  saveError.value = ''
  try {
    Object.assign(form, await updateDailyIPTokenQuotaSettings({ ...form }))
    appStore.showSuccess(t('admin.ipTokenQuota.saved'))
  } catch (error) {
    saveError.value = extractApiErrorMessage(error, t('admin.ipTokenQuota.saveFailed'))
  } finally { saving.value = false }
}

onMounted(load)
</script>
