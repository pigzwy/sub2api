<template>
  <div class="space-y-4">
    <nav
      class="rounded-xl border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      :aria-label="t('modelPlaza.catalog.products')"
      data-testid="plaza-platform-tabs"
    >
      <div class="flex flex-wrap gap-1.5">
        <button
          v-for="p in platforms"
          :key="p"
          type="button"
          :class="platformTabClass(p)"
          :style="platformTabStyle(p)"
          :aria-pressed="platform === p"
          @click="emit('update:platform', p)"
        >
          <PlatformIcon :platform="p as GroupPlatform" size="xl" :class="platformIconClass(p)" />
          {{ plazaTabLabel(p, t) }}
        </button>
      </div>
    </nav>

    <div
      v-if="showDefaultRule"
      class="flex flex-wrap items-center justify-between gap-x-6 gap-y-1.5 rounded-xl border border-gray-200 bg-white px-5 py-3 text-sm shadow-sm dark:border-dark-700 dark:bg-dark-800"
    >
      <p class="inline-flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 leading-6 text-gray-600 dark:text-dark-300">
        <span class="inline-flex items-center gap-1.5 font-semibold text-gray-900 dark:text-white">
          <Icon name="document" size="sm" class="h-4 w-4 text-orange-600 dark:text-orange-300" />
          {{ t('modelPlaza.catalog.ruleTitle') }}
        </span>
        <span>{{ t('modelPlaza.catalog.ruleFx') }}</span>
        <span>{{ t('modelPlaza.catalog.ruleFormula') }}</span>
      </p>
      <p v-if="ruleExample" class="text-[13px] leading-6 text-gray-500 dark:text-dark-400">
        {{ ruleExample }}
      </p>
    </div>

    <!-- 价格列表：标题、分组卡片、分组介绍和价格表收在同一张卡片里 -->
    <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
      <header
        class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-gray-100 px-5 py-4 dark:border-dark-700"
      >
        <h2 class="inline-flex items-center gap-2 text-lg font-semibold text-gray-900 dark:text-white">
          <Icon name="checkCircle" size="md" class="h-5 w-5 text-gray-700 dark:text-dark-200" />
          {{ t('modelPlaza.catalog.priceList') }}
        </h2>
        <div class="flex flex-wrap items-center gap-3">
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('modelPlaza.catalog.selectGroupHint') }}
          </p>
          <div class="inline-flex rounded-full bg-gray-100 p-1 text-xs dark:bg-dark-700">
            <button
              type="button"
              :class="priceModeClass('group')"
              :aria-pressed="priceMode === 'group'"
              @click="emit('update:priceMode', 'group')"
            >
              {{ t('modelPlaza.catalog.groupPrice') }}
            </button>
            <button
              type="button"
              :class="priceModeClass('official')"
              :aria-pressed="priceMode === 'official'"
              @click="emit('update:priceMode', 'official')"
            >
              {{ t('modelPlaza.catalog.officialPrice') }}
            </button>
          </div>
        </div>
      </header>

      <div class="space-y-4 p-5">
        <div class="flex flex-wrap gap-3" data-testid="plaza-group-cards">
          <button
            v-for="g in groups"
            :key="g.id"
            type="button"
            :data-testid="`plaza-group-card-${g.id}`"
            :aria-pressed="groupId === g.id"
            :class="[
              'w-full rounded-xl border px-4 py-3 text-left transition-colors sm:w-auto sm:min-w-[13rem] sm:max-w-full',
              groupId === g.id
                ? 'border-orange-400 bg-orange-50/70 shadow-[0_0_0_1px_rgba(251,146,60,0.45)] dark:border-orange-400/80 dark:bg-orange-500/10'
                : 'border-gray-200 bg-white hover:border-gray-300 hover:bg-gray-50/60 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-dark-500 dark:hover:bg-dark-700/40',
            ]"
            @click="emit('update:groupId', g.id)"
          >
            <span class="flex items-center gap-2">
              <span
                class="min-w-0 text-[15px] font-semibold leading-6 text-gray-900 dark:text-white"
                :data-testid="groupId === g.id ? 'plaza-group-name' : undefined"
              >
                {{ g.name }}
              </span>
              <span
                class="shrink-0 rounded-full bg-orange-100 px-2 py-0.5 text-[11px] font-semibold leading-4 text-orange-800 dark:bg-orange-500/15 dark:text-orange-200"
              >
                {{ t('modelPlaza.catalog.zheBadge', { zhe: formatCatalogZhe(effectiveRate(g)) }) }}
              </span>
              <!-- 未选中时也占位，切换分组时卡片宽度不跳动 -->
              <Icon
                name="checkCircle"
                size="sm"
                class="ml-auto h-4 w-4 shrink-0 text-orange-600 dark:text-orange-300"
                :class="groupId === g.id ? '' : 'invisible'"
              />
            </span>
            <span class="mt-1.5 block text-xs leading-5 text-gray-400 dark:text-dark-400">
              {{ rateCaption(g) }}
            </span>
          </button>
        </div>

        <p
          v-if="selectedGroup"
          class="flex flex-wrap items-baseline gap-x-2 rounded-lg border border-orange-100 bg-orange-50/60 px-4 py-2.5 text-sm leading-6 text-gray-700 dark:border-orange-400/20 dark:bg-orange-500/5 dark:text-dark-200"
          data-testid="plaza-group-intro"
        >
          <span class="shrink-0 font-semibold text-orange-800 dark:text-orange-200">{{ t('modelPlaza.catalog.groupIntro') }}：</span>
          <span class="min-w-0">{{ selectedGroup.description?.trim() || selectedGroup.name }}</span>
        </p>

        <!-- 当前分组的价格表 -->
        <slot />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import type { ModelPlazaGroup } from '@/api/modelPlaza'
