<template>
  <div data-testid="recharge-package-grid" class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
    <article
      v-for="pkg in packages"
      :key="pkg.id"
      :data-testid="`recharge-package-${pkg.amount}`"
      :class="[
        'relative flex flex-col rounded-2xl border p-5 shadow-card',
        pkg.badge === 'bestValue'
          ? 'border-amber-400/70 bg-white dark:border-amber-400/40 dark:bg-dark-800/80'
          : 'border-gray-100 bg-white dark:border-dark-700/70 dark:bg-dark-800/60',
      ]"
    >
      <span
        v-if="pkg.badge"
        :class="[
          'absolute right-4 top-4 rounded-full px-2 py-0.5 text-[11px] font-semibold',
          pkg.badge === 'bestValue'
            ? 'bg-amber-500 text-white'
            : 'bg-amber-200 text-amber-900 dark:bg-amber-300/90 dark:text-amber-950',
        ]"
      >
        {{ pkg.badge === 'bestValue' ? t('payment.bestValue') : t('payment.popular') }}
      </span>
      <h3 class="pr-16 text-lg font-semibold text-gray-900 dark:text-white">
        {{ packageName(pkg.id) }}
      </h3>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ packageDesc(pkg.id) }}
      </p>
      <div class="mt-5 flex flex-wrap items-end gap-2">
        <p class="text-3xl font-bold tracking-tight text-gray-900 dark:text-white">
          <span class="mr-0.5 text-xl font-semibold text-gray-400 dark:text-gray-500">{{ currencySymbol(currency) }}</span>{{ formatIntegerAmount(pkg.amount) }}
        </p>
        <span
          v-if="bonusOf(pkg) > 0"
          class="mb-1 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-400/15 dark:text-amber-200"
        >
          {{ t('payment.bonusTag', { amount: formatUsd(bonusOf(pkg)) }) }}
        </span>
      </div>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
        {{ t('payment.getCredit', { amount: formatUsd(creditOf(pkg)) }) }}
      </p>
      <ul class="mt-4 space-y-2 text-sm text-gray-600 dark:text-gray-300">
        <li class="flex items-start gap-2">
          <Icon name="check" size="sm" class="mt-0.5 shrink-0 text-emerald-500" />
          <span>{{ t('payment.getCredit', { amount: formatUsd(creditOf(pkg)) }) }}</span>
        </li>
        <li class="flex items-start gap-2">
          <Icon name="check" size="sm" class="mt-0.5 shrink-0 text-emerald-500" />
          <span>{{ t('payment.neverExpires') }}</span>
        </li>
        <li class="flex items-start gap-2">
          <Icon name="check" size="sm" class="mt-0.5 shrink-0 text-emerald-500" />
          <span>{{ t('payment.allModels') }}</span>
        </li>
      </ul>
      <button
        type="button"
        :class="[
          'btn mt-6 w-full py-2.5 text-sm font-medium',
          pkg.badge === 'bestValue' ? 'btn-warning' : 'btn-primary',
        ]"
        @click="emit('select', pkg.amount)"
      >
        {{ t('payment.rechargeNow') }}
      </button>
    </article>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { currencySymbol } from './currency'
import {
  creditedRechargeAmount,
  rechargeBonusAmount,
  type RechargePackage,
  type RechargePackageId,
} from './rechargePackages'

const props = defineProps<{
  packages: RechargePackage[]
  multiplier: number
  currency: string
}>()

const emit = defineEmits<{
  select: [amount: number]
}>()

const { t } = useI18n()

function creditOf(pkg: RechargePackage): number {
  return creditedRechargeAmount(pkg.amount, props.multiplier)
}

function bonusOf(pkg: RechargePackage): number {
  return rechargeBonusAmount(pkg.amount, props.multiplier)
}

function formatIntegerAmount(amount: number): string {
  return amount.toLocaleString(undefined, { maximumFractionDigits: 0 })
}

function formatUsd(amount: number): string {
  return `$${amount.toFixed(2)}`
}

function packageName(id: RechargePackageId): string {
  switch (id) {
    case 'starter': return t('payment.packages.starter.name')
    case 'standard': return t('payment.packages.standard.name')
    case 'advanced': return t('payment.packages.advanced.name')
    case 'pro': return t('payment.packages.pro.name')
    case 'team': return t('payment.packages.team.name')
    case 'business': return t('payment.packages.business.name')
    case 'premium': return t('payment.packages.premium.name')
    case 'enterprise': return t('payment.packages.enterprise.name')
  }
}

function packageDesc(id: RechargePackageId): string {
  switch (id) {
    case 'starter': return t('payment.packages.starter.desc')
    case 'standard': return t('payment.packages.standard.desc')
    case 'advanced': return t('payment.packages.advanced.desc')
    case 'pro': return t('payment.packages.pro.desc')
    case 'team': return t('payment.packages.team.desc')
    case 'business': return t('payment.packages.business.desc')
    case 'premium': return t('payment.packages.premium.desc')
    case 'enterprise': return t('payment.packages.enterprise.desc')
  }
}
</script>
