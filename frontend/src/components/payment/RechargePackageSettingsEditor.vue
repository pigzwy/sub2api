<template>
  <div data-testid="recharge-package-settings">
    <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <button
        type="button"
        class="min-w-0 flex-1 text-left"
        :aria-expanded="expanded"
        :aria-label="expanded ? t('admin.settings.payment.collapsePackages') : t('admin.settings.payment.expandPackages')"
        data-testid="recharge-package-toggle"
        @click="expanded = !expanded"
      >
        <div class="flex items-center gap-2">
          <Icon
            :name="expanded ? 'chevronDown' : 'chevronRight'"
            size="sm"
            class="shrink-0 text-gray-400"
          />
          <p class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.payment.rechargePackages') }}
          </p>
          <span class="text-xs text-gray-400">
            {{ t('admin.settings.payment.rechargePackagesCount', { count: model.length }) }}
          </span>
        </div>
        <p class="mt-1 pl-6 text-xs text-gray-400">{{ t('admin.settings.payment.rechargePackagesHint') }}</p>
      </button>
      <div v-if="expanded" class="flex shrink-0 flex-wrap gap-2">
        <button type="button" class="btn btn-secondary btn-sm" @click="resetDefaults">
          {{ t('admin.settings.payment.resetPackages') }}
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="model.length >= 24"
          data-testid="recharge-package-add"
          @click="addPackage"
        >
          {{ t('admin.settings.payment.addPackage') }}
        </button>
      </div>
    </div>

    <div v-show="expanded" class="p-6 pt-4">
      <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
        <table class="min-w-[880px] w-full border-collapse text-sm">
          <thead class="bg-gray-50 dark:bg-dark-800/80">
            <tr class="text-left text-xs font-medium text-gray-500 dark:text-gray-400">
              <th class="w-28 px-3 py-2.5 font-medium">{{ t('admin.settings.payment.packageName') }}</th>
              <th class="w-28 px-3 py-2.5 font-medium">{{ t('admin.settings.payment.packageNameEn') }}</th>
              <th class="w-24 px-3 py-2.5 font-medium">{{ t('admin.settings.payment.packageAmount') }}</th>
              <th class="w-24 px-3 py-2.5 font-medium">{{ t('admin.settings.payment.packageBonus') }}</th>
              <th class="w-28 px-3 py-2.5 font-medium">{{ t('admin.settings.payment.packageBadge') }}</th>
              <th class="px-3 py-2.5 font-medium">{{ t('admin.settings.payment.packageDesc') }}</th>
              <th class="px-3 py-2.5 font-medium">{{ t('admin.settings.payment.packageDescEn') }}</th>
              <th class="w-14 px-3 py-2.5 text-right font-medium">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700/80">
            <tr
              v-for="(pkg, index) in model"
              :key="pkg.id || index"
              class="align-middle bg-white dark:bg-dark-900/40"
            >
              <td class="px-3 py-2">
                <input
                  v-model="pkg.name"
                  type="text"
                  maxlength="32"
                  class="input !h-9 !px-2.5 !py-1.5"
                />
              </td>
              <td class="px-3 py-2">
                <input
                  v-model="pkg.name_en"
                  type="text"
                  maxlength="32"
                  class="input !h-9 !px-2.5 !py-1.5"
                />
              </td>
              <td class="px-3 py-2">
                <input
                  v-model.number="pkg.amount"
                  type="number"
                  min="0.01"
                  step="0.01"
                  class="input !h-9 !px-2.5 !py-1.5 tabular-nums"
                />
              </td>
              <td class="px-3 py-2">
                <input
                  v-model.number="pkg.bonus"
                  type="number"
                  min="0"
                  step="0.01"
                  class="input !h-9 !px-2.5 !py-1.5 tabular-nums"
                />
              </td>
              <td class="px-3 py-2">
                <select v-model="pkg.badge" class="input !h-9 !px-2.5 !py-1.5">
                  <option value="">{{ t('admin.settings.payment.packageBadgeNone') }}</option>
                  <option value="popular">{{ t('payment.popular') }}</option>
                  <option value="bestValue">{{ t('payment.bestValue') }}</option>
                </select>
              </td>
              <td class="px-3 py-2">
                <input
                  v-model="pkg.description"
                  type="text"
                  maxlength="80"
                  class="input !h-9 !px-2.5 !py-1.5"
                />
              </td>
              <td class="px-3 py-2">
                <input
                  v-model="pkg.description_en"
                  type="text"
                  maxlength="80"
                  class="input !h-9 !px-2.5 !py-1.5"
                />
              </td>
              <td class="px-3 py-2 text-right">
                <button
                  type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-gray-400 hover:bg-red-50 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-red-500/10 dark:hover:text-red-400"
                  :disabled="model.length <= 1"
                  :aria-label="t('admin.settings.payment.removePackage')"
                  @click="removePackage(index)"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import {
  DEFAULT_RECHARGE_PACKAGES,
  createEmptyRechargePackage,
  type RechargePackage,
} from './rechargePackages'

const STORAGE_KEY = 'sub2api_recharge_packages_expanded'

const props = defineProps<{
  modelValue: RechargePackage[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: RechargePackage[]]
}>()

const { t } = useI18n()
const expanded = ref(readExpanded())

const model = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

watch(expanded, (value) => {
  try {
    localStorage.setItem(STORAGE_KEY, value ? '1' : '0')
  } catch {
    // ignore quota / private mode
  }
})

function readExpanded(): boolean {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === '0') return false
    if (stored === '1') return true
  } catch {
    // ignore
  }
  return false
}

function addPackage() {
  model.value = [...model.value, createEmptyRechargePackage()]
}

function removePackage(index: number) {
  model.value = model.value.filter((_, i) => i !== index)
}

function resetDefaults() {
  model.value = DEFAULT_RECHARGE_PACKAGES.map((pkg) => ({ ...pkg, bonus: 0 }))
}
</script>
