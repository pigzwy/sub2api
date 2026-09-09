<template>
  <div class="space-y-4">
    <nav
      class="rounded-lg border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      :aria-label="t('modelPlaza.catalog.products')"
      data-testid="plaza-platform-tabs"
    >
      <div class="flex flex-wrap gap-1">
        <button
          v-for="p in platforms"
          :key="p"
          type="button"
          :class="platformTabClass(p)"
          :style="platformTabStyle(p)"
          @click="emit('update:platform', p)"
        >
          <PlatformIcon :platform="p as GroupPlatform" size="sm" :class="platformIconClass(p)" />
          {{ plazaTabLabel(p, t) }}
        </button>
      </div>
    </nav>

    <div
      v-if="showDefaultRule"
      class="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 rounded-xl bg-[#f4ead8] px-4 py-2.5 text-sm text-amber-950 dark:bg-amber-500/10 dark:text-amber-100"
    >
      <p class="inline-flex min-w-0 flex-wrap items-center gap-x-2 leading-6">
        <span class="inline-flex items-center gap-1.5 font-medium">
          <Icon name="document" size="sm" class="h-4 w-4" />
          {{ t('modelPlaza.catalog.ruleTitle') }}
        </span>
        <span>{{ t('modelPlaza.catalog.ruleFx') }}</span>
        <span>{{ t('modelPlaza.catalog.ruleFormula') }}</span>
      </p>
      <p v-if="ruleExample" class="text-xs text-amber-900/80 dark:text-amber-200/80">
        {{ ruleExample }}
      </p>
    </div>

    <div>
      <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <h2 class="inline-flex items-center gap-1.5 text-[15px] font-semibold text-gray-900 dark:text-white">
          <Icon name="checkCircle" size="sm" class="h-4 w-4 text-amber-500" />
          {{ t('modelPlaza.catalog.priceList') }}
        </h2>
        <div class="flex flex-wrap items-center gap-3">
          <p class="text-xs text-gray-400 dark:text-dark-400">
            {{ t('modelPlaza.catalog.selectGroupHint') }}
          </p>
          <div class="inline-flex rounded-full bg-gray-100 p-0.5 text-xs dark:bg-dark-800">
            <button
              type="button"
              :class="priceModeClass('group')"
              @click="emit('update:priceMode', 'group')"
            >
              {{ t('modelPlaza.catalog.groupPrice') }}
            </button>
            <button
              type="button"
              :class="priceModeClass('official')"
              @click="emit('update:priceMode', 'official')"
            >
              {{ t('modelPlaza.catalog.officialPrice') }}
            </button>
          </div>
        </div>
      </div>

      <div
        class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4"
        data-testid="plaza-group-cards"
      >
        <button
          v-for="g in groups"
          :key="g.id"
          type="button"
          :data-testid="`plaza-group-card-${g.id}`"
          :class="[
            'relative min-h-[96px] rounded-xl border px-4 py-3.5 text-left transition-colors',
            groupId === g.id
              ? 'border-amber-500 bg-amber-50 shadow-[0_0_0_1px_rgba(245,158,11,0.35)] dark:border-amber-400 dark:bg-amber-500/10'
              : 'border-gray-200 bg-white hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-dark-500',
          ]"
          @click="emit('update:groupId', g.id)"
        >
          <span
            class="absolute right-3 top-3 rounded-full bg-amber-500 px-2 py-0.5 text-[11px] font-semibold text-white"
          >
            {{ t('modelPlaza.catalog.zheBadge', { zhe: formatCatalogZhe(effectiveRate(g)) }) }}
          </span>
          <div class="flex items-start gap-1.5 pr-14">
            <Icon
              v-if="groupId === g.id"
              name="checkCircle"
              size="sm"
              class="mt-0.5 h-4 w-4 shrink-0 text-amber-500"
            />
            <p
              class="text-sm font-semibold leading-5 text-gray-900 dark:text-white"
              :data-testid="groupId === g.id ? 'plaza-group-name' : undefined"
            >
              {{ g.name }}
            </p>
          </div>
          <p class="mt-2 text-xs leading-5 text-gray-400 dark:text-dark-400">
            {{ rateCaption(g) }}
          </p>
        </button>
      </div>

      <p
        v-if="selectedGroup"
        class="mt-3 text-sm leading-6 text-gray-500 dark:text-dark-400"
        data-testid="plaza-group-intro"
      >
        <span class="text-gray-600 dark:text-dark-300">{{ t('modelPlaza.catalog.groupIntro') }}：</span>
        {{ selectedGroup.description?.trim() || selectedGroup.name }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import type { ModelPlazaGroup } from '@/api/modelPlaza'
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
  const model = g.models.find((m) => m.official_pricing?.input_price || m.pricing?.input_price)
  if (!model) return ''
  const perToken = model.official_pricing?.input_price ?? model.pricing?.input_price
  const official = officialYuan(perToken)
  const group = groupYuan(perToken, effectiveRate(g))
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
    'inline-flex shrink-0 items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors',
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
    'rounded-full px-3 py-1 font-medium',
    props.priceMode === mode
      ? 'bg-amber-500 text-white'
      : 'text-gray-500 dark:text-dark-400',
  ].join(' ')
}
</script>
