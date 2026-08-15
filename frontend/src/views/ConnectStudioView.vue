<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-dark-950">
    <div class="text-center">
      <div
        class="mx-auto mb-3 h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
      ></div>
      <p class="text-sm text-gray-500 dark:text-gray-400">正在进入创作台…</p>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Studio(创作台)免登录接力点(公开路由,不挂 requiresAuth):
 * - 本站已登录:带当前 access token 跳入 Studio(其 ?token= 入口会落盘并复验)
 * - 未登录:带 ?sso=miss 弹回 Studio 的账号密码登录页(不劫持到本站登录页)
 * 网关菜单 iframe、独立 tab、官网首页入口统一指向本路由;
 * Studio 侧无会话时也会自动跳到本路由探测,实现"已登录 sub2 即秒进"。
 */
import { onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'

const STUDIO_URL = 'https://chat.pigcode.ai/studio'
const AUTH_TOKEN_KEY = 'auth_token'

const authStore = useAuthStore()

onMounted(() => {
  // store 可能尚未完成初始化,localStorage 兜底读取
  const token = authStore.token || localStorage.getItem(AUTH_TOKEN_KEY) || ''
  window.location.replace(
    token ? `${STUDIO_URL}?token=${encodeURIComponent(token)}` : `${STUDIO_URL}?sso=miss`
  )
})
</script>
