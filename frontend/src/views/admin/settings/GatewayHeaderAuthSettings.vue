<template>
  <section id="gateway-header-auth" class="card" aria-labelledby="gateway-header-auth-title">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="gateway-header-auth-title" class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.gatewayHeaderAuth.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.gatewayHeaderAuth.description') }}</p>
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
            <label for="gateway-header-auth-enabled" class="font-medium text-gray-900 dark:text-gray-100">{{ t('admin.gatewayHeaderAuth.enabledLabel') }}</label>
            <p class="mt-1 text-sm text-gray-500">{{ t('admin.gatewayHeaderAuth.enabledHint') }}</p>
          </div>
          <Toggle id="gateway-header-auth-enabled" v-model="form.enabled" :aria-label="t('admin.gatewayHeaderAuth.enabledLabel')" :disabled="saving" />
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="font-medium text-gray-900 dark:text-gray-100">{{ t('admin.gatewayHeaderAuth.rulesTitle') }}</h3>
            <p class="mt-1 text-sm text-gray-500">{{ t('admin.gatewayHeaderAuth.matchAllHint') }}</p>
          </div>
          <button type="button" data-testid="add-header-rule" class="btn btn-secondary" :disabled="saving || form.rules.length >= MAX_RULES" @click="addRule">+ {{ t('admin.gatewayHeaderAuth.addRule') }}</button>
        </div>
        <div class="space-y-4">
          <div v-for="(rule, index) in form.rules" :key="rule.id" data-testid="gateway-header-rule" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="mb-4 flex items-center justify-between gap-3">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.gatewayHeaderAuth.ruleNumber', { number: index + 1 }) }}</span>
              <button type="button" data-testid="remove-header-rule" class="text-sm font-medium text-red-600 hover:text-red-700 disabled:opacity-50 dark:text-red-400" :disabled="saving" :aria-label="t('admin.gatewayHeaderAuth.removeRuleLabel', { number: index + 1 })" @click="removeRule(rule.id)">{{ t('admin.gatewayHeaderAuth.removeRule') }}</button>
            </div>
            <div class="grid gap-5 md:grid-cols-2">
              <div>
                <label :for="`gateway-auth-header-name-${rule.id}`" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.gatewayHeaderAuth.headerName') }}</label>
                <input :id="`gateway-auth-header-name-${rule.id}`" v-model.trim="rule.header_name" data-field="header-name" type="text" maxlength="128" class="input font-mono" placeholder="User-Agent" :disabled="saving" spellcheck="false" />
              </div>
              <div>
                <label :for="`gateway-auth-required-substring-${rule.id}`" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.gatewayHeaderAuth.requiredSubstring') }}</label>
                <input :id="`gateway-auth-required-substring-${rule.id}`" v-model="rule.required_substring" data-field="required-substring" type="text" maxlength="512" class="input font-mono" placeholder="XundaAI" :disabled="saving" spellcheck="false" />
              </div>
            </div>
          </div>
          <p v-if="!form.rules.length" role="status" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">{{ t('admin.gatewayHeaderAuth.emptyRulesHint') }}</p>
        </div>
        <div class="space-y-1 text-xs text-gray-500">
          <p>{{ t('admin.gatewayHeaderAuth.headerNameHint') }}</p>
          <p>{{ t('admin.gatewayHeaderAuth.matchHint') }}</p>
          <p>{{ t('admin.gatewayHeaderAuth.maxRulesHint', { max: MAX_RULES }) }}</p>
        </div>
        <div class="rounded-lg bg-gray-50 p-4 text-sm dark:bg-dark-900/40">
          <p class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.gatewayHeaderAuth.exampleLabel') }}</p>
          <div class="mt-2 space-y-1">
            <code v-for="rule in form.rules" :key="rule.id" class="block break-all text-primary-600 dark:text-primary-400">{{ rule.header_name || 'Header-Name' }}: {{ rule.required_substring || '...' }}</code>
          </div>
          <p class="mt-2 text-xs text-gray-500">{{ t('admin.gatewayHeaderAuth.rejectionHint') }}</p>
        </div>
        <p v-if="saveError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ saveError }}</p>
        <div class="flex justify-end">
          <button type="button" data-testid="save-header-auth" class="btn btn-primary" :disabled="saving || !valid" @click="save">{{ saving ? t('common.submitting') : t('admin.gatewayHeaderAuth.saveSettings') }}</button>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { getGatewayHeaderAuthSettings, updateGatewayHeaderAuthSettings, type GatewayHeaderAuthRule, type GatewayHeaderAuthSettings } from '@/api/admin/gatewayHeaderAuth'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

interface RuleRow extends GatewayHeaderAuthRule { id: number }
const MAX_RULES = 32
const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')
let nextRuleID = 1
const form = reactive<{ enabled: boolean; rules: RuleRow[] }>({ enabled: true, rules: [] })
const validRule = (rule: GatewayHeaderAuthRule) => /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(rule.header_name) && rule.header_name.length <= 128 && !!rule.required_substring.trim() && rule.required_substring.length <= 512 && !/[\r\n\0]/.test(rule.required_substring)
const valid = computed(() => (!form.enabled || form.rules.length > 0) && form.rules.length <= MAX_RULES && form.rules.every(validRule))

function applySettings(settings: GatewayHeaderAuthSettings) {
  form.enabled = settings.enabled
  form.rules = settings.rules.map(rule => ({ ...rule, id: nextRuleID++ }))
}
function addRule() {
  if (saving.value || form.rules.length >= MAX_RULES) return
  form.rules.push({ id: nextRuleID++, header_name: '', required_substring: '' })
}
function removeRule(id: number) {
  if (saving.value) return
  form.rules = form.rules.filter(rule => rule.id !== id)
}
async function load() {
  loading.value = true
  loadError.value = ''
  try { applySettings(await getGatewayHeaderAuthSettings()) }
  catch (error) { loadError.value = extractApiErrorMessage(error, t('admin.gatewayHeaderAuth.loadFailed')) }
  finally { loading.value = false }
}

async function save() {
  if (!valid.value || saving.value) return
  saving.value = true
  saveError.value = ''
  try {
    const settings = { enabled: form.enabled, rules: form.rules.map(rule => ({ header_name: rule.header_name, required_substring: rule.required_substring })) }
    applySettings(await updateGatewayHeaderAuthSettings(settings))
    appStore.showSuccess(t('admin.gatewayHeaderAuth.saved'))
  } catch (error) { saveError.value = extractApiErrorMessage(error, t('admin.gatewayHeaderAuth.saveFailed')) }
  finally { saving.value = false }
}

onMounted(load)
</script>
