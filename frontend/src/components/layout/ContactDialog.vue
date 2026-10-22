<template>
  <BaseDialog
    :show="show"
    :title="t('nav.contact')"
    width="wide"
    close-on-click-outside
    @close="emit('close')"
  >
    <div data-testid="contact-dialog">
      <div class="mx-auto grid max-w-xl grid-cols-3 gap-1 rounded-full bg-[#f4efe8] p-1 dark:bg-dark-700">
        <button
          v-for="tab in modal.tabs"
          :key="tab.id"
          type="button"
          class="rounded-full px-2 py-2 text-sm transition-colors"
          :class="tab.id === activeId
            ? 'bg-white font-semibold text-[#c05621] shadow-sm dark:bg-dark-600 dark:text-amber-400'
            : 'text-[#6b6358] hover:text-[#3f3a34] dark:text-dark-300 dark:hover:text-white'"
          @click="activeId = tab.id"
        >
          {{ t(`contactModal.${tab.id}`) }}
        </button>
      </div>

      <div v-if="cards.length" class="mt-5 grid gap-4" :class="cards.length > 1 ? 'sm:grid-cols-2' : 'mx-auto max-w-sm'">
        <div
          v-for="(card, index) in cards"
          :key="`${activeId}-${index}`"
          class="rounded-2xl border border-[#f0ebe3] bg-white px-5 py-5 text-center dark:border-dark-600 dark:bg-dark-800"
        >
          <h4 class="text-base font-semibold text-gray-900 dark:text-white">{{ card.title }}</h4>
          <div class="mx-auto mt-4 flex min-h-48 items-center justify-center rounded-2xl bg-[#faf7f2] p-4 dark:bg-dark-900">
            <img
              v-if="imageSrc(card.image)"
              :src="imageSrc(card.image)"
              alt=""
              class="max-h-52 w-full object-contain"
            />
          </div>
          <p v-if="card.caption" class="mt-4 text-sm font-medium text-[#c05621] dark:text-amber-400">{{ card.caption }}</p>
        </div>
      </div>
      <p v-else class="py-16 text-center text-sm text-gray-400 dark:text-dark-400">{{ t('contactModal.empty') }}</p>

      <div v-if="activeTab?.email" class="mt-6 pb-2 text-center">
        <p class="text-sm font-medium text-[#c05621] dark:text-amber-400">{{ t('contactModal.email') }}</p>
        <a
          v-if="emailHref"
          :href="emailHref"
          target="_blank"
          rel="noopener noreferrer"
          class="mt-1 inline-block text-2xl font-semibold text-gray-900 hover:text-[#c05621] dark:text-white"
        >{{ activeTab.email }}</a>
        <p v-else class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">{{ activeTab.email }}</p>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { sanitizeUrl } from '@/utils/url'
import {
  contactTabHasContent,
  parseContactModal,
  visibleContactCards,
  type ContactTabId,
} from '@/utils/contactModal'

const props = defineProps<{
  show: boolean
  source: string
}>()

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const activeId = ref<ContactTabId>('service')
const modal = computed(() => parseContactModal(props.source))
const activeTab = computed(() => modal.value.tabs.find((tab) => tab.id === activeId.value) ?? modal.value.tabs[0])
const cards = computed(() => visibleContactCards(activeTab.value))
const emailHref = computed(() => sanitizeUrl(activeTab.value?.email || ''))

function imageSrc(value: string): string {
  return sanitizeUrl(value, { allowRelative: true, allowDataUrl: true })
}

watch(
  () => props.show,
  (open) => {
    if (!open) return
    const first = modal.value.tabs.find(contactTabHasContent)
    activeId.value = first?.id ?? 'service'
  },
)
</script>
