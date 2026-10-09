<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('admin.ipTokenQuota.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.ipTokenQuota.description') }}</p>
        </div>
        <RouterLink to="/admin/settings?tab=gateway#daily-ip-token-quota" class="btn btn-secondary">{{ t('admin.ipTokenQuota.configure') }}</RouterLink>
      </div>

      <div v-if="result" class="grid gap-4 sm:grid-cols-3">
        <div class="card p-5">
          <p class="text-sm text-gray-500">{{ t('admin.ipTokenQuota.limitedCount') }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber(activeTotal) }}</p>
        </div>
        <div class="card p-5">
          <p class="text-sm text-gray-500">{{ t('admin.ipTokenQuota.dailyLimit') }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber(result.settings.daily_token_limit) }}</p>
          <p class="mt-1 text-xs text-gray-500">{{ result.settings.enabled ? t('admin.ipTokenQuota.on') : t('admin.ipTokenQuota.off') }}</p>
        </div>
        <div class="card p-5">
          <p class="text-sm text-gray-500">{{ t('admin.ipTokenQuota.nextReset') }}</p>
          <p class="mt-2 font-semibold text-gray-900 dark:text-white">{{ formatTime(result.reset_at) }}</p>
          <p class="mt-1 text-xs text-gray-500">{{ result.settings.timezone }}</p>
        </div>
      </div>

      <div v-if="result && !result.settings.enabled" class="rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200" role="status">{{ t('admin.ipTokenQuota.disabledHint') }}</div>
      <div v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</div>

      <div class="card overflow-hidden">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 p-4 dark:border-dark-700">
          <form class="flex w-full gap-2 sm:w-auto" @submit.prevent="searchIPs">
            <input v-model="searchInput" type="search" maxlength="45" :aria-label="t('admin.ipTokenQuota.searchPlaceholder')" :placeholder="t('admin.ipTokenQuota.searchPlaceholder')" class="input sm:w-72" />
            <button type="submit" class="btn btn-secondary">{{ t('common.search') }}</button>
          </form>
          <div class="flex items-center gap-3">
            <label class="flex items-center gap-2 text-sm text-gray-500"><input v-model="autoRefresh" type="checkbox" class="rounded" />{{ t('admin.ipTokenQuota.autoRefresh') }}</label>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">{{ loading ? t('common.loading') : t('admin.ipTokenQuota.refresh') }}</button>
          </div>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
              <tr><th class="px-5 py-3">IP</th><th class="px-5 py-3">{{ t('admin.ipTokenQuota.usedTokens') }}</th><th class="px-5 py-3">{{ t('admin.ipTokenQuota.requests') }}</th><th class="px-5 py-3">{{ t('common.status') }}</th><th class="px-5 py-3">{{ t('admin.ipTokenQuota.resetAt') }}</th><th class="px-5 py-3">{{ t('admin.ipTokenQuota.remaining') }}</th><th class="px-5 py-3">{{ t('admin.ipTokenQuota.lastUsed') }}</th></tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in activeItems" :key="item.ip_address" class="text-gray-700 dark:text-gray-200">
                <td class="whitespace-nowrap px-5 py-4 font-mono">{{ item.ip_address }}</td>
                <td class="whitespace-nowrap px-5 py-4"><span class="font-semibold">{{ formatNumber(item.used_tokens) }}</span><span class="mt-1 block text-xs text-gray-500">{{ t('admin.ipTokenQuota.limitShort', { limit: formatNumber(result!.settings.daily_token_limit) }) }}</span></td>
                <td class="px-5 py-4">{{ formatNumber(item.request_count) }}</td>
                <td class="whitespace-nowrap px-5 py-4"><span class="rounded-full bg-red-50 px-2.5 py-1 text-xs font-medium text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ t('admin.ipTokenQuota.blocked') }}</span></td>
                <td class="whitespace-nowrap px-5 py-4">{{ formatTime(item.reset_at) }}</td>
                <td class="whitespace-nowrap px-5 py-4 tabular-nums">{{ remaining(item.reset_at) }}</td>
                <td class="whitespace-nowrap px-5 py-4 text-gray-500">{{ formatTime(item.last_used_at) }}</td>
              </tr>
              <tr v-if="!activeItems.length"><td colspan="7" class="px-6 py-16 text-center text-gray-500">{{ loading ? t('common.loading') : error ? t('admin.ipTokenQuota.unavailable') : t('admin.ipTokenQuota.empty') }}</td></tr>
            </tbody>
          </table>
        </div>
        <Pagination v-if="activeTotal > 0" :page="page" :page-size="pageSize" :total="activeTotal" :page-size-options="[20, 50, 100]" @update:page="changePage" @update:pageSize="changePageSize" />
      </div>
      <p class="text-xs text-gray-500">{{ t('admin.ipTokenQuota.listHint') }}</p>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listLimitedIPs, type LimitedIPList } from '@/api/admin/ipTokenQuota'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t, locale } = useI18n()
