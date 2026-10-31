<template>
  <div class="space-y-3 border-b border-gray-200 pb-4 dark:border-dark-600">
    <label class="flex cursor-pointer items-start gap-3 text-sm font-medium text-gray-900 dark:text-white">
      <input v-model="enabled" data-testid="first-output-protection" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
      {{ t('admin.accounts.tempUnschedulable.firstOutputTitle') }}
    </label>
    <div v-if="enabled" class="grid grid-cols-1 gap-4 sm:grid-cols-3">
      <div v-for="field in firstOutputFields" :key="field.key" class="min-w-0">
        <label :for="`${id}-${field.key}`" class="mb-1.5 block text-sm font-medium text-gray-900 dark:text-white">
          {{ t(`admin.accounts.tempUnschedulable.${field.label}`) }}
        </label>
        <input
          :id="`${id}-${field.key}`"
          v-model.number="config[field.key]"
          :data-testid="`first-output-${field.key}`"
          type="number"
          min="1"
          :max="field.max"
          step="1"
          required
          class="input w-full tabular-nums"
          :class="{ 'border-red-500 dark:border-red-400': !isFirstOutputValueValid(config[field.key], field.max) }"
          :aria-invalid="!isFirstOutputValueValid(config[field.key], field.max)"
          :aria-describedby="`${id}-${field.key}-hint`"
        />
        <p
          :id="`${id}-${field.key}-hint`"
          class="mt-1 text-xs"
          :class="isFirstOutputValueValid(config[field.key], field.max) ? 'text-gray-600 dark:text-gray-300' : 'text-red-600 dark:text-red-400'"
        >
          {{ t('admin.accounts.tempUnschedulable.firstOutputIntegerRange', { min: 1, max: field.max }) }}
        </p>
      </div>
    </div>
    <p v-if="enabled && valid" class="max-w-prose text-sm text-gray-600 dark:text-gray-300">
      {{ t('admin.accounts.tempUnschedulable.firstOutputHint', { seconds: config.limitSeconds, cooldown: config.cooldownMinutes, window: config.windowMinutes }) }}
    </p>
    <p class="max-w-prose text-xs text-gray-600 dark:text-gray-300">{{ t('admin.accounts.tempUnschedulable.firstOutputBoundary') }}</p>
    <p v-if="enabled" class="max-w-prose text-xs text-gray-600 dark:text-gray-300">{{ t('admin.accounts.tempUnschedulable.firstOutputConfigChangeHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import { firstOutputFields, isFirstOutputValueValid, type FirstOutputProtectionForm } from './firstOutputProtection'

const { t } = useI18n()
const id = useId()
const enabled = defineModel<boolean>({ required: true })
const config = defineModel<FirstOutputProtectionForm>('config', { required: true })
const valid = computed(() => firstOutputFields.every(field => isFirstOutputValueValid(config.value[field.key], field.max)))
</script>
