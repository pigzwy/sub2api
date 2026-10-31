<template>
  <div class="space-y-2 border-b border-gray-200 pb-4 dark:border-dark-600">
    <label class="flex cursor-pointer items-start gap-3 text-sm font-medium text-gray-900 dark:text-white">
      <input v-model="enabled" data-testid="first-output-protection" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
      {{ t('admin.accounts.tempUnschedulable.firstOutputTitle') }}
    </label>
    <p class="max-w-prose text-sm text-gray-600 dark:text-gray-300">{{ t('admin.accounts.tempUnschedulable.firstOutputHint') }}</p>
    <p class="max-w-prose text-xs text-gray-600 dark:text-gray-300">{{ t('admin.accounts.tempUnschedulable.firstOutputBoundary') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
const enabled = defineModel<boolean>({ required: true })
</script>
