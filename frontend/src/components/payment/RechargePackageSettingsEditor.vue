<template>
  <div data-testid="recharge-package-settings" class="space-y-3">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <p class="input-label mb-0">{{ t('admin.settings.payment.rechargePackages') }}</p>
        <p class="mt-0.5 text-xs text-gray-400">{{ t('admin.settings.payment.rechargePackagesHint') }}</p>
      </div>
      <div class="flex shrink-0 flex-wrap gap-2">
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
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import {
  DEFAULT_RECHARGE_PACKAGES,
  createEmptyRechargePackage,
  type RechargePackage,
} from './rechargePackages'

const props = defineProps<{
  modelValue: RechargePackage[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: RechargePackage[]]
}>()

const { t } = useI18n()

const model = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

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
