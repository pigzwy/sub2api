<template>
  <!-- 功能关闭或未登录时整块不渲染，头部不留占位。 -->
  <div
    v-if="snapshot"
    class="relative"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
  >
    <button
      type="button"
      class="flex items-center gap-1.5 rounded-full border px-2.5 py-1.5 transition-colors"
      :class="
        snapshot.signed_today
          ? 'border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400'
          : 'border-primary-200 bg-primary-50 text-primary-700 hover:bg-primary-100 dark:border-primary-800 dark:bg-primary-900/30 dark:text-primary-300'
      "
      :title="t('checkin.title')"
      @click="onTriggerClick"
    >
      <Icon name="calendar" size="sm" />
      <span class="hidden text-sm font-medium sm:inline">{{
        t("checkin.title")
      }}</span>
      <!-- 未签到时的小红点：头部空间有限，用它把「今天还没领」这件事说清楚。 -->
      <span
        v-if="!snapshot.signed_today"
        class="h-1.5 w-1.5 rounded-full bg-red-500"
      ></span>
    </button>

    <transition name="dropdown">
      <!-- 外层只负责定位与那段间距：间距用 padding 而非 margin，触发按钮和面板之间
           因此没有"空隙"，鼠标下移途中不会离开本元素、把面板关掉。 -->
      <div v-if="open" class="absolute right-0 top-full z-50 w-64 pt-2">
        <div
          class="rounded-lg border border-gray-200 bg-white p-3 shadow-lg dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="flex items-center justify-between">
            <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
              t("checkin.title")
            }}</span>
            <span
              class="rounded-full px-2 py-0.5 text-[11px] font-medium"
              :class="
                snapshot.signed_today
                  ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                  : 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
              "
            >
              {{
                snapshot.signed_today
                  ? t("checkin.alreadySigned")
                  : t("checkin.badgeAvailable")
              }}
            </span>
          </div>

          <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">
            {{
              snapshot.signed_today
                ? t("checkin.signedHint")
                : t("checkin.readyHint")
            }}
          </p>

          <div
            class="mt-3 space-y-1.5 border-t border-gray-100 pt-2.5 text-xs dark:border-dark-700"
          >
            <div class="flex items-center justify-between">
              <span class="text-gray-500 dark:text-dark-400">{{
                t("checkin.totalDays")
              }}</span>
              <span class="font-medium text-gray-900 dark:text-white">{{
                snapshot.total_days
              }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-gray-500 dark:text-dark-400">{{
                t("checkin.totalAmount")
              }}</span>
              <span class="font-medium text-emerald-600 dark:text-emerald-400">
                {{ formatCurrency(snapshot.total_amount) }}
              </span>
            </div>
          </div>

          <button
            class="btn btn-primary btn-sm mt-3 w-full"
            :disabled="snapshot.signed_today || submitting || captchaPending"
            @click="onCheckinClick"
          >
            {{ buttonLabel }}
          </button>

          <!-- 需要人机验证时才挂载，且面板会被钉住（onLeave 不再收起），
             否则鼠标一移开验证码就没了。 -->
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
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "@/components/icons/Icon.vue";
import CaptchaChallenge from "@/components/CaptchaChallenge.vue";
import { useAppStore } from "@/stores";
import { getCheckin, submitCheckin, type CheckinSnapshot } from "@/api/user";
import { extractApiErrorMessage } from "@/utils/apiError";
import { FeatureFlags, isFeatureFlagEnabled } from "@/utils/featureFlags";

const emit = defineEmits<{ (e: "checked-in"): void }>();

const { t } = useI18n();
const appStore = useAppStore();

const snapshot = ref<CheckinSnapshot | null>(null);
const open = ref(false);
// 点开或进入验证码流程后钉住面板，避免鼠标移开就关掉正在进行的操作。
const pinned = ref(false);
const submitting = ref(false);
const captchaPending = ref(false);
const captchaRef = ref<{ reset: () => void } | null>(null);