import { BILLING_MODE_TOKEN } from '@/constants/channel'
import { platformAccentColor, platformIconClass } from '@/utils/platformColors'
import {
  formatCatalogZhe,
  formatYuan,
  groupYuan,
  officialYuan,
  plazaTabLabel,
} from './plazaCatalog'

const props = defineProps<{
  platforms: string[]
  platform: string
  groups: ModelPlazaGroup[]
  groupId: number | 'all'
  showDefaultRule: boolean
  priceMode: 'group' | 'official'
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number]
  'update:priceMode': [value: 'group' | 'official']
}>()

const { t } = useI18n()

const selectedGroup = computed(
  () => props.groups.find((g) => g.id === props.groupId) ?? props.groups[0] ?? null,
)

const ruleExample = computed(() => {
  const g = selectedGroup.value
  if (!g) return ''
  const model = g.models.find((m) =>
    (m.pricing?.billing_mode || BILLING_MODE_TOKEN) === BILLING_MODE_TOKEN &&
    m.official_pricing?.input_price != null && m.pricing?.input_price != null,
  )
  if (!model) return ''
  const official = officialYuan(model.official_pricing?.input_price)
  const group = groupYuan(model.pricing?.input_price, effectiveRate(g))
  if (official == null || group == null) return ''
  return t('modelPlaza.catalog.ruleExample', {
    model: model.name,
    official: formatYuan(official),
    group: formatYuan(group),
  })
})

function effectiveRate(g: ModelPlazaGroup): number {
  return g.user_rate_multiplier ?? g.rate_multiplier
}

function rateCaption(g: ModelPlazaGroup): string {
  const rate = effectiveRate(g)
  return `${t('modelPlaza.catalog.rateLine', { rate })} · ${t('modelPlaza.catalog.discountLine', { zhe: formatCatalogZhe(rate) })}`
}

function platformTabClass(p: string): string {
  return [
    'inline-flex shrink-0 items-center gap-2.5 rounded-lg border px-4 py-2.5 text-base font-medium leading-6 transition-colors',
    props.platform === p
      ? ''
      : 'border-transparent text-gray-500 hover:bg-gray-50 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-700/50 dark:hover:text-dark-200',
  ].join(' ')
}

function platformTabStyle(p: string): Record<string, string> | undefined {
  if (props.platform !== p) return undefined
  const accent = platformAccentColor(p)
  return {
    borderColor: accent,
    color: accent,
    backgroundColor: `color-mix(in srgb, ${accent} 12%, transparent)`,
  }
}

function priceModeClass(mode: 'group' | 'official'): string {
  return [
    'rounded-full px-3.5 py-1 font-semibold transition-colors',
    props.priceMode === mode
      ? 'bg-orange-700 text-white shadow-sm dark:bg-orange-500'
      : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white',
  ].join(' ')
}
</script>