const result = ref<LimitedIPList | null>(null)
const loading = ref(false)
const error = ref('')
const searchInput = ref('')
const search = ref('')
const page = ref(1)
const pageSize = ref(20)
const autoRefresh = ref(true)
const now = ref(Date.now())
let serverOffset = 0
let controller: AbortController | undefined
let interval: ReturnType<typeof setInterval> | undefined
let lastRefresh = Date.now()
let generation = 0
let rolloverRefresh = false
const rolledOver = computed(() => !!result.value && now.value >= Date.parse(result.value.reset_at))
const activeItems = computed(() => rolledOver.value ? [] : result.value?.items ?? [])
const activeTotal = computed(() => rolledOver.value ? 0 : result.value?.total ?? 0)
const formatNumber = (n: number) => new Intl.NumberFormat(locale.value).format(n)
function formatTime(value: string) {
  return new Intl.DateTimeFormat(locale.value, { timeZone: result.value?.settings.timezone || 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(value))
}
function remaining(value: string) {
  const seconds = Math.max(0, Math.ceil((Date.parse(value) - now.value) / 1000))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor(seconds % 3600 / 60)
  return t('admin.ipTokenQuota.countdown', { hours, minutes, seconds: seconds % 60 })
}
async function load() {
  controller?.abort()
  const current = ++generation
  controller = new AbortController()
  loading.value = true
  error.value = ''
  try {
    const data = await listLimitedIPs({ page: page.value, page_size: pageSize.value, search: search.value }, controller.signal)
    if (current !== generation) return
    // Settings may have changed and removed IPs from the final page.
    if (data.total > 0 && page.value > Math.ceil(data.total / pageSize.value)) {
      page.value = Math.ceil(data.total / pageSize.value)
      await load()
      return
    }
    result.value = data
    serverOffset = Date.parse(data.server_time) - Date.now()
    now.value = Date.now() + serverOffset
    lastRefresh = Date.now()
    rolloverRefresh = false
  } catch (err) {
    if (current !== generation || controller.signal.aborted) return
    result.value = null
    error.value = extractApiErrorMessage(err, t('admin.ipTokenQuota.loadFailed'))
    lastRefresh = Date.now()
  } finally { if (current === generation) loading.value = false }
}
function searchIPs() { search.value = searchInput.value.trim(); page.value = 1; void load() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void load() }

onMounted(() => {
  void load()
  interval = setInterval(() => {
    now.value = Date.now() + serverOffset
    if (rolledOver.value && !loading.value && !rolloverRefresh) {
      rolloverRefresh = true
      page.value = 1
      void load()
    } else if (autoRefresh.value && !loading.value && Date.now() - lastRefresh >= 30000) { void load() }
  }, 1000)
})
onBeforeUnmount(() => { generation++; controller?.abort(); if (interval) clearInterval(interval) })
</script>
