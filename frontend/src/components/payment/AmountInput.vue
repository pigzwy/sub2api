<template>
  <div class="space-y-5">
    <!-- Quick Amount Buttons -->
    <div v-if="filteredAmounts.length > 0">
      <label class="mb-3 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.quickAmounts') }}
      </label>
      <div data-testid="quick-amount-grid" class="grid grid-cols-3 gap-2.5 sm:gap-3">
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          :data-testid="`quick-amount-${amt}`"
          :aria-pressed="modelValue === amt"
          :class="[
            'group relative overflow-hidden rounded-xl border px-2 py-3.5 text-center transition-all duration-200 sm:px-3',
            modelValue === amt
              ? 'border-primary-500 bg-primary-50 shadow-sm ring-2 ring-primary-500/15 dark:border-primary-400 dark:bg-primary-500/15 dark:ring-primary-400/20'
              : 'border-gray-200 bg-gray-50/80 hover:-translate-y-0.5 hover:border-primary-300 hover:bg-white dark:border-dark-600 dark:bg-dark-800/80 dark:hover:border-primary-500/40 dark:hover:bg-dark-700',
          ]"
          @click="selectAmount(amt)"
        >
          <span
            v-if="modelValue === amt"
            class="absolute right-1.5 top-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-primary-500 text-white dark:bg-primary-400"
            aria-hidden="true"
          >
            <Icon name="check" size="xs" :stroke-width="2.4" />
          </span>
          <span class="flex items-baseline justify-center gap-0.5">
            <span
              :class="[
                'text-xs font-medium',
                modelValue === amt
                  ? 'text-primary-500 dark:text-primary-300'
                  : 'text-gray-400 dark:text-gray-500',
              ]"
            >$</span>
            <span
              :class="[
                'text-lg font-semibold tabular-nums tracking-tight',
                modelValue === amt
                  ? 'text-primary-700 dark:text-primary-100'
                  : 'text-gray-800 dark:text-gray-100',
              ]"
            >
              {{ formatQuickAmount(amt) }}
            </span>
          </span>
        </button>
      </div>
    </div>

    <!-- Custom Amount Input -->
    <div>
      <label class="mb-3 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.customAmount') }}
      </label>
      <div
        :class="[
          'relative rounded-xl transition-shadow',
          isCustomAmount ? 'ring-2 ring-primary-500/20 dark:ring-primary-400/20' : '',
        ]"
      >
        <span class="absolute left-3.5 top-1/2 -translate-y-1/2 text-sm font-medium text-gray-400 dark:text-dark-400">
          $
        </span>
        <input
          type="text"
          inputmode="decimal"
          data-testid="custom-amount-input"
          :value="customText"
          :placeholder="placeholderText"
          :class="[
            'input w-full py-3 pl-8 pr-4',
            isCustomAmount
              ? 'border-primary-500 focus:border-primary-500 dark:border-primary-400'
              : '',
          ]"
          @input="handleInput"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t, locale } = useI18n()

const customText = ref('')

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const isCustomAmount = computed(() =>
  props.modelValue !== null && !filteredAmounts.value.includes(props.modelValue)
)

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function localeTag(): string {
  const raw = locale as unknown
  const value = typeof raw === 'string'
    ? raw
    : (raw && typeof raw === 'object' && 'value' in raw ? String((raw as { value?: string }).value || '') : '')
  return value.startsWith('zh') ? 'zh-CN' : 'en-US'
}

function formatQuickAmount(amt: number): string {
  return amt.toLocaleString(localeTag(), { maximumFractionDigits: 2 })
}

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  if (!AMOUNT_PATTERN.test(val)) return
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  if (!isNaN(num) && num > 0) {
    emit('update:modelValue', num)
  } else {
    emit('update:modelValue', null)
  }
}

watch(() => props.modelValue, (v) => {
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>
