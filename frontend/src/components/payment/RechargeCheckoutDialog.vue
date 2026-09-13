<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
        data-testid="recharge-checkout-dialog"
        @click.self="emit('close')"
      >
        <div class="relative w-full max-w-lg overflow-hidden rounded-3xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900">
          <div class="flex items-center justify-between px-6 pb-1 pt-6">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.selectPaymentMethod') }}</h3>
            <button
              type="button"
              class="rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              :aria-label="t('common.close')"
              @click="emit('close')"
            >
              <Icon name="x" size="md" />
            </button>
          </div>

          <div class="space-y-6 px-6 pb-2 pt-5">
            <div class="rounded-2xl bg-gray-50 px-5 py-5 dark:bg-dark-800">
              <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.paymentInfo') }}</p>
              <p v-if="quoteLoading" class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.quoteLoading') }}</p>
              <template v-else>
                <p class="mt-2 text-4xl font-bold tracking-tight text-gray-900 dark:text-white">{{ payAmountLabel }}</p>
                <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('payment.creditedBalance') }} {{ creditAmountLabel }}
                </p>
                <p v-if="feeRate > 0" class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('payment.fee') }} ({{ feeRate }}%): {{ feeAmountLabel }}
                </p>
                <p v-if="fxRateLabel" class="mt-2 text-xs text-gray-400 dark:text-gray-500">{{ fxRateLabel }}</p>
                <p v-if="multiplier !== 1" class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('payment.rechargeRatePreview', { currency, usd: multiplier.toFixed(2) }) }}
                </p>
              </template>
            </div>

            <div v-if="showLaneToggle" class="flex rounded-2xl bg-gray-100 p-1.5 dark:bg-dark-800">
              <button
                type="button"
                data-testid="pay-lane-rmb"
                :disabled="submitting"
                class="flex-1 rounded-xl px-3 py-2.5 text-sm font-medium transition-all"
                :class="lane === 'rmb'
                  ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white'
                  : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
                @click="emit('update:lane', 'rmb')"
              >
                {{ t('payment.rmbPay') }}
              </button>
              <button
                type="button"
                data-testid="pay-lane-usdt"
                :disabled="submitting"
                class="flex-1 rounded-xl px-3 py-2.5 text-sm font-medium transition-all"
                :class="lane === 'usdt'
                  ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white'
                  : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
                @click="emit('update:lane', 'usdt')"
              >
                {{ t('payment.usdtPay') }}
              </button>
            </div>

            <div>
              <p class="mb-3 text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('payment.paymentMethod') }}</p>
              <div v-if="visibleMethods.length === 0" class="rounded-xl border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
                {{ lane === 'usdt' ? t('payment.noUsdtMethods') : t('payment.amountNoMethod') }}
              </div>
              <div v-else class="space-y-3">
                <button
                  v-for="method in visibleMethods"
                  :key="method.type"
                  type="button"
                  :data-testid="`checkout-method-${method.type}`"
                  data-surface="transparent"
                  :disabled="!method.available || submitting || quoteLoading"
                  :class="CHECKOUT_METHOD_BUTTON_CLASS"
                  @click="payWithMethod(method)"
                >
                  <span v-if="submitting && selected === method.type" class="flex items-center justify-center gap-2">
                    <span class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"></span>
                    {{ t('common.processing') }}
                  </span>
                  <template v-else>
                    <img :src="methodIcon(method.type)" :alt="methodLabel(method)" class="h-8 w-8 object-contain" />
                    <span>{{ methodLabel(method) }}</span>
                  </template>
                </button>
              </div>
            </div>

            <p v-if="error" class="text-xs text-amber-600 dark:text-amber-300">{{ error }}</p>
          </div>

          <div
            data-testid="supported-methods"
            :data-lane="lane"
            class="flex flex-wrap items-center gap-2 px-6 pb-6 pt-4 text-xs font-medium text-gray-500 dark:text-gray-300"
          >
            <span>{{ t('payment.supportedMethods') }}</span>
            <span
              v-for="brand in supportedBrands"
              :key="brand.alt"
              class="inline-flex h-6 items-center rounded-md bg-white px-1 shadow-sm"
            >
              <img
                :src="brand.src"
                :alt="brand.alt"
                :class="brand.class"
              />
            </span>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { isBuiltInAlipayMethod, isBuiltInWxpayMethod } from './providerConfig'
