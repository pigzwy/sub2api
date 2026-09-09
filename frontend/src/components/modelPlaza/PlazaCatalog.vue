<template>
  <div class="space-y-5">
    <div
      class="flex gap-1 overflow-x-auto border-b border-gray-200 pb-px dark:border-dark-700"
      data-testid="plaza-platform-tabs"
    >
      <button
        v-for="p in platforms"
        :key="p"
        type="button"
        :class="[
          'inline-flex shrink-0 items-center gap-2 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors',
          platform === p
            ? 'border-primary-500 text-gray-900 dark:border-primary-400 dark:text-white'
            : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-white',
        ]"
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
        <input
          :value="search"
          type="search"
          class="input h-9 w-full max-w-xs text-sm"
          :placeholder="t('modelPlaza.filters.searchPlaceholder')"
          @input="emit('update:search', ($event.target as HTMLInputElement).value)"
        />
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
            v-if="effectiveRate(g) < 1"
            class="absolute right-3 top-3 rounded-full bg-amber-500 px-2 py-0.5 text-[11px] font-semibold text-white"
          >
            {{ t('modelPlaza.catalog.zheBadge', { zhe: formatZhe(effectiveRate(g)) }) }}
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
import { formatZhe, plazaTabLabel } from './plazaCatalog'

const props = defineProps<{
  platforms: string[]
  platform: string
  groups: ModelPlazaGroup[]
  groupId: number | 'all'
  search: string
  showDefaultRule: boolean
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number]
  'update:search': [value: string]
}>()

const { t } = useI18n()

function effectiveRate(g: ModelPlazaGroup): number {
  return g.user_rate_multiplier ?? g.rate_multiplier
}

function rateCaption(g: ModelPlazaGroup): string {
  const rate = effectiveRate(g)
  const base = t('modelPlaza.catalog.rateLine', { rate })
  if (rate < 1) {
    return `${base} · ${t('modelPlaza.catalog.discountLine', { zhe: formatZhe(rate), percent: Math.round(rate * 100) })}`
  }
  if (rate > 1) {
    return `${base} · ${t('modelPlaza.catalog.markupLine')}`
  }
  return `${base} · ${t('modelPlaza.catalog.sameLine')}`
}
</script>
