<template>
  <!--
    内嵌官网页面的通用外壳 [FORK]。
    官网页面以 ?embed=1 打开时隐藏自己的页头页脚，并通过 postMessage 上报内容高度；
    这里据此调整 iframe 高度，并把后台的主题、语言同步过去。握手超时则给出重试与新窗口打开两个出口。
    只传 theme / lang，不带用户 ID 和登录令牌：嵌入的是公开页面，令牌不该出现在别的域名的地址里。
  -->
  <div class="relative w-full">
    <div
      v-if="showFallbackBar"
      class="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-2.5 text-sm text-amber-800 dark:text-amber-200"
    >
      <span>{{ fallbackHint }}</span>
      <div class="flex flex-shrink-0 items-center gap-2">
        <button
          type="button"
          class="rounded-lg border border-amber-500/40 px-2.5 py-1 text-xs font-medium hover:bg-amber-500/15"
          @click="retryEmbed"
        >
          {{ retryLabel }}
        </button>
        <a
          :href="directUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="rounded-lg bg-amber-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-amber-500"
        >
          {{ openExternalLabel }}
        </a>
      </div>
    </div>
    <div
      v-if="loadFailed"
      class="flex min-h-[360px] flex-col items-center justify-center gap-4 rounded-2xl border border-gray-200 bg-white/90 px-6 py-10 text-center dark:border-dark-700 dark:bg-dark-800/80"
    >
      <p class="max-w-md text-sm text-gray-600 dark:text-gray-300">
        {{ loadTimeoutText }}
      </p>
      <div class="flex flex-wrap items-center justify-center gap-3">
        <button
          type="button"
          class="rounded-xl border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
          @click="retryEmbed"
        >
          {{ retryLabel }}
        </button>
        <a
          :href="directUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="rounded-xl bg-primary-600 px-4 py-2 text-sm font-semibold text-white hover:bg-primary-500"
        >
          {{ openExternalLabel }}
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
        :title="title"
        class="block w-full border-0 bg-transparent transition-opacity duration-200"
        :class="frameLoaded ? 'opacity-100' : 'pointer-events-none absolute inset-0 opacity-0'"
        :style="iframeStyle"
        :scrolling="heightSynced ? 'no' : 'auto'"
        @load="handleFrameLoad"
        @error="handleFrameError"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  /** 要嵌入的官网页面地址，不含 embed / theme / lang 参数。 */
  baseUrl: string
  /** 官网页面上报内容高度时使用的消息类型。 */
  heightMessageType: string
  /** iframe 的无障碍标题。 */
  title: string
  fallbackHint: string
  loadTimeoutText: string
  retryLabel: string
  openExternalLabel: string
}>()

const HANDSHAKE_TIMEOUT_MS = 8000
const MIN_FRAME_HEIGHT = 520
const MAX_FRAME_HEIGHT = 20000

const { locale } = useI18n()

const frameRef = ref<HTMLIFrameElement | null>(null)
const frameKey = ref(0)
const frameLoaded = ref(false)
const heightSynced = ref(false)
const handshakeTimedOut = ref(false)
const frameHeight = ref(760)

const embedOrigin = computed(() => {
  try {
    return new URL(props.baseUrl).origin
  } catch {
    return ''
  }
})

/** 兜底的“新窗口打开”地址：就是页面本身，不带内嵌参数。 */
const directUrl = computed(() => {
  try {
    const url = new URL(props.baseUrl)
    url.searchParams.delete('embed')
    return url.toString()
  } catch {
    return props.baseUrl
  }
})

const loadFailed = computed(() => handshakeTimedOut.value && !frameLoaded.value)
const showFallbackBar = computed(
  () => handshakeTimedOut.value && frameLoaded.value && !heightSynced.value,
)
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

function currentLocale(): 'zh-CN' | 'en-US' {
  return toBcp47(String(locale.value || 'zh'))
}

function buildEmbedUrl(): string {
  try {
    const url = new URL(props.baseUrl)
    url.searchParams.set('embed', '1')
    url.searchParams.set('theme', currentTheme())
    url.searchParams.set('lang', currentLocale())
    return url.toString()
  } catch {
    return props.baseUrl
  }
}

// 初始地址只在挂载和重试时确定；之后的主题、语言切换走 postMessage，避免 iframe 重载闪烁。
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
  }, HANDSHAKE_TIMEOUT_MS)
}

function syncHostState() {
  const win = frameRef.value?.contentWindow
  if (!win || !embedOrigin.value) return
  win.postMessage(
    { type: 'pigcode-embed-sync', theme: currentTheme(), locale: currentLocale() },
    embedOrigin.value,
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
  // 只认来自被嵌入页面所在域名的消息
  if (!embedOrigin.value || event.origin !== embedOrigin.value) return
  const data = event.data as { type?: string; height?: number } | null
  if (!data || data.type !== props.heightMessageType) return
  const nextHeight = Number(data.height)
  if (!Number.isFinite(nextHeight) || nextHeight <= 0) return
  const firstSync = !heightSynced.value
  frameHeight.value = Math.max(MIN_FRAME_HEIGHT, Math.min(MAX_FRAME_HEIGHT, Math.ceil(nextHeight)))
  heightSynced.value = true
  handshakeTimedOut.value = false
  clearHandshakeTimer()
  if (firstSync) {
    syncHostState()
  }
}

let themeObserver: MutationObserver | null = null

watch(locale, () => {
  syncHostState()
})

onMounted(() => {
  window.addEventListener('message', handleMessage)
  startHandshakeTimer()
  themeObserver = new MutationObserver(() => {
    syncHostState()
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onBeforeUnmount(() => {
  clearHandshakeTimer()
  window.removeEventListener('message', handleMessage)
  themeObserver?.disconnect()
  themeObserver = null
})
</script>
