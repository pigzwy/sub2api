<template>
  <div class="space-y-5">
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-xs font-medium text-gray-400 dark:text-dark-500">
        {{ t('modelPlaza.catalog.category') }}
      </span>
      <div
        class="flex gap-1 overflow-x-auto"
        data-testid="plaza-platform-tabs"
      >
        <button
          type="button"
          :class="chipClass(platform === 'all')"
          @click="emit('update:platform', 'all')"
        >
          {{ t('modelPlaza.filters.all') }}
        </button>
        <button
          v-for="p in platforms"
          :key="p"
          type="button"
          :class="chipClass(platform === p)"
          @click="emit('update:platform', p)"
        >
          <span
            class="h-2 w-2 rounded-full"
            :style="{ backgroundColor: platformAccentColor(p) }"
          />
          {{ plazaTabLabel(p, t) }}
        </button>
      </div>
    </div>

    <div
      v-if="showDefaultRule"
      class="rounded-xl border border-amber-200/80 bg-amber-50 px-4 py-3 text-sm text-amber-950 dark:border-amber-400/20 dark:bg-amber-500/10 dark:text-amber-100"
    >
      {{ t('modelPlaza.catalog.rule') }}
    </div>

    <div>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ t('modelPlaza.catalog.priceList') }}
        </h2>
        <div class="flex flex-wrap items-center gap-2">
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
        class="mt-3 flex gap-1 overflow-x-auto border-b border-gray-200 pb-px dark:border-dark-700"
        data-testid="plaza-group-tabs"
      >
        <button
          v-for="g in groups"
          :key="g.id"
          type="button"
          :data-testid="`plaza-group-tab-${g.id}`"
          :class="tabClass(groupId === g.id)"
          @click="emit('update:groupId', g.id)"
        >
          <span>{{ g.name }}</span>
          <span
            class="rounded-full bg-amber-500 px-1.5 py-px text-[10px] font-semibold leading-4 text-white"
          >
            {{ t('modelPlaza.catalog.zheBadge', { zhe: formatCatalogZhe(effectiveRate(g)) }) }}
          </span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { platformAccentColor } from '@/utils/platformColors'
import type { ModelPlazaGroup } from '@/api/modelPlaza'
import { formatCatalogZhe, plazaTabLabel } from './plazaCatalog'

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

function effectiveRate(g: ModelPlazaGroup): number {
  return g.user_rate_multiplier ?? g.rate_multiplier
}

function chipClass(active: boolean): string {
  return [
    'inline-flex shrink-0 items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-medium transition-colors',
    active
      ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900'
      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700',
  ].join(' ')
}

function tabClass(active: boolean): string {
  return [
    'inline-flex shrink-0 items-center gap-2 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors',
    active
      ? 'border-primary-500 text-gray-900 dark:border-primary-400 dark:text-white'
      : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-white',
  ].join(' ')
}

function priceModeClass(mode: 'group' | 'official'): string {
  return [
    'rounded-md px-2.5 py-1 font-medium',
    props.priceMode === mode
      ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
      : 'text-gray-500 dark:text-dark-400',
  ].join(' ')
}
</script>
