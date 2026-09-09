<template>
  <div class="space-y-5">
    <div
      class="flex gap-1 overflow-x-auto border-b border-gray-200 dark:border-dark-700"
      data-testid="plaza-platform-tabs"
    >
      <button
        v-for="p in platforms"
        :key="p"
        type="button"
        :class="tabClass(platform === p)"
        @click="emit('update:platform', p)"
      >
        <PlatformIcon :platform="p as GroupPlatform" size="sm" />
        {{ plazaTabLabel(p, t) }}
      </button>
    </div>

    <div
      v-if="showDefaultRule"
      class="flex flex-wrap items-start gap-3 rounded-xl border border-amber-200/80 bg-[#f8f1e7] px-4 py-3 text-sm text-amber-950 dark:border-amber-400/20 dark:bg-amber-500/10 dark:text-amber-100"
    >
      <div class="flex items-center gap-1.5 font-medium">
        <Icon name="document" size="sm" class="h-4 w-4 text-amber-700 dark:text-amber-300" />
        {{ t('modelPlaza.catalog.ruleTitle') }}
      </div>
      <p class="min-w-0 flex-1 leading-6">
        {{ t('modelPlaza.catalog.ruleFx') }}
        <span class="mx-1.5 text-amber-400">·</span>
        {{ t('modelPlaza.catalog.ruleFormula') }}
        <span v-if="ruleExample" class="mt-0.5 block text-xs text-amber-800/80 dark:text-amber-200/80">
          {{ ruleExample }}
        </span>
      </p>
    </div>

    <div>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h2 class="inline-flex items-center gap-1.5 text-base font-semibold text-gray-900 dark:text-white">
          <Icon name="checkCircle" size="sm" class="h-4 w-4 text-amber-500" />
          {{ t('modelPlaza.catalog.priceList') }}
        </h2>
        <div class="flex flex-wrap items-center gap-3">
          <div class="text-right">
            <div class="inline-flex rounded-lg bg-gray-100 p-0.5 text-xs dark:bg-dark-800">
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
            <p class="mt-1 text-[11px] text-gray-400 dark:text-dark-500">
              {{ t('modelPlaza.catalog.priceModeHint') }}
            </p>
          </div>
          <input
            :value="search"
            type="search"
            class="input h-9 w-44 text-sm"
            :placeholder="t('modelPlaza.filters.searchPlaceholder')"
            @input="emit('update:search', ($event.target as HTMLInputElement).value)"
          />
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
            'relative rounded-2xl border p-4 text-left transition-colors',
            groupId === g.id
              ? 'border-amber-500 bg-amber-50/90 shadow-[0_0_0_3px_rgba(245,158,11,0.15)] dark:border-amber-400/70 dark:bg-amber-500/10'
              : 'border-gray-200 bg-white hover:border-gray-300 dark:border-dark-700 dark:bg-dark-800/50 dark:hover:border-dark-500',
          ]"
          @click="emit('update:groupId', g.id)"
        >
          <div class="absolute right-3 top-3 flex items-center gap-1.5">
            <Icon
              v-if="groupId === g.id"
              name="checkCircle"
              size="sm"
              class="h-4 w-4 text-emerald-500"
            />
            <span class="rounded-full bg-amber-500 px-2 py-0.5 text-[11px] font-semibold text-white">
              {{ t('modelPlaza.catalog.zheBadge', { zhe: formatCatalogZhe(effectiveRate(g)) }) }}
            </span>
          </div>
          <p
            class="pr-24 text-sm font-semibold text-gray-900 dark:text-white"
            :data-testid="groupId === g.id ? 'plaza-group-name' : undefined"
          >
            {{ g.name }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ rateCaption(g) }}
          </p>
        </button>
      </div>

      <p
        v-if="selectedGroup"
        class="mt-3 text-sm text-gray-500 dark:text-dark-400"
        data-testid="plaza-group-intro"
      >
        <span class="font-medium text-gray-700 dark:text-dark-200">{{ t('modelPlaza.catalog.groupIntro') }}：</span>
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
  search: string
  showDefaultRule: boolean
  priceMode: 'group' | 'official'
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number]
  'update:search': [value: string]
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

function tabClass(active: boolean): string {
  return [
    'inline-flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors',
    active
      ? 'border-amber-500 text-amber-600 dark:border-amber-400 dark:text-amber-300'
      : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-white',
  ].join(' ')
}

function priceModeClass(mode: 'group' | 'official'): string {
  return [
    'rounded-md px-2.5 py-1 font-medium',
    props.priceMode === mode
      ? 'bg-amber-500 text-white shadow-sm'
      : 'text-gray-500 dark:text-dark-400',
  ].join(' ')
}
</script>
