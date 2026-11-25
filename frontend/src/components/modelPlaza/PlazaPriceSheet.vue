<template>
  <div data-testid="plaza-price-sheet">
    <div class="overflow-x-auto">
      <table class="w-full min-w-[760px] table-auto border-collapse text-sm">
        <thead>
          <tr class="border-b border-gray-200 bg-gray-50 text-left text-[13px] text-gray-500 dark:border-dark-700 dark:bg-dark-700/40 dark:text-dark-300">
            <th class="w-[26%] px-5 py-3.5 font-medium">{{ t('modelPlaza.table.modelId') }}</th>
            <template v-if="mediaOnly">
              <th :colspan="priceColumnCount" class="px-4 py-3.5 font-medium">{{ mediaColumnLabel }}</th>
            </template>
            <template v-else>
              <th class="px-4 py-3.5 font-medium">{{ t('modelPlaza.table.inputPrice') }}</th>
              <th class="px-4 py-3.5 font-medium">{{ t('modelPlaza.table.outputPrice') }}</th>
              <th class="px-4 py-3.5 font-medium">{{ t('modelPlaza.table.cacheCreate') }}</th>
              <th class="px-4 py-3.5 font-medium">{{ t('modelPlaza.table.cacheReadCol') }}</th>
              <th v-if="showSavings" class="px-4 py-3.5 pr-5 font-medium">{{ t('modelPlaza.catalog.inputSavings') }}</th>
            </template>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in rows"
            :key="row.name"
            class="border-b border-gray-100 transition-colors last:border-b-0 hover:bg-gray-50/70 dark:border-dark-700/60 dark:hover:bg-dark-700/30"
          >
            <td class="px-5 py-[1.125rem] align-middle">
              <div class="flex items-center gap-1">
                <span class="whitespace-nowrap text-[15px] font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
                <button
                  type="button"
                  class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-orange-400 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-white"
                  :title="t('modelPlaza.table.copyModel')"
                  :aria-label="t('modelPlaza.table.copyModel')"
                  data-testid="plaza-copy-model"
                  @click="copyToClipboard(row.name)"
                >
                  <Icon name="copy" size="sm" aria-hidden="true" />
                </button>
              </div>
            </td>
            <template v-if="row.kind === 'token'">
              <td class="px-4 py-[1.125rem] align-middle">
                <PriceStack v-bind="row.input" :unit="row.unit" />
              </td>
              <td class="px-4 py-[1.125rem] align-middle">
                <PriceStack v-bind="row.output" :unit="row.unit" />
              </td>
              <td class="px-4 py-[1.125rem] align-middle">
                <PriceStack v-bind="row.cacheWrite" :unit="row.unit" />
              </td>
              <td class="px-4 py-[1.125rem] align-middle">
                <PriceStack v-bind="row.cacheRead" :unit="row.unit" />
              </td>
              <td v-if="showSavings" class="px-4 py-[1.125rem] pr-5 align-middle">
                <span
                  v-if="row.savings != null"
                  class="inline-flex whitespace-nowrap rounded-full bg-emerald-50 px-2.5 py-1 text-[13px] font-semibold leading-4 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300"
                >
                  {{ t('modelPlaza.table.savePercent', { percent: row.savings }) }}
                </span>
                <span v-else class="text-gray-300 dark:text-dark-500">-</span>
              </td>
            </template>
            <td v-else :colspan="priceColumnCount" class="px-4 py-[1.125rem] align-middle" data-testid="plaza-media-price">
              <div v-if="row.prices.length" class="flex flex-wrap items-center gap-x-5 gap-y-1.5">
                <span
                  v-for="(price, index) in row.prices"
                  :key="`${row.name}-${index}`"
                  class="inline-flex items-baseline gap-1"
                >
                  <span v-if="price.label" class="text-xs text-gray-500 dark:text-dark-400">{{ price.label }}</span>
                  <span class="text-[17px] font-semibold tabular-nums text-orange-700 dark:text-orange-300">{{ price.amount }}</span>
                  <span class="text-xs text-gray-400 dark:text-dark-500">{{ row.unit }}</span>
                </span>
              </div>
              <span v-else class="text-gray-300 dark:text-dark-500">-</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p
      class="border-t border-gray-100 px-5 py-3 text-xs leading-5 text-gray-400 dark:border-dark-700 dark:text-dark-400"
      data-testid="plaza-price-note"
    >
      {{ t('modelPlaza.catalog.displayPriceNote') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import type { PlazaModel } from '@/api/modelPlaza'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode,
} from '@/constants/channel'
import {
  comparePlazaModels,
  formatYuan,
  groupYuan,
  officialYuan,
  requestGroupYuan,
  savingsPercent,
} from './plazaCatalog'

const props = defineProps<{
  models: PlazaModel[]
  rateMultiplier: number
  userRateMultiplier?: number | null
  imageRateIndependent?: boolean
  imageRateMultiplier?: number | null
  videoRateIndependent?: boolean
  videoRateMultiplier?: number | null
  priceMode?: 'group' | 'official'
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const effectiveRate = computed(() => props.userRateMultiplier ?? props.rateMultiplier)
const mode = computed(() => props.priceMode ?? 'group')

const PriceStack = defineComponent({
  name: 'PriceStack',
  props: {
    primary: { type: String, required: true },
    official: { type: String, required: true },
    unit: { type: String, required: true },
    /** 分组价低于官方价时，官方价加删除线，突出折后价。 */
    discounted: { type: Boolean, default: false },
  },
  setup(stackProps) {
    return () => {
      if (stackProps.primary === '-' && stackProps.official === '-') {
        return h('span', { class: 'text-gray-400 dark:text-dark-500' }, '-')
      }
      return h('div', { class: 'min-w-[7.5rem]' }, [
        h('p', { class: 'whitespace-nowrap text-[17px] font-semibold leading-6 tabular-nums text-orange-700 dark:text-orange-300' }, [
          stackProps.primary === '-' ? '-' : `${stackProps.primary} `,
          h('span', { class: 'text-xs font-normal text-gray-400 dark:text-dark-500' }, stackProps.unit),
        ]),
        stackProps.official !== '-'
          ? h(
              'p',
              {
                class: [
                  'mt-0.5 whitespace-nowrap text-xs tabular-nums text-gray-400 dark:text-dark-500',
                  stackProps.discounted ? 'line-through' : '',
                ],
              },
              t('modelPlaza.table.officialLine', { amount: stackProps.official }),
            )
          : null,
      ])
    }
  },
})

interface PricePair {
  primary: string
  official: string
  discounted: boolean
}

interface TokenSheetRow {
  kind: 'token'
  name: string
  unit: string
  input: PricePair
  output: PricePair
  cacheWrite: PricePair
  cacheRead: PricePair
  savings: number | null
}

interface MediaPrice {
  label: string
  amount: string
}

interface MediaSheetRow {
  kind: 'media'
  name: string
  unit: string
  prices: MediaPrice[]
}

type SheetRow = TokenSheetRow | MediaSheetRow

function billingMode(m: PlazaModel): BillingMode {
  return (m.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

function tokenRate(): number {
  return effectiveRate.value
}

function requestRate(m: PlazaModel): number {
  if (billingMode(m) === BILLING_MODE_VIDEO && props.videoRateIndependent) {
    return props.videoRateMultiplier ?? 1
  }
  if (billingMode(m) === BILLING_MODE_IMAGE && props.imageRateIndependent) {
    return props.imageRateMultiplier ?? 1
  }
  return effectiveRate.value
}

function mediaUnit(m: PlazaModel): string {
  if (billingMode(m) === BILLING_MODE_IMAGE) return t('modelPlaza.table.perUnitImage')
  if (billingMode(m) === BILLING_MODE_VIDEO) return t('modelPlaza.table.perUnitVideo')
  return t('modelPlaza.table.perUnitRequest')
}

/** 按图/按视频/按次只展示按次价或分辨率档，不拆进输入/输出列。 */
function mediaPrices(m: PlazaModel, rate: number): MediaPrice[] {
  if (mode.value === 'official') return []
  const tiers = (m.pricing?.intervals ?? []).filter((interval) => interval.per_request_price != null)
  if (tiers.length > 0) {
    return tiers.map((interval) => ({
      label: interval.tier_label || '',
      amount: formatYuan(requestGroupYuan(interval.per_request_price, rate)),
    }))
  }
  if (m.pricing?.per_request_price == null) return []
  return [{ label: '', amount: formatYuan(requestGroupYuan(m.pricing.per_request_price, rate)) }]
}

function pair(group: number | null, official: number | null): PricePair {
  const groupText = formatYuan(group)
  const officialText = formatYuan(official)
  if (mode.value === 'official') {
    return { primary: officialText, official: '-', discounted: false }
  }
  return {
    primary: groupText,
    official: officialText,
    discounted: group != null && official != null && group < official,
  }
}

const rows = computed<SheetRow[]>(() =>
  [...props.models]
    .sort(comparePlazaModels)
    .map((m): SheetRow => {
      const token = billingMode(m) === BILLING_MODE_TOKEN
      const rate = token ? tokenRate() : requestRate(m)
      if (!token) {
        return { kind: 'media', name: m.name, unit: mediaUnit(m), prices: mediaPrices(m, rate) }
      }
      const unit = t('modelPlaza.table.unitPerMillionShort')
      const gIn = groupYuan(m.pricing?.input_price, rate)
      // Official prices are per token, never comparable to per-image/request prices.
      const oIn = officialYuan(m.official_pricing?.input_price)
      const gOut = groupYuan(m.pricing?.output_price, rate)
      const oOut = officialYuan(m.official_pricing?.output_price)
      const gCw = groupYuan(m.pricing?.cache_write_price, rate)
      const oCw = officialYuan(m.official_pricing?.cache_write_price)
      const gCr = groupYuan(m.pricing?.cache_read_price, rate)
      const oCr = officialYuan(m.official_pricing?.cache_read_price)
      return {
        kind: 'token',
        name: m.name,
        unit,
        input: pair(gIn, oIn),
        output: pair(gOut, oOut),
        cacheWrite: pair(gCw, oCw),
        cacheRead: pair(gCr, oCr),
        savings: mode.value === 'group' ? savingsPercent(gIn, oIn) : null,
      }
    }),
)

const mediaOnly = computed(() => rows.value.length > 0 && rows.value.every((row) => row.kind === 'media'))

/** 没有任何模型比官方价便宜时（倍率 ≥ 1 或官方价模式），整列都是「-」，直接不显示这一列。 */
const showSavings = computed(() => rows.value.some((row) => row.kind === 'token' && row.savings != null))

/** 模型 ID 之外的价格列数：输入/输出/缓存创建/缓存读取，外加可选的对比列。 */
const priceColumnCount = computed(() => (showSavings.value ? 5 : 4))

const mediaColumnLabel = computed(() => {
  const modes = new Set(props.models.map((model) => billingMode(model)))
  if (modes.size === 1 && modes.has(BILLING_MODE_IMAGE)) return t('modelPlaza.table.imagePrice')
  if (modes.size === 1 && modes.has(BILLING_MODE_VIDEO)) return t('modelPlaza.table.videoPrice')
  return t('modelPlaza.table.mediaPrice')
})
</script>
