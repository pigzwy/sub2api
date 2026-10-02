<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { RechargeBonusMode, RechargeBonusTier } from '@/types/payment'

const { t } = useI18n()
const mode = defineModel<RechargeBonusMode>('mode', { required: true })
const tiers = defineModel<RechargeBonusTier[]>('tiers', { required: true })

function addTier() {
  if (tiers.value.length >= 20) return
  const largest = Math.max(0, ...tiers.value.map((tier) => Number(tier.min_amount) || 0))
  tiers.value = [...tiers.value, { min_amount: largest + 100, bonus_percent: 0 }]
}
</script>

<template>
  <section class="space-y-4 border-b border-gray-100 p-6 dark:border-dark-700" data-testid="recharge-bonus-settings">
    <div>
      <label for="recharge-bonus-mode" class="mb-2 block text-sm font-medium text-gray-900 dark:text-white">
        {{ t('admin.settings.payment.bonusMode') }}
      </label>
      <select id="recharge-bonus-mode" v-model="mode" class="input max-w-sm" data-testid="recharge-bonus-mode">
        <option value="fixed">{{ t('admin.settings.payment.bonusFixed') }}</option>
        <option value="percentage">{{ t('admin.settings.payment.bonusPercentage') }}</option>
      </select>
      <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.bonusPreserveHint') }}</p>
    </div>
    <div v-if="mode === 'percentage'" class="space-y-3">
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.settings.payment.bonusPercentageHint') }}</p>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.bonusExample') }}</p>
      <div v-for="(tier, index) in tiers" :key="index" class="flex flex-wrap items-end gap-3">
        <label class="min-w-0 flex-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.payment.bonusThreshold') }}
          <input v-model.number="tier.min_amount" type="number" min="0" max="9999999999.99" step="0.01" required class="input mt-1 tabular-nums" data-testid="bonus-threshold" />
        </label>
        <label class="min-w-0 flex-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.payment.bonusPercent') }}
          <input v-model.number="tier.bonus_percent" type="number" min="0" max="1000" step="0.01" required class="input mt-1 tabular-nums" data-testid="bonus-percent" />
        </label>
        <button type="button" class="btn btn-secondary" :aria-label="t('admin.settings.payment.removeBonusTier', { index: index + 1 })" @click="tiers = tiers.filter((_, i) => i !== index)">
          {{ t('common.delete') }}
        </button>
      </div>
      <p v-if="tiers.length === 0" class="text-sm text-gray-500">{{ t('admin.settings.payment.bonusNoTiers') }}</p>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="tiers.length >= 20" data-testid="bonus-add-tier" @click="addTier">
        {{ t('admin.settings.payment.addBonusTier') }}
      </button>
    </div>
  </section>
</template>
