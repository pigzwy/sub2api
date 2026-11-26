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
          <template v-for="row in rows" :key="row.name">
            <tr
              class="border-b border-gray-100 transition-colors last:border-b-0 hover:bg-gray-50/70 dark:border-dark-700/60 dark:hover:bg-dark-700/30"
              data-testid="plaza-model-row"
            >
              <td class="px-5 py-[1.125rem] align-middle">
                <div class="flex flex-wrap items-center gap-x-1.5 gap-y-1.5">
                  <span class="inline-flex items-center gap-1">
                    <span class="whitespace-nowrap text-[15px] font-medium text-gray-800 dark:text-gray-100">{{ row.name }}</span>
                    <button
                      type="button"
                      class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-white"
                      :title="t('modelPlaza.table.copyModel')"
                      :aria-label="t('modelPlaza.table.copyModel')"
                      data-testid="plaza-copy-model"
                      @click="copyToClipboard(row.name)"
                    >
                      <Icon name="copy" size="sm" aria-hidden="true" />
                    </button>
                  </span>
                  <template v-if="row.kind === 'token'">
                    <!-- 阶梯价 / 分时价：点开在下方逐档列出 -->
                    <button
                      v-if="row.details.length"
                      type="button"
                      class="inline-flex items-center gap-1 whitespace-nowrap rounded-full border border-primary-200 bg-primary-50 px-2 py-0.5 text-[11px] font-medium leading-4 text-primary-700 transition-colors hover:border-primary-300 hover:bg-primary-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-primary-300/30 dark:bg-primary-300/15 dark:text-primary-300 dark:hover:border-primary-300/50 dark:hover:bg-primary-300/25"
                      :aria-expanded="isExpanded(row.name)"
                      :aria-label="t('modelPlaza.table.detailToggle', { model: row.name })"
                      data-testid="plaza-detail-toggle"
                      @click="toggle(row.name)"
                    >
                      <span v-if="row.tierCount">{{ t('modelPlaza.table.tierToggle', { count: row.tierCount }) }}</span>
                      <span v-if="row.tierCount && row.periodCount" aria-hidden="true">·</span>
                      <span v-if="row.periodCount">{{ t('modelPlaza.table.timeToggle', { count: row.periodCount }) }}</span>
                      <Icon
                        name="chevronDown"
                        size="xs"
                        class="h-3 w-3 transition-transform"
                        :class="isExpanded(row.name) ? 'rotate-180' : ''"
                        aria-hidden="true"
                      />
                    </button>
                    <span
                      v-for="badge in row.badges"
                      :key="badge.label"
                      class="whitespace-nowrap rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium leading-4 text-gray-500 dark:bg-dark-700/70 dark:text-dark-300"
                      :title="badge.title"
                    >
                      {{ badge.label }}
                    </span>
                  </template>
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
                    <span class="text-[17px] font-semibold tabular-nums text-primary-700 dark:text-primary-300">{{ price.amount }}</span>
                    <span class="text-xs text-gray-500 dark:text-dark-400">{{ row.unit }}</span>
                  </span>
                </div>
                <span v-else class="text-gray-300 dark:text-dark-500">-</span>
              </td>
            </tr>

            <template v-if="row.kind === 'token' && isExpanded(row.name)">
              <tr
                v-for="detail in row.details"
                :key="`${row.name}:${detail.key}`"
                class="border-b border-gray-100 bg-gray-50/70 last:border-b-0 dark:border-dark-700/60 dark:bg-dark-700/20"
                data-testid="plaza-detail-row"
              >
                <td class="py-2.5 pl-9 pr-4 align-middle">
                  <span
                    class="inline-flex items-center whitespace-nowrap rounded-md bg-white px-2 py-0.5 text-xs font-medium tabular-nums text-gray-600 ring-1 ring-inset ring-gray-200 dark:bg-dark-800 dark:text-dark-200 dark:ring-dark-600"
                    :title="detail.hint"
                  >
                    {{ detail.label }}
                  </span>
                </td>
                <td class="px-4 py-2.5 align-middle">
                  <PriceStack v-bind="detail.input" :unit="row.unit" compact />
                </td>
                <td class="px-4 py-2.5 align-middle">
                  <PriceStack v-bind="detail.output" :unit="row.unit" compact />
                </td>
                <td class="px-4 py-2.5 align-middle">
                  <PriceStack v-bind="detail.cacheWrite" :unit="row.unit" compact />
                </td>
                <td class="px-4 py-2.5 align-middle">
                  <PriceStack v-bind="detail.cacheRead" :unit="row.unit" compact />
                </td>
                <td v-if="showSavings" class="px-4 py-2.5 pr-5"></td>
              </tr>
              <tr
                v-if="row.detailHint"
                class="border-b border-gray-100 bg-gray-50/70 last:border-b-0 dark:border-dark-700/60 dark:bg-dark-700/20"
              >
                <td
                  :colspan="priceColumnCount + 1"
                  class="pb-3 pl-9 pr-5 pt-0.5 text-xs leading-5 text-gray-500 dark:text-dark-400"
                  data-testid="plaza-detail-hint"
                >
                  {{ row.detailHint }}
                </td>
              </tr>
            </template>
          </template>
        </tbody>
      </table>
    </div>
    <p
      class="border-t border-gray-100 px-5 py-3 text-xs leading-5 text-gray-500 dark:border-dark-700 dark:text-dark-400"
      data-testid="plaza-price-note"
    >
      {{ t('modelPlaza.catalog.displayPriceNote') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import type { PlazaModel } from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  REASONING_EFFORT_LEVELS,
  type BillingMode,
} from '@/constants/channel'
import { resolveIntervalPrices } from '@/utils/pricing'
import {
  comparePlazaModels,
  formatPlazaTimeWindow,
  formatYuan,
  groupYuan,
  groupYuanExact,
  isCheaperPrice,
  officialYuan,
  officialYuanExact,
  plazaTierLabel,
  requestGroupYuan,
  savingsPercent,
  sortPlazaIntervals,
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
  /** 分组启用高峰时的倍率；token 价格下额外标出高峰价。缺省或为 1 时不显示。 */
  peakRateMultiplier?: number | null
  /** 高峰窗口描述，用于分时价说明里披露“与高峰重叠的部分再乘高峰倍率”。 */
  peakWindow?: string
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const effectiveRate = computed(() => props.userRateMultiplier ?? props.rateMultiplier)
const mode = computed(() => props.priceMode ?? 'group')

/** 高峰因子只叠加在 token 倍率上（与后端计费一致），按张/按次价不受影响。 */
const peakFactor = computed(() => {
  const factor = props.peakRateMultiplier
  return factor != null && factor > 0 && factor !== 1 ? factor : null
})

const PriceStack = defineComponent({
  name: 'PriceStack',
  props: {
    primary: { type: String, required: true },
    official: { type: String, required: true },
    unit: { type: String, required: true },
    /** 分组价低于官方价时，官方价加删除线，突出折后价。 */
    discounted: { type: Boolean, default: false },
    /** 价格下方的补充行：1 小时缓存写入价、高峰价。 */
    notes: { type: Array as PropType<string[]>, default: () => [] },
    /** 展开的分档行用小一号的字。 */
    compact: { type: Boolean, default: false },
  },
  setup(stackProps) {
    return () => {
      if (stackProps.primary === '-' && stackProps.official === '-') {
        return h('span', { class: 'text-gray-300 dark:text-dark-500' }, '-')
      }
      return h('div', { class: 'min-w-[7.5rem]' }, [
        h(
          'p',
          {
            class: [
              'whitespace-nowrap font-semibold tabular-nums text-primary-700 dark:text-primary-300',
              stackProps.compact ? 'text-sm leading-5' : 'text-[17px] leading-6',
            ],
          },
          [
            stackProps.primary === '-' ? '-' : `${stackProps.primary} `,
            h('span', { class: 'text-xs font-normal text-gray-500 dark:text-dark-400' }, stackProps.unit),
          ],
        ),
        stackProps.official !== '-'
          ? h(
              'p',
              {
                class: [
                  'mt-0.5 whitespace-nowrap text-xs tabular-nums text-gray-500 dark:text-dark-400',
                  stackProps.discounted ? 'line-through' : '',
                ],
              },
              t('modelPlaza.table.officialLine', { amount: stackProps.official }),
            )
          : null,
        ...stackProps.notes.map((note) =>
          h('p', { class: 'mt-0.5 whitespace-nowrap text-xs tabular-nums text-gray-500 dark:text-dark-300' }, note),
        ),
      ])
    }
  },
})

interface PricePair {
  primary: string
  official: string
  discounted: boolean
  notes: string[]
}

interface TokenPairs {
  input: PricePair
  output: PricePair
  cacheWrite: PricePair
  cacheRead: PricePair
}

/** 展开后的一行：一个上下文档位，或一个分时时段。 */
interface DetailRow extends TokenPairs {
  key: string
  label: string
  hint: string
}

interface RowBadge {
  label: string
  title: string
}

interface TokenSheetRow extends TokenPairs {
  kind: 'token'
  name: string
  unit: string
  savings: number | null
  tierCount: number
  periodCount: number
  details: DetailRow[]
  detailHint: string
  badges: RowBadge[]
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

/** token 价格的来源：模型基础价、某个上下文档位，或官方参考价。 */
type TokenPriceSource = Pick<
  UserPricingInterval,
  'input_price' | 'output_price' | 'cache_write_price' | 'cache_write_1h_price' | 'cache_read_price'
>

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

/** 一格价格：展示用取整后的金额，是否划线用未取整的价格比较。 */
function pair(
  groupPerToken: number | null | undefined,
  officialPerToken: number | null | undefined,
  rate: number,
  notes: string[] = [],
): PricePair {
  const officialText = formatYuan(officialYuan(officialPerToken))
  if (mode.value === 'official') {
    return { primary: officialText, official: '-', discounted: false, notes }
  }
  return {
    primary: formatYuan(groupYuan(groupPerToken, rate)),
    official: officialText,
    discounted: isCheaperPrice(groupYuanExact(groupPerToken, rate), officialYuanExact(officialPerToken)),
    notes,
  }
}

/** 分组启用高峰时，在分组价下补一行高峰价。 */
function peakNotes(perToken: number | null | undefined, rate: number): string[] {
  if (mode.value !== 'group' || peakFactor.value == null) return []
  const amount = groupYuan(perToken, rate * peakFactor.value)
  return amount == null ? [] : [t('modelPlaza.table.peakLine', { amount: formatYuan(amount) })]
}

/** 1 小时缓存写入价与默认写入价不同才单列一行，相同就不重复。 */
function cacheWrite1hNotes(
  group: TokenPriceSource | null | undefined,
  official: TokenPriceSource | null | undefined,
  rate: number,
): string[] {
  const [base, oneHour] =
    mode.value === 'official'
      ? [officialYuan(official?.cache_write_price), officialYuan(official?.cache_write_1h_price)]
      : [groupYuan(group?.cache_write_price, rate), groupYuan(group?.cache_write_1h_price, rate)]
  if (oneHour == null || oneHour === base) return []
  return [t('modelPlaza.table.cacheWrite1hLine', { amount: formatYuan(oneHour) })]
}

function tokenPairs(
  group: TokenPriceSource | null | undefined,
  official: TokenPriceSource | null | undefined,
  rate: number,
  withPeak: boolean,
): TokenPairs {
  const peak = (perToken: number | null | undefined) => (withPeak ? peakNotes(perToken, rate) : [])
  return {
    input: pair(group?.input_price, official?.input_price, rate, peak(group?.input_price)),
    output: pair(group?.output_price, official?.output_price, rate, peak(group?.output_price)),
    cacheWrite: pair(
      group?.cache_write_price,
      official?.cache_write_price,
      rate,
      [...cacheWrite1hNotes(group, official, rate), ...peak(group?.cache_write_price)],
    ),
    cacheRead: pair(group?.cache_read_price, official?.cache_read_price, rate, peak(group?.cache_read_price)),
  }
}

/** 分组实付的上下文阶梯；分组关闭阶梯时后端不下发，这里自然为空。 */
function groupTiers(m: PlazaModel): UserPricingInterval[] {
  if (!m.pricing) return []
  return sortPlazaIntervals(m.pricing.intervals ?? []).map((interval) => resolveIntervalPrices(interval, m.pricing!))
}

function officialTiers(m: PlazaModel): UserPricingInterval[] {
  return sortPlazaIntervals(m.official_pricing?.intervals ?? [])
}

/** 档位说明：整单按档计价，或仅超出部分按档计价。 */
function tierHint(m: PlazaModel): string {
  return m.long_context_basis === 'marginal'
    ? t('modelPlaza.table.tierHintMarginal')
    : t('modelPlaza.table.tierHint')
}

/** 只有一档时与基础价相同，不必展开；两档起才列出。 */
function tierDetails(m: PlazaModel, rate: number): DetailRow[] {
  const groupSide = groupTiers(m)
  const officialSide = officialTiers(m)
  const official = mode.value === 'official'
  const source = official ? officialSide : groupSide
  if (source.length < 2) return []
  const hint = tierHint(m)
  return source.map((interval, index) => {
    // 两边档数一致才逐档对照官方价，否则官方参考留空，避免错位对比。
    const officialTier = official
      ? interval
      : officialSide.length === groupSide.length
        ? officialSide[index]
        : null
    return {
      key: `tier-${index}`,
      label: plazaTierLabel(interval),
      hint,
      ...tokenPairs(official ? null : interval, officialTier, rate, true),
    }
  })
}

function timeHint(m: PlazaModel): string {
  const key = m.time_pricing?.weekdays_only
    ? 'modelPlaza.table.timePricingRowHintWeekdays'
    : 'modelPlaza.table.timePricingRowHint'
  let hint = t(key, { timezone: m.time_pricing?.timezone })
  if (props.peakWindow) {
    hint += t('modelPlaza.table.timePricingRowHintPeak', {
      window: props.peakWindow,
      multiplier: props.peakRateMultiplier ?? 1,
    })
  }
  return hint
}

/** 分时价 = 基础价 × 生效倍率 × 时段倍率；官方价没有分时，官方价模式下不列。 */
function timeDetails(m: PlazaModel, rate: number): DetailRow[] {
  if (mode.value === 'official' || !m.pricing) return []
  const periods = m.time_pricing?.periods ?? []
  if (periods.length === 0) return []
  const hint = timeHint(m)
  const prefix = m.time_pricing?.weekdays_only ? `${t('modelPlaza.table.timePricingWeekdays')} ` : ''
  return periods.map((period, index) => ({
    key: `time-${index}`,
    label:
      prefix +
      t('modelPlaza.table.timePeriodLabel', {
        window: formatPlazaTimeWindow(period),
        multiplier: period.multiplier,
      }),
    hint,
    ...tokenPairs(m.pricing, m.official_pricing, rate * period.multiplier, false),
  }))
}

function rowBadges(m: PlazaModel, hasTiers: boolean): RowBadge[] {
  const badges: RowBadge[] = []
  if (hasTiers && m.long_context_basis === 'marginal') {
    badges.push({
      label: t('modelPlaza.table.marginalBadge'),
      title: t('modelPlaza.table.tierHintMarginal'),
    })
  }
  const multipliers = m.pricing?.reasoning_effort_multipliers
  for (const effort of REASONING_EFFORT_LEVELS) {
    const multiplier = multipliers?.[effort]
    if (typeof multiplier === 'number' && Number.isFinite(multiplier) && multiplier > 0) {
      badges.push({
        label: t('modelPlaza.table.reasoningMultiplierBadge', { effort, multiplier }),
        title: t('modelPlaza.table.reasoningMultiplierHint', { effort, multiplier }),
      })
    }
  }
  return badges
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
      // Official prices are per token, never comparable to per-image/request prices.
      const tiers = tierDetails(m, rate)
      const periods = timeDetails(m, rate)
      const hints = [tiers[0]?.hint, periods[0]?.hint].filter((hint): hint is string => Boolean(hint))
      return {
        kind: 'token',
        name: m.name,
        unit: t('modelPlaza.table.unitPerMillionShort'),
        ...tokenPairs(m.pricing, m.official_pricing, rate, true),
        savings:
          mode.value === 'group'
            ? savingsPercent(
                groupYuanExact(m.pricing?.input_price, rate),
                officialYuanExact(m.official_pricing?.input_price),
              )
            : null,
        tierCount: tiers.length,
        periodCount: periods.length,
        details: [...tiers, ...periods],
        detailHint: hints.join(' '),
        badges: rowBadges(m, tiers.length > 0),
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

/** 已展开分档价格的模型名。切换分组时组件按分组重建，展开状态随之清空。 */
const expanded = ref<Set<string>>(new Set())

function isExpanded(name: string): boolean {
  return expanded.value.has(name)
}

function toggle(name: string) {
  const next = new Set(expanded.value)
  if (!next.delete(name)) next.add(name)
  expanded.value = next
}
</script>
