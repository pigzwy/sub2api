<template>
  <div class="overflow-x-auto" data-testid="plaza-price-sheet">
    <p class="px-5 py-3 text-sm text-gray-600 dark:text-dark-300" data-testid="plaza-price-note">
      {{ t('modelPlaza.catalog.displayPriceNote') }}
    </p>
    <table class="w-full min-w-[720px] table-auto border-collapse text-sm">
      <thead>
        <tr class="border-b border-gray-200 text-left text-xs font-medium text-gray-500 dark:border-dark-700 dark:text-dark-400">
          <th class="px-5 py-3 font-medium">{{ t('modelPlaza.table.modelId') }}</th>
          <template v-if="mediaOnly">
            <th colspan="5" class="px-4 py-3 font-medium">{{ mediaColumnLabel }}</th>
          </template>
          <template v-else>
            <th class="px-4 py-3 font-medium">{{ t('modelPlaza.table.inputPrice') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('modelPlaza.table.outputPrice') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('modelPlaza.table.cacheCreate') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('modelPlaza.table.cacheReadCol') }}</th>
            <th class="px-4 py-3 pr-5 text-right font-medium">{{ t('modelPlaza.catalog.inputSavings') }}</th>
          </template>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="row in rows"
          :key="row.name"
          class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
        >
          <td class="px-5 py-4 align-middle">
            <div class="flex items-center gap-1.5">
              <span class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
              <button
                type="button"
                class="rounded p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-white"
                :title="t('modelPlaza.table.copyModel')"
                @click="copyName(row.name)"
              >
                <Icon name="copy" size="xs" />
              </button>
            </div>
          </td>
          <template v-if="row.kind === 'token'">
            <td class="px-4 py-4 align-middle">
              <PriceStack :primary="row.inputPrimary" :official="row.inputOfficial" :unit="row.unit" />
            </td>
            <td class="px-4 py-4 align-middle">
              <PriceStack :primary="row.outputPrimary" :official="row.outputOfficial" :unit="row.unit" />
            </td>
            <td class="px-4 py-4 align-middle">
              <PriceStack :primary="row.cacheWritePrimary" :official="row.cacheWriteOfficial" :unit="row.unit" />
            </td>
            <td class="px-4 py-4 align-middle">
              <PriceStack :primary="row.cacheReadPrimary" :official="row.cacheReadOfficial" :unit="row.unit" />
            </td>
            <td class="px-4 py-4 pr-5 text-right align-middle">
              <span
                v-if="row.savings != null"
                class="inline-flex rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300"
              >
                {{ t('modelPlaza.table.savePercent', { percent: row.savings }) }}
              </span>
              <span v-else class="text-xs text-gray-400">-</span>
            </td>
          </template>
          <td v-else colspan="5" class="px-4 py-4 align-middle" data-testid="plaza-media-price">
            <div v-if="row.prices.length" class="flex flex-wrap items-center gap-2">
              <span
                v-for="(price, index) in row.prices"
                :key="`${row.name}-${index}`"
                class="inline-flex items-baseline gap-1"
              >
                <span v-if="price.label" class="text-xs text-gray-400 dark:text-dark-500">{{ price.label }}</span>
                <span class="text-base font-semibold tabular-nums text-amber-600 dark:text-amber-300">{{ price.amount }}</span>
                <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ row.unit }}</span>
              </span>
            </div>
            <span v-else class="text-gray-400 dark:text-dark-500">-</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
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

const effectiveRate = computed(() => props.userRateMultiplier ?? props.rateMultiplier)
const mode = computed(() => props.priceMode ?? 'group')

const PriceStack = defineComponent({
  name: 'PriceStack',
  props: {
    primary: { type: String, required: true },
    official: { type: String, required: true },
    unit: { type: String, required: true },
  },
  setup(stackProps) {
    return () => {
      if (stackProps.primary === '-' && stackProps.official === '-') {
        return h('span', { class: 'text-gray-400 dark:text-dark-500' }, '-')
      }
      return h('div', { class: 'min-w-[7rem]' }, [
        h('p', { class: 'text-base font-semibold tabular-nums text-amber-600 dark:text-amber-300' }, [
          stackProps.primary === '-' ? '-' : `${stackProps.primary} `,
          h('span', { class: 'text-[11px] font-normal text-gray-400 dark:text-dark-500' }, stackProps.unit),
        ]),
        stackProps.official !== '-'
          ? h('p', { class: 'mt-0.5 text-xs text-gray-400 dark:text-dark-500' }, t('modelPlaza.table.officialLine', { amount: stackProps.official }))
          : null,
      ])
    }
  },
})

interface TokenSheetRow {
  kind: 'token'
  name: string
  unit: string
  inputPrimary: string
  inputOfficial: string
  outputPrimary: string
  outputOfficial: string
  cacheWritePrimary: string
  cacheWriteOfficial: string
  cacheReadPrimary: string
  cacheReadOfficial: string
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

function pair(group: number | null, official: number | null): { primary: string; official: string } {
  const groupText = formatYuan(group)
  const officialText = formatYuan(official)
  if (mode.value === 'official') {
    return { primary: officialText, official: '-' }
  }
  return { primary: groupText, official: officialText }
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
      const input = pair(gIn, oIn)
      const output = pair(gOut, oOut)
      const cacheWrite = pair(gCw, oCw)
      const cacheRead = pair(gCr, oCr)
      return {
        kind: 'token',
        name: m.name,
        unit,
        inputPrimary: input.primary,
        inputOfficial: input.official,
        outputPrimary: output.primary,
        outputOfficial: output.official,
        cacheWritePrimary: cacheWrite.primary,
        cacheWriteOfficial: cacheWrite.official,
        cacheReadPrimary: cacheRead.primary,
        cacheReadOfficial: cacheRead.official,
        savings: mode.value === 'group' ? savingsPercent(gIn, oIn) : null,
      }
    }),
)

const mediaOnly = computed(() => rows.value.length > 0 && rows.value.every((row) => row.kind === 'media'))

const mediaColumnLabel = computed(() => {
  const modes = new Set(props.models.map((model) => billingMode(model)))
  if (modes.size === 1 && modes.has(BILLING_MODE_IMAGE)) return t('modelPlaza.table.imagePrice')
  if (modes.size === 1 && modes.has(BILLING_MODE_VIDEO)) return t('modelPlaza.table.videoPrice')
  return t('modelPlaza.table.mediaPrice')
})

async function copyName(name: string) {
  try {
    await navigator.clipboard.writeText(name)
  } catch {
    // ignore clipboard failures in non-secure contexts
  }
}
</script>
