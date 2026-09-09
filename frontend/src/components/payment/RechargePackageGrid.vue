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
        {{ localizedPackageName(pkg, localeCode) }}
      </h3>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ localizedPackageDescription(pkg, localeCode) }}
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
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { currencySymbol } from './currency'
import {
  localizedPackageDescription,
  localizedPackageName,
  packageBonusAmount,
  packageCreditAmount,
  type RechargePackage,
} from './rechargePackages'

const props = defineProps<{
  packages: RechargePackage[]
  multiplier: number
  currency: string
}>()

const emit = defineEmits<{
  select: [amount: number]
}>()

const { t, locale } = useI18n()
const localeCode = computed(() => {
  if (typeof locale === 'string') return locale
  if (locale && typeof locale === 'object' && 'value' in locale) {
    return String((locale as { value?: string }).value || '')
  }
  return ''
})

function creditOf(pkg: RechargePackage): number {
  return packageCreditAmount(pkg, props.packages, props.multiplier)
}

function bonusOf(pkg: RechargePackage): number {
  return packageBonusAmount(pkg, props.packages, props.multiplier)
}

function formatIntegerAmount(amount: number): string {
  return amount.toLocaleString(undefined, { maximumFractionDigits: 0 })
}

function formatUsd(amount: number): string {
  return `$${amount.toFixed(2)}`
}
</script>
