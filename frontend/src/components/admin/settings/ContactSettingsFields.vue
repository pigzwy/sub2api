<template>
  <div class="space-y-4">
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('contactModal.hint') }}</p>
    <div
      v-for="tab in draft.tabs"
      :key="tab.id"
      class="rounded-xl border border-gray-200 p-4 dark:border-dark-600"
    >
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t(`contactModal.${tab.id}`) }}</h3>
      <label class="mt-3 mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('contactModal.email') }}</label>
      <input
        v-model="tab.email"
        type="text"
        class="input text-sm"
        :placeholder="t('contactModal.emailPlaceholder')"
        @input="commit"
      />
      <div class="mt-4 grid gap-4 lg:grid-cols-2">
        <div
          v-for="(card, index) in tab.cards"
          :key="`${tab.id}-${index}`"
          class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800"
        >
          <p class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('contactModal.qr') }} {{ index + 1 }}</p>
          <ImageUpload
            :model-value="card.image"
            mode="image"
            size="lg"
            :max-size="1024 * 1024"
            :upload-label="t('contactModal.qrUpload')"
            :remove-label="t('admin.settings.site.remove')"
            :hint="t('contactModal.qrHint')"
            @update:model-value="(value) => setImage(tab.id, index, value)"
          />
          <input
            v-model="card.title"
            type="text"
            class="input mb-2 mt-3 text-sm"
            :placeholder="t('contactModal.titlePlaceholder')"
            @input="commit"
          />
          <input
            v-model="card.caption"
            type="text"
            class="input mb-2 text-sm"
            :placeholder="t('contactModal.captionPlaceholder')"
            @input="commit"
          />
          <input
            v-if="!card.image.startsWith('data:')"
            v-model="card.image"
            type="text"
            class="input font-mono text-xs"
            placeholder="https://"
            @input="commit"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ImageUpload from '@/components/common/ImageUpload.vue'
import { parseContactModal, serializeContactModal, type ContactTabId } from '@/utils/contactModal'

const props = defineProps<{
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const { t } = useI18n()
const draft = ref(parseContactModal(props.modelValue))

watch(
  () => props.modelValue,
  (value) => {
    if (value === serializeContactModal(draft.value)) return
    draft.value = parseContactModal(value)
  },
)

function commit() {
  emit('update:modelValue', serializeContactModal(draft.value))
}

function setImage(id: ContactTabId, index: number, value: string) {
  const tab = draft.value.tabs.find((item) => item.id === id)
  const card = tab?.cards[index]
  if (!card) return
  card.image = value
  commit()
}
</script>
