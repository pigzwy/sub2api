import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n } from './i18n'
import { useAppStore } from '@/stores/app'
import { updateFavicon } from '@/utils/branding'
import { isIOSDevice } from '@/utils/device'
import './style.css'

const publicSettingsBootstrapTimeoutMs = 2500

async function loadPublicSettingsBeforeMount(appStore: ReturnType<typeof useAppStore>): Promise<void> {
  // A static HTML shell has no embedded runtime configuration. Give the
  // public settings request a short head start so branding and feature gates
  // are ready before the first paint, but never hold the whole application
  // hostage when the endpoint is degraded or under attack.
  if (appStore.initFromInjectedConfig()) return

  let timeoutId: number | undefined
  const timeout = new Promise<null>((resolve) => {
    timeoutId = window.setTimeout(() => resolve(null), publicSettingsBootstrapTimeoutMs)
  })

  try {
    await Promise.race([appStore.fetchPublicSettings(), timeout])
  } finally {
    if (timeoutId !== undefined) {
      window.clearTimeout(timeoutId)
    }
  }
}

function initIOSViewportZoomFix() {
  // iOS Safari 在输入框字号小于 16px 时聚焦会自动放大页面，且失焦后不会恢复。
  // 限制 maximum-scale 可阻止该行为；iOS 10+ 用户仍可双指手动缩放，不影响可访问性。
  // 仅在 iOS 设备上注入，避免影响 Android Chrome 的手动缩放能力。
  if (!isIOSDevice()) return

  const viewport = document.querySelector('meta[name="viewport"]')
  if (!viewport) return

  const content = viewport.getAttribute('content') || ''
  if (/maximum-scale/i.test(content)) return
  viewport.setAttribute('content', `${content}, maximum-scale=1.0`)
}

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const shouldUseDark =
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
}

async function bootstrap() {
  // Apply theme class globally before app mount to keep all routes consistent.
  initThemeClass()
  initIOSViewportZoomFix()

  const app = createApp(App)
  const pinia = createPinia()
  app.use(pinia)

  // Initialize settings before mounting. Production uses a stable HTML shell;
  // the request has a short deadline and continues in the store after timeout.
  const appStore = useAppStore()
  await loadPublicSettingsBeforeMount(appStore)

  // Set document title immediately after config is loaded
  if (appStore.siteName && appStore.siteName !== 'Sub2API') {
    document.title = `${appStore.siteName} - AI API Gateway`
  }
  updateFavicon(appStore.siteLogo)

  await initI18n()

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
}

bootstrap()
