<template>
  <!-- 已登录走后台布局（侧栏「模型广场」）；未登录走独立导航形态，统一单源内嵌官网实时价格页 -->
  <AppLayout v-if="isEmbedded">
    <div class="relative w-full">
      <div
        v-if="showFallbackBar"
        class="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-2.5 text-sm text-amber-800 dark:text-amber-200"
      >
        <span>{{ t('modelPlaza.embed.fallbackHint') }}</span>
        <div class="flex flex-shrink-0 items-center gap-2">
          <button
            type="button"
            class="rounded-lg border border-amber-500/40 px-2.5 py-1 text-xs font-medium hover:bg-amber-500/15"
            @click="retryEmbed"
          >
            {{ t('modelPlaza.embed.retry') }}
          </button>
          <a
            :href="directPricingUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg bg-amber-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-amber-500"
          >
            {{ t('modelPlaza.embed.openExternal') }}
          </a>
        </div>
      </div>

      <div
        v-if="loadFailed"
        class="flex min-h-[360px] flex-col items-center justify-center gap-4 rounded-2xl border border-gray-200 bg-white/90 px-6 py-10 text-center dark:border-dark-700 dark:bg-dark-800/80"
      >
        <p class="max-w-md text-sm text-gray-600 dark:text-gray-300">
          {{ t('modelPlaza.embed.loadTimeout') }}
        </p>
        <div class="flex flex-wrap items-center justify-center gap-3">
          <button
            type="button"
            class="rounded-xl border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
            @click="retryEmbed"
          >
            {{ t('modelPlaza.embed.retry') }}
          </button>
          <a
            :href="directPricingUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-xl bg-primary-600 px-4 py-2 text-sm font-semibold text-white hover:bg-primary-500"
          >
            {{ t('modelPlaza.embed.openExternal') }}
          </a>
        </div>
      </div>

      <template v-else>
        <div
          v-if="showSkeleton"
          class="min-h-[520px] w-full animate-pulse rounded-2xl border border-gray-200 bg-white/80 dark:border-dark-700 dark:bg-dark-800/70"
        />
        <iframe
          :key="frameKey"
          ref="frameRef"
          :src="embedSrc"
          :title="t('nav.modelPlaza')"
          class="block w-full border-0 bg-transparent transition-opacity duration-200"
          :class="frameLoaded ? 'opacity-100' : 'pointer-events-none absolute inset-0 opacity-0'"
          :style="iframeStyle"
          :scrolling="heightSynced ? 'no' : 'auto'"
          @load="handleFrameLoad"
          @error="handleFrameError"
        />
      </template>
    </div>
  </AppLayout>

  <div v-else class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <PlazaNavBar />
    <main class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
      <div class="relative w-full">
        <div
          v-if="showFallbackBar"
          class="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-2.5 text-sm text-amber-800 dark:text-amber-200"
        >
          <span>{{ t('modelPlaza.embed.fallbackHint') }}</span>
          <div class="flex flex-shrink-0 items-center gap-2">
            <button
              type="button"
              class="rounded-lg border border-amber-500/40 px-2.5 py-1 text-xs font-medium hover:bg-amber-500/15"
              @click="retryEmbed"
            >
              {{ t('modelPlaza.embed.retry') }}
            </button>
            <a
              :href="directPricingUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="rounded-lg bg-amber-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-amber-500"
            >
              {{ t('modelPlaza.embed.openExternal') }}
            </a>
          </div>
        </div>

        <div
          v-if="loadFailed"
          class="flex min-h-[360px] flex-col items-center justify-center gap-4 rounded-2xl border border-gray-200 bg-white/90 px-6 py-10 text-center dark:border-dark-700 dark:bg-dark-800/80"
        >
          <p class="max-w-md text-sm text-gray-600 dark:text-gray-300">
            {{ t('modelPlaza.embed.loadTimeout') }}
          </p>
          <div class="flex flex-wrap items-center justify-center gap-3">
            <button
              type="button"
              class="rounded-xl border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
              @click="retryEmbed"
            >
              {{ t('modelPlaza.embed.retry') }}
            </button>
            <a
              :href="directPricingUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="rounded-xl bg-primary-600 px-4 py-2 text-sm font-semibold text-white hover:bg-primary-500"
            >
              {{ t('modelPlaza.embed.openExternal') }}
            </a>
          </div>
        </div>

        <template v-else>
          <div
            v-if="showSkeleton"
            class="min-h-[520px] w-full animate-pulse rounded-2xl border border-gray-200 bg-white/80 dark:border-dark-700 dark:bg-dark-800/70"
          />
          <iframe
            :key="frameKey"
            ref="frameRef"
            :src="embedSrc"
            :title="t('nav.modelPlaza')"
            class="block w-full border-0 bg-transparent transition-opacity duration-200"
            :class="frameLoaded ? 'opacity-100' : 'pointer-events-none absolute inset-0 opacity-0'"
            :style="iframeStyle"
            :scrolling="heightSynced ? 'no' : 'auto'"
            @load="handleFrameLoad"
            @error="handleFrameError"
          />
        </template>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const DEFAULT_EMBED_BASE = 'https://www.pigcode.ai/pricing.html'
