<template>
  <!-- 功能关闭（或接口拒绝）时整块不渲染，首页不留空卡片。 -->
  <div v-if="snapshot" class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('checkin.title') }}</h2>
    </div>

    <div class="p-4">
      <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/50">
        <div class="flex items-center gap-4">
          <div
            class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl"
            :class="snapshot.signed_today
              ? 'bg-emerald-100 dark:bg-emerald-900/30'
              : 'bg-primary-100 dark:bg-primary-900/30'"
          >
            <Icon
              name="calendar"
              size="lg"
              :class="snapshot.signed_today
                ? 'text-emerald-600 dark:text-emerald-400'
                : 'text-primary-600 dark:text-primary-400'"
            />
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ snapshot.signed_today ? t('checkin.signedHint') : t('checkin.readyHint') }}
            </p>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
              {{ t('checkin.summary', { days: snapshot.total_days, amount: formatCurrency(snapshot.total_amount) }) }}
            </p>
          </div>
        </div>

        <button
          class="btn btn-primary mt-4 w-full"
          :disabled="snapshot.signed_today || submitting || captchaPending"
          @click="onCheckinClick"
        >
          {{ buttonLabel }}
        </button>

        <!-- 验证码只在管理员要求、且用户点了签到之后才挂载：常驻挂载会让每个
             打开首页的人都被挑战一次。 -->
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
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import CaptchaChallenge from '@/components/CaptchaChallenge.vue'
import { useAppStore } from '@/stores'
import { getCheckin, submitCheckin, type CheckinSnapshot } from '@/api/user'
import { extractApiErrorMessage } from '@/utils/apiError'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const emit = defineEmits<{ (e: 'checked-in'): void }>()

const { t } = useI18n()
const appStore = useAppStore()

const snapshot = ref<CheckinSnapshot | null>(null)
const submitting = ref(false)
const captchaPending = ref(false)
const captchaRef = ref<{ reset: () => void } | null>(null)

// 验证码配置取自公开设置，与登录页同源；CaptchaChallenge 自行挑选启用的那家。
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

const buttonLabel = computed(() => {
  if (submitting.value) return t('checkin.submitting')
  if (snapshot.value?.signed_today) return t('checkin.alreadySigned')
  if (captchaPending.value) return t('checkin.verifyFirst')
  return t('checkin.action')
})

function formatCurrency(value: number | undefined): string {
  return `$${(value ?? 0).toFixed(2)}`
}

async function load() {
  // 功能关闭时直接跳过：否则每个打开首页的用户都会发一次注定 404 的请求。
  if (!isFeatureFlagEnabled(FeatureFlags.checkin)) {
    snapshot.value = null
    return
  }
  try {
    snapshot.value = await getCheckin()
  } catch {
    // 功能关闭时接口返回 404；此时整块不渲染，无需提示。
    snapshot.value = null
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
  void doCheckin(randstr ? { tencent_ticket: token, tencent_randstr: randstr } : { captcha_token: token })
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
    if (result.snapshot) {
      snapshot.value = result.snapshot
    } else {
      await load()
    }
    appStore.showSuccess(t('checkin.rewardGranted', { amount: formatCurrency(result.amount) }))
    // 余额已变，让首页统计跟着刷新。
    emit('checked-in')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('checkin.failed')))
    // 重新拉取，让「今日已签到」这类结果立刻反映到按钮上。
    await load()
  } finally {
    submitting.value = false
    captchaPending.value = false
  }
}

onMounted(load)
</script>
