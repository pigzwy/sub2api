<template>
  <div data-testid="recharge-package-settings" class="space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div>
        <p class="input-label mb-0">{{ t('admin.settings.payment.rechargePackages') }}</p>
        <p class="mt-0.5 text-xs text-gray-400">{{ t('admin.settings.payment.rechargePackagesHint') }}</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary btn-sm" @click="resetDefaults">
          {{ t('admin.settings.payment.resetPackages') }}
        </button>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="model.length >= 24" @click="addPackage">
          {{ t('admin.settings.payment.addPackage') }}
        </button>
      </div>
    </div>
    <div class="space-y-3">
      <div
        v-for="(pkg, index) in model"
        :key="pkg.id || index"
        class="rounded-xl border border-gray-200 p-3 dark:border-dark-600"
      >
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-6">
          <label class="block">
            <span class="mb-1 block text-xs text-gray-500">{{ t('admin.settings.payment.packageName') }}</span>
            <input v-model="pkg.name" type="text" maxlength="32" class="input" />
          </label>
          <label class="block">
            <span class="mb-1 block text-xs text-gray-500">{{ t('admin.settings.payment.packageNameEn') }}</span>
            <input v-model="pkg.name_en" type="text" maxlength="32" class="input" />
          </label>
          <label class="block">
            <span class="mb-1 block text-xs text-gray-500">{{ t('admin.settings.payment.packageAmount') }}</span>
            <input v-model.number="pkg.amount" type="number" min="0.01" step="0.01" class="input" />
          </label>
          <label class="block">
            <span class="mb-1 block text-xs text-gray-500">{{ t('admin.settings.payment.packageBonus') }}</span>
            <input v-model.number="pkg.bonus" type="number" min="0" step="0.01" class="input" />
          </label>
          <label class="block">
            <span class="mb-1 block text-xs text-gray-500">{{ t('admin.settings.payment.packageBadge') }}</span>
            <select v-model="pkg.badge" class="input">
              <option value="">{{ t('admin.settings.payment.packageBadgeNone') }}</option>
              <option value="popular">{{ t('payment.popular') }}</option>
              <option value="bestValue">{{ t('payment.bestValue') }}</option>
            </select>
          </label>
          <div class="flex items-end">
            <button type="button" class="btn btn-secondary btn-sm w-full" :disabled="model.length <= 1" @click="removePackage(index)">
              {{ t('admin.settings.payment.removePackage') }}
            </button>
          </div>
          <label class="block sm:col-span-2 lg:col-span-3">
            <span class="mb-1 block text-xs text-gray-500">{{ t('admin.settings.payment.packageDesc') }}</span>
            <input v-model="pkg.description" type="text" maxlength="80" class="input" />
          </label>
          <label class="block sm:col-span-2 lg:col-span-3">
            <span class="mb-1 block text-xs text-gray-500">{{ t('admin.settings.payment.packageDescEn') }}</span>
            <input v-model="pkg.description_en" type="text" maxlength="80" class="input" />
          </label>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
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
