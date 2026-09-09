<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
        data-testid="recharge-checkout-dialog"
        @click.self="emit('close')"
      >
        <div class="relative w-full max-w-md overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900">
          <div class="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-dark-700">
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

          <div class="space-y-4 p-5">
            <div class="rounded-xl bg-gray-50 px-4 py-4 dark:bg-dark-800">
              <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.paymentInfo') }}</p>
              <p class="mt-1 text-3xl font-bold text-gray-900 dark:text-white">{{ payAmountLabel }}</p>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('payment.creditedBalance') }} {{ creditAmountLabel }}
              </p>
              <p v-if="feeRate > 0" class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                {{ t('payment.fee') }} ({{ feeRate }}%): {{ feeAmountLabel }}
              </p>
              <p v-if="multiplier !== 1" class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                {{ t('payment.rechargeRatePreview', { currency, usd: multiplier.toFixed(2) }) }}
              </p>
            </div>

            <div v-if="showLaneToggle" class="flex rounded-xl bg-gray-100 p-1 dark:bg-dark-800">
              <button
                type="button"
                data-testid="pay-lane-rmb"
                :disabled="submitting"
                class="flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all"
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
                class="flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all"
                :class="lane === 'usdt'
                  ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white'
                  : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
                @click="emit('update:lane', 'usdt')"
              >
                {{ t('payment.usdtPay') }}
              </button>
            </div>

            <div>
              <p class="mb-2 text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('payment.paymentMethod') }}</p>
              <div v-if="visibleMethods.length === 0" class="rounded-xl border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
                {{ lane === 'usdt' ? t('payment.noUsdtMethods') : t('payment.amountNoMethod') }}
              </div>
              <div v-else class="space-y-2">
                <button
                  v-for="method in visibleMethods"
                  :key="method.type"
                  type="button"
                  :data-testid="`checkout-method-${method.type}`"
                  :disabled="!method.available || submitting"
                  :aria-pressed="selected === method.type"
                  :class="[
                    'btn w-full justify-center py-3 text-base font-medium',
                    methodButtonClass(method.type),
                    selected === method.type ? 'ring-2 ring-primary-500 ring-offset-2 dark:ring-offset-dark-900' : '',
                  ]"
                  @click="selectMethod(method)"
                >
                  <img :src="methodIcon(method.type)" :alt="methodLabel(method)" class="h-6 w-6 object-contain" />
                  <span>{{ methodLabel(method) }}</span>
                </button>
              </div>
              <div v-if="visibleMethods.some((method) => method.type === 'stripe')" class="mt-3 flex flex-wrap items-center gap-2 text-xs text-gray-400 dark:text-gray-500">
                <span>{{ t('payment.supportedMethods') }}</span>
                <img :src="alipayIcon" alt="" class="h-5 w-5 object-contain" />
                <img :src="wxpayIcon" alt="" class="h-5 w-5 object-contain" />
              </div>
            </div>

            <button
              type="button"
              data-testid="checkout-confirm"
              :disabled="!canConfirm"
              :class="['btn w-full justify-center py-3 text-base font-medium', confirmButtonClass]"
              @click="confirmSelected"
            >
              <span v-if="submitting" class="flex items-center justify-center gap-2">
                <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                {{ t('common.processing') }}
              </span>
              <span v-else>{{ t('payment.createOrder') }} {{ payAmountLabel }}</span>
            </button>

            <p v-if="error" class="text-xs text-amber-600 dark:text-amber-300">{{ error }}</p>
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
import { paymentMethodLane, type PaymentMethodLane } from './rechargePackages'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'
import airwallexIcon from '@/assets/icons/airwallex.svg'
import paymentIcon from '@/assets/icons/payment.svg'

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
const selectedMethod = computed(() =>
  visibleMethods.value.find((method) => method.type === props.selected),
)
const canConfirm = computed(() =>
  !props.submitting && !!selectedMethod.value?.available,
)
const confirmButtonClass = computed(() =>
  selectedMethod.value ? methodButtonClass(selectedMethod.value.type) : 'btn-primary',
)

function selectMethod(method: PaymentMethodOption) {
  if (!method.available || props.submitting) return
  emit('select', method.type)
}

function confirmSelected() {
  if (!canConfirm.value || !props.selected) return
  emit('confirm', props.selected)
}

function methodLabel(method: PaymentMethodOption): string {
  return method.display_name || t(`payment.methods.${method.type}`, method.type)
}

function methodIcon(type: string): string {
  if (isBuiltInAlipayMethod(type)) return alipayIcon
  if (isBuiltInWxpayMethod(type)) return wxpayIcon
  if (type === 'airwallex') return airwallexIcon
  if (type === 'stripe') return stripeIcon
  return paymentIcon
}

function methodButtonClass(type: string): string {
  if (isBuiltInAlipayMethod(type)) return 'btn-alipay'
  if (isBuiltInWxpayMethod(type)) return 'btn-wxpay'
  if (type === 'stripe') return 'btn-stripe'
  if (type === 'airwallex') return 'btn-airwallex'
  if (paymentMethodLane(type) === 'usdt') {
    return 'bg-dark-800 text-white hover:bg-dark-700 dark:bg-dark-700 dark:hover:bg-dark-600'
  }
  return 'btn-primary'
}
</script>