import type { PaymentMethodOption } from './PaymentMethodSelector.vue'
import type { PaymentMethodLane } from './rechargePackages'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'
import airwallexIcon from '@/assets/icons/airwallex.svg'
import infiniIcon from '@/assets/icons/infini.svg'
import paymentIcon from '@/assets/icons/payment.svg'
import visaIcon from '@/assets/icons/visa.svg'
import mastercardIcon from '@/assets/icons/mastercard.svg'
import applePayIcon from '@/assets/icons/apple-pay.svg'
import dollarIcon from '@/assets/icons/dollar.svg'

const CHECKOUT_METHOD_BUTTON_CLASS = [
  'inline-flex w-full items-center justify-center gap-3 rounded-2xl border border-gray-200',
  'bg-transparent px-4 py-4 text-base font-medium text-gray-900 shadow-none',
  'transition-colors hover:bg-white/80',
  'focus:outline-none focus:ring-2 focus:ring-primary-500/40 focus:ring-offset-2',
  'disabled:cursor-not-allowed disabled:opacity-50',
  'dark:border-white/15 dark:text-white dark:hover:bg-white/5',
].join(' ')

const props = defineProps<{
  open: boolean
  payAmountLabel: string
  creditAmountLabel: string
  feeAmountLabel: string
  feeRate: number
  multiplier: number
  currency: string
  selected: string
  lane: PaymentMethodLane
  rmbMethods: PaymentMethodOption[]
  usdtMethods: PaymentMethodOption[]
  submitting: boolean
  error?: string
  quoteLoading?: boolean
  fxRateLabel?: string
}>()

const emit = defineEmits<{
  close: []
  select: [type: string]
  confirm: [type: string]
  'update:lane': [lane: PaymentMethodLane]
}>()

const { t } = useI18n()

const showLaneToggle = computed(() => props.rmbMethods.length > 0 && props.usdtMethods.length > 0)
const visibleMethods = computed(() => (props.lane === 'usdt' ? props.usdtMethods : props.rmbMethods))
const rmbBrands = [
  { src: alipayIcon, alt: 'Alipay', class: 'h-5 w-5 object-contain' },
  { src: wxpayIcon, alt: 'WeChat Pay', class: 'h-5 w-5 object-contain' },
  { src: visaIcon, alt: 'Visa', class: 'h-3.5 w-auto object-contain' },
  { src: mastercardIcon, alt: 'Mastercard', class: 'h-5 w-auto object-contain' },
  { src: applePayIcon, alt: 'Apple Pay', class: 'h-5 w-5 object-contain' },
  { src: dollarIcon, alt: 'USD', class: 'h-5 w-5 object-contain' },
]
const supportedBrands = computed(() => {
  const types = new Set(visibleMethods.value.map((method) => method.type.toLowerCase()))
  if (props.lane === 'usdt') {
    return types.has('infini') ? [{ src: infiniIcon, alt: 'Infini', class: 'h-5 w-5 object-contain' }] : []
  }
  return rmbBrands.filter((brand) => {
    if (brand.alt === 'Alipay') return types.has('alipay') || types.has('alipay_direct')
    if (brand.alt === 'WeChat Pay') return types.has('wxpay') || types.has('wxpay_direct')
    if (['Visa', 'Mastercard', 'Apple Pay'].includes(brand.alt)) return types.has('stripe')
    if (brand.alt === 'USD') return types.has('stripe') || types.has('airwallex')
    return false
  })
})

function payWithMethod(method: PaymentMethodOption) {
  if (!method.available || props.submitting || props.quoteLoading) return
  emit('select', method.type)
  emit('confirm', method.type)
}

function methodLabel(method: PaymentMethodOption): string {
  return method.display_name || t(`payment.methods.${method.type}`, method.type)
}

function methodIcon(type: string): string {
  if (isBuiltInAlipayMethod(type)) return alipayIcon
  if (isBuiltInWxpayMethod(type)) return wxpayIcon
  if (type === 'airwallex') return airwallexIcon
  if (type === 'infini') return infiniIcon
  if (type === 'stripe') return stripeIcon
  return paymentIcon
}
</script>
