<template>
  <section class="card" aria-labelledby="telegram-settings-title">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="telegram-settings-title" class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.telegram.title') }}</h2>
      <p class="mt-1 max-w-prose text-sm text-gray-600 dark:text-gray-300">{{ t('admin.settings.telegram.description') }}</p>
    </div>
    <div class="space-y-4 p-6" :aria-busy="loading || saving">
      <p v-if="error" role="alert" class="text-sm text-red-700 dark:text-red-300">{{ error }}</p>
      <div v-if="loading" class="h-24 rounded-lg bg-gray-100 dark:bg-dark-700" />
      <button v-else-if="!loaded" type="button" class="btn btn-secondary btn-sm" @click="load">{{ t('admin.settings.telegram.retry') }}</button>
      <template v-else>
        <label class="flex items-center gap-3 text-sm font-medium">
          <input v-model="form.enabled" type="checkbox" :disabled="saving" class="h-4 w-4 rounded" />
          {{ t('admin.settings.telegram.enabled') }}
        </label>
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="block text-sm font-medium">
            {{ t('admin.settings.telegram.token') }}
            <input v-model="form.bot_token" data-testid="telegram-token" type="password" autocomplete="new-password" maxlength="256" :disabled="saving" :placeholder="form.bot_token_configured ? t('admin.settings.telegram.tokenSaved') : ''" class="input mt-2 w-full" />
          </label>
          <label class="block text-sm font-medium">
            {{ t('admin.settings.telegram.chatId') }}
            <input v-model="form.chat_id" data-testid="telegram-chat-id" type="text" autocomplete="off" maxlength="128" :disabled="saving" class="input mt-2 w-full" />
          </label>
        </div>
        <p class="max-w-prose text-xs text-gray-600 dark:text-gray-300">{{ t('admin.settings.telegram.hint') }}</p>
        <div class="flex flex-wrap items-center gap-3">
          <button type="button" class="btn btn-primary btn-sm" :disabled="saving" @click="save">{{ saving ? t('common.saving') : t('common.save') }}</button>
          <p v-if="saved" role="status" class="text-sm text-emerald-700 dark:text-emerald-300">{{ t('admin.settings.telegram.saved') }}</p>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getTelegramNotificationSettings, updateTelegramNotificationSettings, type TelegramNotificationSettings } from '@/api/admin/telegramNotification'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const loading = ref(true)
const loaded = ref(false)
const saving = ref(false)
const saved = ref(false)
const error = ref('')
const form = reactive<TelegramNotificationSettings>({ enabled: false, chat_id: '', bot_token: '', bot_token_configured: false })

async function load() {
  loading.value = true
  error.value = ''
  try {
    Object.assign(form, await getTelegramNotificationSettings(), { bot_token: '' })
    loaded.value = true
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('admin.settings.telegram.loadFailed'))
  } finally { loading.value = false }
}

async function save() {
  error.value = ''
  saved.value = false
  if (form.enabled && (!form.chat_id.trim() || (!form.bot_token?.trim() && !form.bot_token_configured))) {
    error.value = t('admin.settings.telegram.incomplete')
    return
  }
  saving.value = true
  try {
    Object.assign(form, await updateTelegramNotificationSettings({ ...form }), { bot_token: '' })
    saved.value = true
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('admin.settings.telegram.saveFailed'))
  } finally { saving.value = false }
}

onMounted(load)
</script>
