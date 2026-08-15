<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <div v-else-if="!snapshot" class="card flex flex-col items-center p-10 text-center">
        <div
          class="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
        >
          <Icon name="calendar" size="lg" class="text-gray-400" />
        </div>
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('checkin.disabledTitle') }}
        </h3>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('checkin.disabledDesc') }}
        </p>
      </div>

      <template v-else>
        <!-- 左栏固定窄宽、右栏日历自适应：整页限宽，避免宽屏下卡片被拉得又空又大。 -->
        <div class="mx-auto grid w-full max-w-5xl gap-4 lg:grid-cols-[300px_minmax(0,1fr)]">
          <!-- 左：签到操作 + 统计 -->
          <div class="space-y-4">
            <div class="card p-5">
              <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
                {{ t('checkin.stats.balance') }}
              </p>
              <p class="mt-1 text-3xl font-bold tracking-tight text-primary-600 dark:text-primary-400">
                {{ formatCurrency(snapshot.balance) }}
              </p>

              <div class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700">
                <p class="text-xs text-gray-500 dark:text-dark-400">
                  {{ t('checkin.rewardRange', { min: formatCurrency(snapshot.min_amount), max: formatCurrency(snapshot.max_amount) }) }}
                </p>
                <button
                  class="btn btn-primary mt-3 w-full"
                  :disabled="snapshot.signed_today || submitting || captchaPending"
                  @click="onCheckinClick"
                >
                  <Icon v-if="!submitting" name="calendar" size="sm" />
                  <span>{{ checkinButtonLabel }}</span>
                </button>

                <!-- 验证码只在管理员要求、且用户点了签到之后才挂载，
                     常驻挂载会让每个打开页面的人都被挑战一次。 -->
                <div v-if="captchaPending" class="mt-3 flex justify-center">
                  <CaptchaChallenge
                    ref="captchaRef"
                    :turnstile-enabled="turnstileEnabled"
                    :turnstile-site-key="turnstileSiteKey"
                    :tencent-enabled="tencentCaptchaEnabled"
                    :tencent-app-id="tencentCaptchaAppId"
                    :tencent-region="tencentCaptchaRegion"
                    :aliyun-enabled="aliyunCaptchaEnabled"
                    :aliyun-scene-id="aliyunCaptchaSceneId"
                    :aliyun-prefix="aliyunCaptchaPrefix"
                    :aliyun-region="aliyunCaptchaRegion"
                    @verify="onCaptchaVerified"
                    @error="onCaptchaError"
                    @expire="onCaptchaExpire"
                  />
                </div>

                <p
                  v-if="lastReward"
                  class="mt-3 rounded-lg bg-emerald-50 px-3 py-2 text-center text-xs font-medium text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300"
                >
                  {{ t('checkin.rewardGranted', { amount: formatCurrency(lastReward) }) }}
                </p>
              </div>
            </div>

            <div class="card divide-y divide-gray-100 p-0 dark:divide-dark-700">
              <div class="flex items-center justify-between px-5 py-3">
                <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('checkin.stats.monthDays') }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ snapshot.month_signed_days }}</span>
              </div>
              <div class="flex items-center justify-between px-5 py-3">
                <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('checkin.stats.totalDays') }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ snapshot.total_days }}</span>
              </div>
              <div class="flex items-center justify-between px-5 py-3">
                <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('checkin.stats.totalAmount') }}</span>
                <span class="text-sm font-semibold text-emerald-600 dark:text-emerald-400">
                  {{ formatCurrency(snapshot.total_amount) }}
                </span>
              </div>
            </div>
          </div>

          <!-- 右：当月日历 -->
          <div class="card p-5">
            <div class="flex items-baseline justify-between">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('checkin.calendarTitle') }}
              </h3>
              <span class="text-xs text-gray-400 dark:text-dark-500">{{ snapshot.year_month }}</span>
            </div>

            <div class="mt-4 grid grid-cols-7 gap-1 text-center">
              <div
                v-for="label in weekdayLabels"
                :key="label"
                class="pb-1 text-[11px] font-medium text-gray-400 dark:text-dark-500"
              >
                {{ label }}
              </div>
            </div>

            <div class="mt-1 grid grid-cols-7 gap-1">
              <!-- 前置空格，让 1 号落在真实星期上。 -->
              <div v-for="blank in leadingBlanks" :key="`blank-${blank}`"></div>
              <div
                v-for="day in snapshot.days"
                :key="day.date"
                class="flex h-11 flex-col items-center justify-center rounded-lg border text-xs transition-colors"
                :class="dayClass(day)"
                :title="day.signed ? `${day.date}  +${formatCurrency(day.amount)}` : day.date"
              >
                <span class="font-medium leading-none">{{ day.day }}</span>
                <span v-if="day.signed" class="mt-0.5 text-[10px] leading-none opacity-80">
                  {{ formatCurrency(day.amount) }}
                </span>
              </div>
            </div>

            <div class="mt-4 flex items-center gap-4 border-t border-gray-100 pt-3 text-[11px] text-gray-500 dark:border-dark-700 dark:text-dark-400">
              <span class="flex items-center gap-1.5">
                <span class="h-2.5 w-2.5 rounded-sm border border-emerald-200 bg-emerald-50 dark:border-emerald-800 dark:bg-emerald-900/30"></span>
                {{ t('checkin.legendSigned') }}
              </span>
              <span class="flex items-center gap-1.5">
                <span class="h-2.5 w-2.5 rounded-sm border border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/30"></span>
                {{ t('checkin.legendToday') }}
              </span>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import CaptchaChallenge from '@/components/CaptchaChallenge.vue'
