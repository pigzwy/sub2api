<template>
  <span>
    <span>{{ t('payment.getCreditLead') }}</span>
    <template v-if="bonus > 0">
      <s class="mx-1 text-gray-400 dark:text-gray-500">{{ formatUsd(baseAmount) }}</s>
      <span class="font-semibold text-amber-500 dark:text-amber-300">{{ formatUsd(creditAmount) }}</span>
    </template>
    <span v-else class="mx-1">{{ formatUsd(creditAmount) }}</span>
    <span> {{ t('payment.getCreditTrail') }}</span>
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  baseAmount: number
  creditAmount: number
  bonus: number
}>()

const { t } = useI18n()

function formatUsd(amount: number): string {
  return `$${amount.toFixed(2)}`
}
</script>
