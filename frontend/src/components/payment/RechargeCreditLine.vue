<template>
  <span class="inline-flex max-w-full items-baseline whitespace-nowrap">
    <span>{{ t('payment.getCreditLead') }}</span>
    <span
      :class="[
        'mx-1',
        bonus > 0 ? 'font-semibold text-amber-500 dark:text-amber-300' : '',
      ]"
    >{{ formatUsd(creditAmount) }}</span>
    <span>{{ t('payment.getCreditTrail') }}</span>
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  creditAmount: number
  bonus: number
}>()

const { t } = useI18n()

function formatUsd(amount: number): string {
  return `$${amount.toFixed(2)}`
}
</script>