import { useAppStore } from '@/stores'
import { getCheckin, submitCheckin, type CheckinDay, type CheckinSnapshot } from '@/api/user'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t, locale } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const submitting = ref(false)
const snapshot = ref<CheckinSnapshot | null>(null)
const lastReward = ref<number | null>(null)
const captchaPending = ref(false)
const captchaRef = ref<{ reset: () => void } | null>(null)

// Captcha provider settings come from public settings, the same source the auth
// pages read; CaptchaChallenge picks whichever provider is enabled.
const publicSettings = computed(() => appStore.cachedPublicSettings)
const turnstileEnabled = computed(() => publicSettings.value?.turnstile_enabled === true)
const turnstileSiteKey = computed(() => publicSettings.value?.turnstile_site_key ?? '')
const tencentCaptchaEnabled = computed(() => publicSettings.value?.tencent_captcha_enabled === true)
const tencentCaptchaAppId = computed(() => publicSettings.value?.tencent_captcha_app_id ?? '')
const tencentCaptchaRegion = computed(() => publicSettings.value?.tencent_captcha_region ?? 'cn')
const aliyunCaptchaEnabled = computed(() => publicSettings.value?.aliyun_captcha_enabled === true)
const aliyunCaptchaSceneId = computed(() => publicSettings.value?.aliyun_captcha_scene_id ?? '')
const aliyunCaptchaPrefix = computed(() => publicSettings.value?.aliyun_captcha_prefix ?? '')
const aliyunCaptchaRegion = computed(() => publicSettings.value?.aliyun_captcha_region ?? 'cn')

const checkinButtonLabel = computed(() => {
  if (submitting.value) return t('checkin.submitting')
  if (snapshot.value?.signed_today) return t('checkin.alreadySigned')
  if (captchaPending.value) return t('checkin.verifyFirst')
  return t('checkin.action')
})

// 星期表头跟随界面语言，与日历首列（周日）对齐。
const weekdayLabels = computed(() => {
  const base = new Date(Date.UTC(2024, 0, 7)) // 2024-01-07 是周日
  return Array.from({ length: 7 }, (_, i) => {
    const d = new Date(base)
    d.setUTCDate(base.getUTCDate() + i)
    return new Intl.DateTimeFormat(locale.value, { weekday: 'narrow', timeZone: 'UTC' }).format(d)
  })
})

// The backend calendar starts at day 1; pad the grid so weekdays line up.
const leadingBlanks = computed(() => {
  const first = snapshot.value?.days?.[0]
  if (!first) return []
  const weekday = new Date(`${first.date}T00:00:00`).getDay()
  return Array.from({ length: weekday }, (_, index) => index)
})

function formatCurrency(value: number | undefined): string {
  return `$${(value ?? 0).toFixed(2)}`
}

function dayClass(day: CheckinDay): string {
  if (day.signed) {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-300'
  }
  if (day.date === snapshot.value?.today) {
    return 'border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-300'
  }
  return 'border-gray-200 text-gray-400 dark:border-dark-700 dark:text-dark-500'
}

async function load() {
  loading.value = true
  try {
    snapshot.value = await getCheckin()
  } catch {
    // A disabled feature answers 404; showing the empty state is the whole
    // response we need, so the error itself is not surfaced.
    snapshot.value = null
  } finally {
    loading.value = false
  }
}

function onCheckinClick() {
  if (snapshot.value?.captcha_enabled) {
    captchaPending.value = true
    return
  }
  void doCheckin({})
}

function onCaptchaVerified(token: string, randstr?: string) {
  void doCheckin(
    randstr
      ? { tencent_ticket: token, tencent_randstr: randstr }
      : { captcha_token: token },
  )
}

function onCaptchaError() {
  captchaPending.value = false
  appStore.showError(t('checkin.captchaFailed'))
}

function onCaptchaExpire() {
  captchaRef.value?.reset()
}

async function doCheckin(proof: Record<string, string>) {
  submitting.value = true
  try {
    const result = await submitCheckin(proof)
    lastReward.value = result.amount
    if (result.snapshot) {
      snapshot.value = result.snapshot
    } else {
      await load()
    }
    appStore.showSuccess(t('checkin.rewardGranted', { amount: formatCurrency(result.amount) }))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('checkin.failed')))
    // Refresh so an "already signed today" answer immediately disables the button.
    await load()
  } finally {
    submitting.value = false
    captchaPending.value = false
  }
}

onMounted(load)
</script>
