<template>
  <!-- 分组状态 [FORK]：内嵌官网的状态页，后台和官网看到的是同一份状态，只需维护官网那一处。 -->
  <AppLayout>
    <SiteEmbedFrame
      :base-url="embedBase"
      height-message-type="pigcode-status-height"
      :title="t('nav.groupStatus')"
      :fallback-hint="t('groupStatus.embed.fallbackHint')"
      :load-timeout-text="t('groupStatus.embed.loadTimeout')"
      :retry-label="t('groupStatus.embed.retry')"
      :open-external-label="t('groupStatus.embed.openExternal')"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import SiteEmbedFrame from '@/components/common/SiteEmbedFrame.vue'

const DEFAULT_EMBED_BASE = 'https://www.pigcode.ai/status.html'

const { t } = useI18n()

const embedBase =
  (import.meta.env.VITE_GROUP_STATUS_EMBED_URL || DEFAULT_EMBED_BASE).trim() || DEFAULT_EMBED_BASE
</script>
