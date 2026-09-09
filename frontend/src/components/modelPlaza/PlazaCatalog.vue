<template>
  <div class="space-y-5">
    <div
      class="flex gap-1 overflow-x-auto border-b border-gray-200 pb-px dark:border-dark-700"
      data-testid="plaza-platform-tabs"
    >
      <button
        type="button"
        :class="tabClass(platform === 'all')"
        @click="emit('update:platform', 'all')"
      >
        {{ t('modelPlaza.filters.all') }}
      </button>
      <button
        v-for="p in platforms"
        :key="p"
        type="button"
        :class="tabClass(platform === p)"
        @click="emit('update:platform', p)"
      >
        <span
          class="h-2 w-2 rounded-full"
          :style="{ backgroundColor: platformAccentColor(p) }"
        />
        {{ plazaTabLabel(p, t) }}
      </button>
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
              ? 'border-primary-500 bg-primary-50/90 shadow-card dark:border-primary-400/60 dark:bg-primary-500/10'
              : 'border-gray-200 bg-white hover:border-gray-300 dark:border-dark-700 dark:bg-dark-800/50 dark:hover:border-dark-500',
          ]"
          @click="emit('update:groupId', g.id)"
        >
          <span
            class="absolute right-3 top-3 rounded-full bg-amber-500 px-2 py-0.5 text-[11px] font-semibold text-white"
          >
            {{ t('modelPlaza.catalog.zheBadge', { zhe: formatCatalogZhe(effectiveRate(g)) }) }}
          </span>
          <p class="pr-16 text-sm font-semibold text-gray-900 dark:text-white">{{ g.name }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ rateCaption(g) }}
          </p>
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

function rateCaption(g: ModelPlazaGroup): string {
  const rate = effectiveRate(g)
  const base = t('modelPlaza.catalog.rateLine', { rate })
  return `${base} · ${t('modelPlaza.catalog.discountLine', { zhe: formatCatalogZhe(rate) })}`
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