const publicSettings = computed(() => appStore.cachedPublicSettings);
const turnstileEnabled = computed(
  () => publicSettings.value?.turnstile_enabled === true,
);
const turnstileSiteKey = computed(
  () => publicSettings.value?.turnstile_site_key ?? "",
);
const tencentCaptchaEnabled = computed(
  () => publicSettings.value?.tencent_captcha_enabled === true,
);
const tencentCaptchaAppId = computed(
  () => publicSettings.value?.tencent_captcha_app_id ?? "",
);
const tencentCaptchaRegion = computed(
  () => publicSettings.value?.tencent_captcha_region ?? "cn",
);
const aliyunCaptchaEnabled = computed(
  () => publicSettings.value?.aliyun_captcha_enabled === true,
);
const aliyunCaptchaSceneId = computed(
  () => publicSettings.value?.aliyun_captcha_scene_id ?? "",
);
const aliyunCaptchaPrefix = computed(
  () => publicSettings.value?.aliyun_captcha_prefix ?? "",
);
const aliyunCaptchaRegion = computed(
  () => publicSettings.value?.aliyun_captcha_region ?? "cn",
);

const buttonLabel = computed(() => {
  if (submitting.value) return t("checkin.submitting");
  if (snapshot.value?.signed_today) return t("checkin.alreadySigned");
  if (captchaPending.value) return t("checkin.verifyFirst");
  return t("checkin.action");
});

function formatCurrency(value: number | undefined): string {
  return `$${(value ?? 0).toFixed(2)}`;
}

// 关闭留 200ms 缓冲：padding 桥接已经消除了按钮与面板之间的空隙，这层延迟
// 再兜住"鼠标从边缘划出去又回来"的情况，避免面板在够到之前就消失。
let closeTimer: ReturnType<typeof setTimeout> | null = null;

function cancelClose() {
  if (closeTimer) {
    clearTimeout(closeTimer);
    closeTimer = null;
  }
}

function onEnter() {
  cancelClose();
  open.value = true;
}

function onLeave() {
  if (pinned.value) return;
  cancelClose();
  closeTimer = setTimeout(() => {
    open.value = false;
    closeTimer = null;
  }, 200);
}

// 触摸设备没有 hover，点击也要能打开；再点一次收起。
function onTriggerClick() {
  if (open.value && !pinned.value) {
    open.value = false;
    return;
  }
  open.value = true;
}

async function load() {
  if (!isFeatureFlagEnabled(FeatureFlags.checkin)) {
    snapshot.value = null;
    return;
  }
  try {
    snapshot.value = await getCheckin();
  } catch {
    // 未登录或功能关闭时接口拒绝，此时整块不渲染。
    snapshot.value = null;
  }
}

function onCheckinClick() {
  if (snapshot.value?.captcha_enabled) {
    captchaPending.value = true;
    pinned.value = true;
    return;
  }
  void doCheckin({});
}

function onCaptchaVerified(token: string, randstr?: string) {
  void doCheckin(
    randstr
      ? { tencent_ticket: token, tencent_randstr: randstr }
      : { captcha_token: token },
  );
}

function onCaptchaError() {
  captchaPending.value = false;
  pinned.value = false;
  appStore.showError(t("checkin.captchaFailed"));
}

function onCaptchaExpire() {
  captchaRef.value?.reset();
}

async function doCheckin(proof: Record<string, string>) {
  submitting.value = true;
  pinned.value = true;
  try {
    const result = await submitCheckin(proof);
    if (result.snapshot) {
      snapshot.value = result.snapshot;
    } else {
      await load();
    }
    appStore.showSuccess(
      t("checkin.rewardGranted", { amount: formatCurrency(result.amount) }),
    );
    emit("checked-in");
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t("checkin.failed")));
    await load();
  } finally {
    submitting.value = false;
    captchaPending.value = false;
    pinned.value = false;
  }
}

onMounted(load);
onBeforeUnmount(cancelClose);
</script>
