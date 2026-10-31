import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import FirstOutputProtectionFields from '../FirstOutputProtectionFields.vue'
import { readFirstOutputProtectionConfig, writeFirstOutputProtectionConfig } from '../firstOutputProtection'
import zhAccounts from '@/i18n/locales/zh/admin/accounts'

function render() {
  return mount(defineComponent({
    components: { FirstOutputProtectionFields },
    setup() { return { enabled: ref(true), config: ref(readFirstOutputProtectionConfig()) } },
    template: '<FirstOutputProtectionFields v-model="enabled" v-model:config="config" />'
  }), { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: { admin: zhAccounts } } })] } })
}

describe('first-output settings', () => {
  it('renders defaults and updates the rule explanation using all three fields', async () => {
    const wrapper = render()
    expect(wrapper.text()).toContain('超过 30 秒')
    for (const [field, value] of [['limitSeconds', 45], ['cooldownMinutes', 12], ['windowMinutes', 20]] as const) {
      await wrapper.get(`[data-testid="first-output-${field}"]`).setValue(value)
    }
    expect(wrapper.text()).toContain('超过 45 秒')
    expect(wrapper.text()).toContain('20 分钟内达到 3 次')
    expect(wrapper.text()).toContain('账号 12 分钟')
    expect(wrapper.text()).toContain('低于 45 秒立即清零')
    await wrapper.get('[data-testid="first-output-windowMinutes"]').setValue('')
    expect(wrapper.get('[data-testid="first-output-windowMinutes"]').attributes('aria-invalid')).toBe('true')
    expect(wrapper.text()).not.toContain('超过 45 秒')
    await wrapper.get('[data-testid="first-output-protection"]').setValue(false)
    expect(wrapper.findAll('input[type="number"]')).toHaveLength(0)
    wrapper.unmount()
  })

  it('loads old accounts with defaults and rejects incomplete or fractional configurations', () => {
    expect(readFirstOutputProtectionConfig()).toEqual({ limitSeconds: 30, cooldownMinutes: 5, windowMinutes: 10 })
    for (const value of ['', 0, -1, 1.5, 3601] as const) {
      const credentials = { api_key: 'kept' }
      expect(writeFirstOutputProtectionConfig(credentials, true, { ...readFirstOutputProtectionConfig(), limitSeconds: value })).toBe(false)
      expect(credentials).toEqual({ api_key: 'kept' })
    }
  })
})
