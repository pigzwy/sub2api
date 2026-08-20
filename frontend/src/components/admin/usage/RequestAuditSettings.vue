<template>
  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ localText('请求审计设置', 'Request Audit Settings') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ localText('记录网关请求体和响应体，便于排查异常请求。默认关闭，开启前请确认数据合规要求。', 'Record gateway request and response bodies for troubleshooting. Disabled by default; verify compliance requirements before enabling.') }}
        </p>
      </div>
      <Toggle v-model="form.request_audit_enabled" />
    </div>

    <div v-if="form.request_audit_enabled" class="mt-4 space-y-4">
      <div class="grid gap-4 md:grid-cols-2">
        <div>
          <label class="input-label">{{ localText('保留时长', 'Retention') }}</label>
          <Select v-model="form.request_audit_retention_hours" :options="retentionOptions" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ localText('超过保留时长的审计日志会在写入新日志时清理；关闭自动清理时仍可手动清理。', 'Logs older than the retention period are cleaned when new logs are written. Manual cleanup remains available when auto cleanup is off.') }}
          </p>
        </div>

        <div>
          <label class="input-label">{{ localText('分组范围', 'Group Scope') }}</label>
          <Select
            v-model="groupPicker"
            :options="groupOptions"
            searchable
            clearable
            :placeholder="localText('选择分组', 'Select group')"
            @change="addGroupScope"
          />
          <div v-if="selectedGroups.length > 0" class="mt-2 flex flex-wrap gap-2">
            <span
              v-for="group in selectedGroups"
              :key="group.id"
              class="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200"
            >
              {{ group.name }}
              <button type="button" class="text-gray-400 hover:text-red-500" @click="removeGroupScope(group.id)">
                <Icon name="x" size="xs" />
              </button>
            </span>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ localText('不选分组表示不限分组；同时设置用户范围时取交集。', 'Leave empty for all groups. If user scope is also set, both scopes must match.') }}
          </p>
        </div>
      </div>

      <div ref="userSearchRef" class="relative">
        <label class="input-label">{{ localText('用户范围', 'User Scope') }}</label>
        <input
          v-model="userKeyword"
          type="text"
          class="input pr-8"
          :placeholder="localText('按邮箱搜索并添加用户', 'Search users by email')"
          @input="debounceUserSearch"
          @focus="showUserDropdown = true"
        />
        <div
          v-if="showUserDropdown && (userResults.length > 0 || userKeyword)"
          class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
        >
          <button
            v-for="user in userResults"
            :key="user.id"
            type="button"
            class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
            @click="addUserScope(user)"
          >
            <span>{{ user.email }}<span v-if="user.deleted" class="ml-1 text-xs text-gray-400">{{ localText('（已删除）', ' (deleted)') }}</span></span>
            <span class="ml-2 text-xs text-gray-400">#{{ user.id }}</span>
          </button>
          <div v-if="userResults.length === 0" class="px-4 py-2 text-sm text-gray-500 dark:text-gray-400">
            {{ localText('未找到用户', 'No users found') }}
          </div>
        </div>
        <div v-if="selectedUsers.length > 0" class="mt-2 flex flex-wrap gap-2">
          <span
            v-for="user in selectedUsers"
            :key="user.id"
            class="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200"
          >
            {{ user.email }}
            <button type="button" class="text-gray-400 hover:text-red-500" @click="removeUserScope(user.id)">
              <Icon name="x" size="xs" />
            </button>
          </span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ localText('不选用户表示不限用户。请只对必要用户或分组开启，避免长期保存敏感内容。', 'Leave empty for all users. Enable only for required users or groups to avoid storing sensitive content long-term.') }}
        </p>
      </div>
    </div>

    <div class="mt-4 flex items-center justify-end gap-2">
      <span v-if="saved" class="text-xs text-green-600 dark:text-green-400">
        {{ localText('已保存', 'Saved') }}
      </span>
      <button type="button" class="btn btn-primary btn-sm" :disabled="saving || loading" @click="save">
        {{ saving ? localText('保存中…', 'Saving…') : localText('保存', 'Save') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 请求审计的设置就近放在用量页的审计面板里：看日志和配「谁被记录、留多久」
 * 属于同一件事，隔在系统设置里会让人两头找。
 *
 * 只提交这四个 key —— 后端 UpdateSettings 按请求里实际出现的字段决定写哪些，
 * 省略的保持原值，所以不需要把整份系统设置读回来再整体回写。
 */
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { adminAPI } from '@/api'
import Icon from '@/components/common/Icon.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'
import type { AdminGroup } from '@/types'
import type { SimpleUser } from '@/api/admin/usage'

const emit = defineEmits<{ (e: 'saved', enabled: boolean): void }>()

const { locale } = useI18n()
const appStore = useAppStore()

const localText = (zh: string, en: string) => (locale.value === 'zh' ? zh : en)

const loading = ref(true)
const saving = ref(false)
const saved = ref(false)

const form = reactive({
  request_audit_enabled: false,
  request_audit_retention_hours: 0 as number,
  request_audit_group_scope: [] as number[],
  request_audit_user_scope: [] as number[],
})

const retentionOptions = computed(() => [
  { value: 0, label: localText('关闭自动清理', 'No auto cleanup') },
  { value: 1, label: localText('1 小时', '1 hour') },
  { value: 6, label: localText('6 小时', '6 hours') },
  { value: 24, label: localText('24 小时', '24 hours') },
  { value: 24 * 7, label: localText('7 天', '7 days') },
  { value: 24 * 30, label: localText('30 天', '30 days') },
])

const groups = ref<AdminGroup[]>([])
const groupPicker = ref<number | null>(null)
const userSearchRef = ref<HTMLElement | null>(null)
const userKeyword = ref('')
const userResults = ref<SimpleUser[]>([])
const knownUsers = ref<SimpleUser[]>([])
const showUserDropdown = ref(false)
let userSearchTimer: ReturnType<typeof setTimeout> | null = null

function normalizeNumberArray(value: unknown): number[] {
  if (!Array.isArray(value)) return []
  return Array.from(
    new Set(value.map((item) => Number(item)).filter((item) => Number.isInteger(item) && item > 0)),
  )
}

const groupOptions = computed(() =>
  groups.value
    .filter((group) => !form.request_audit_group_scope.includes(group.id))
    .map((group) => ({ value: group.id, label: group.name })),
)

const selectedGroups = computed(() => {
  const selected = new Set(form.request_audit_group_scope)
  return groups.value.filter((group) => selected.has(group.id))
})

const selectedUsers = computed(() => {
  const byID = new Map(knownUsers.value.map((user) => [user.id, user]))
  return form.request_audit_user_scope.map(
    (id) => byID.get(id) || ({ id, email: `#${id}`, deleted: false } as SimpleUser),
  )
})

function addGroupScope(value: string | number | boolean | null) {
  const id = Number(value)
  if (Number.isInteger(id) && id > 0 && !form.request_audit_group_scope.includes(id)) {
    form.request_audit_group_scope.push(id)
  }
  groupPicker.value = null
}

function removeGroupScope(id: number) {
  form.request_audit_group_scope = form.request_audit_group_scope.filter((item) => item !== id)
}

function debounceUserSearch() {
  if (userSearchTimer) clearTimeout(userSearchTimer)
  userSearchTimer = setTimeout(async () => {
    const keyword = userKeyword.value.trim()
    if (!keyword) {
      userResults.value = []
      return
    }
    try {
      const selected = new Set(form.request_audit_user_scope)
      const results = await adminAPI.usage.searchUsers(keyword)
      userResults.value = results
        .filter((user) => !selected.has(user.id))
        .sort((a, b) => Number(a.deleted) - Number(b.deleted))
    } catch {
      userResults.value = []
    }
  }, 300)
}

function addUserScope(user: SimpleUser) {
  if (!form.request_audit_user_scope.includes(user.id)) {
    form.request_audit_user_scope.push(user.id)
  }
  if (!knownUsers.value.some((item) => item.id === user.id)) {
    knownUsers.value.push(user)
  }
  userKeyword.value = ''
  userResults.value = []
  showUserDropdown.value = false
}

function removeUserScope(id: number) {
  form.request_audit_user_scope = form.request_audit_user_scope.filter((item) => item !== id)
  knownUsers.value = knownUsers.value.filter((user) => user.id !== id)
}

async function hydrateSelectedUsers() {
  const ids = form.request_audit_user_scope
  const existing = new Map(knownUsers.value.map((user) => [user.id, user]))
  const missing = ids.filter((id) => !existing.has(id))
  if (missing.length === 0) return
  const loaded = await Promise.all(
    missing.map(async (id) => {
      try {
        const user = await adminAPI.users.getById(id, true)
        return { id: user.id, email: user.email || `#${id}`, deleted: Boolean(user.deleted_at) } as SimpleUser
      } catch {
        return { id, email: `#${id}`, deleted: false } as SimpleUser
      }
    }),
  )
  knownUsers.value = [...knownUsers.value, ...loaded]
}

function handleDocumentClick(event: MouseEvent) {
  const target = event.target as Node | null
  if (!target || userSearchRef.value?.contains(target)) return
  showUserDropdown.value = false
}

async function load() {
  loading.value = true
  try {
    const [settings, groupList] = await Promise.all([
      adminAPI.settings.getSettings(),
      adminAPI.groups.getAll().catch(() => [] as AdminGroup[]),
    ])
    groups.value = groupList
    form.request_audit_enabled = settings.request_audit_enabled === true
    form.request_audit_retention_hours = Number(settings.request_audit_retention_hours) || 0
    form.request_audit_group_scope = normalizeNumberArray(settings.request_audit_group_scope)
    form.request_audit_user_scope = normalizeNumberArray(settings.request_audit_user_scope)
    await hydrateSelectedUsers()
  } catch {
    // 读不到就保持默认，面板本身仍可用。
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  saved.value = false
  try {
    await adminAPI.settings.updateSettings({
      request_audit_enabled: form.request_audit_enabled,
      request_audit_retention_hours: Number(form.request_audit_retention_hours) || 0,
      request_audit_group_scope: normalizeNumberArray(form.request_audit_group_scope),
      request_audit_user_scope: normalizeNumberArray(form.request_audit_user_scope),
    })
    saved.value = true
    setTimeout(() => (saved.value = false), 2000)
    emit('saved', form.request_audit_enabled)
  } catch (error: unknown) {
    appStore.showError(
      (error as { message?: string })?.message ||
        localText('保存请求审计设置失败', 'Failed to save request audit settings'),
    )
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
  void load()
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleDocumentClick)
  if (userSearchTimer) clearTimeout(userSearchTimer)
})
</script>