const EMBED_HANDSHAKE_TIMEOUT_MS = 8000

const { t, locale } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const isEmbedded = computed(() => authStore.isAuthenticated)
const frameRef = ref<HTMLIFrameElement | null>(null)
const frameKey = ref(0)
const frameLoaded = ref(false)
const heightSynced = ref(false)
const handshakeTimedOut = ref(false)
const frameHeight = ref(760)

const embedBase = (import.meta.env.VITE_MODEL_PLAZA_EMBED_URL || DEFAULT_EMBED_BASE).trim() || DEFAULT_EMBED_BASE
const embedOrigin = (() => {
  try {
    return new URL(embedBase).origin
  } catch {
    return 'https://www.pigcode.ai'
  }
})()

const directPricingUrl = computed(() => {
  try {
    const url = new URL(embedBase)
    url.searchParams.delete('embed')
    return url.toString()
  } catch {
    return DEFAULT_EMBED_BASE
  }
})

const loadFailed = computed(() => handshakeTimedOut.value && !frameLoaded.value)
const showFallbackBar = computed(() => handshakeTimedOut.value && frameLoaded.value && !heightSynced.value)
const showSkeleton = computed(() => !frameLoaded.value && !loadFailed.value)
const iframeStyle = computed(() =>
  heightSynced.value ? { height: `${frameHeight.value}px` } : { height: 'max(760px, 78vh)' },
)

function currentTheme(): 'dark' | 'light' {
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

function toBcp47(code: string): 'zh-CN' | 'en-US' {
  return code.toLowerCase().startsWith('en') ? 'en-US' : 'zh-CN'
}

function buildEmbedUrl(): string {
  try {
    const url = new URL(embedBase)
    url.searchParams.set('embed', '1')
    url.searchParams.set('theme', currentTheme())
    url.searchParams.set('lang', toBcp47(String(locale.value || 'zh')))
    return url.toString()
  } catch {
    return `${DEFAULT_EMBED_BASE}?embed=1&theme=${currentTheme()}&lang=${toBcp47(String(locale.value || 'zh'))}`
  }
}

// 初始 URL 仅在挂载/重试时固定，后续暗色与语言切换通过 postMessage 同步，避免 iframe 重载闪烁
const embedSrc = ref(buildEmbedUrl())

let handshakeTimer: ReturnType<typeof setTimeout> | null = null

function clearHandshakeTimer() {
  if (handshakeTimer !== null) {
    clearTimeout(handshakeTimer)
    handshakeTimer = null
  }
}

function startHandshakeTimer() {
  clearHandshakeTimer()
  handshakeTimer = setTimeout(() => {
    if (!heightSynced.value) {
      handshakeTimedOut.value = true
    }
  }, EMBED_HANDSHAKE_TIMEOUT_MS)
}

function syncHostState() {
  const win = frameRef.value?.contentWindow
  if (!win) return
  win.postMessage(
    {
      type: 'pigcode-embed-sync',
      theme: currentTheme(),
      locale: toBcp47(String(locale.value || 'zh')),
    },
    embedOrigin,
  )
}

function retryEmbed() {
  frameLoaded.value = false
  heightSynced.value = false
  handshakeTimedOut.value = false
  embedSrc.value = buildEmbedUrl()
  frameKey.value += 1
  startHandshakeTimer()
}

function handleFrameLoad() {
  frameLoaded.value = true
  syncHostState()
}

function handleFrameError() {
  clearHandshakeTimer()
  frameLoaded.value = false
  handshakeTimedOut.value = true
}

function handleMessage(event: MessageEvent) {
  if (event.origin !== embedOrigin) return
  const data = event.data as { type?: string; height?: number } | null
  if (!data || data.type !== 'pigcode-pricing-height') return
  const nextHeight = Number(data.height)
  if (Number.isFinite(nextHeight) && nextHeight > 0) {
    const firstSync = !heightSynced.value
    frameHeight.value = Math.max(520, Math.min(20000, Math.ceil(nextHeight)))
    heightSynced.value = true
    handshakeTimedOut.value = false
    clearHandshakeTimer()
    if (firstSync) {
      syncHostState()
    }
  }
}

let themeObserver: MutationObserver | null = null

watch(locale, () => {
  syncHostState()
})

watch(isEmbedded, () => {
  frameLoaded.value = false
  heightSynced.value = false
  handshakeTimedOut.value = false
  embedSrc.value = buildEmbedUrl()
  startHandshakeTimer()
})

onMounted(() => {
  void appStore.fetchPublicSettings()
  window.addEventListener('message', handleMessage)
  startHandshakeTimer()
  themeObserver = new MutationObserver(() => {
    syncHostState()
  })
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  })
})

onBeforeUnmount(() => {
  clearHandshakeTimer()
  window.removeEventListener('message', handleMessage)
  themeObserver?.disconnect()
  themeObserver = null
})
</script>
