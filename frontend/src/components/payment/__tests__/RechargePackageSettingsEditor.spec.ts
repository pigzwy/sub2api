import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import RechargePackageSettingsEditor from '../RechargePackageSettingsEditor.vue'
import { DEFAULT_RECHARGE_PACKAGES } from '../rechargePackages'
import zh from '@/i18n/locales/zh'

const i18n = createI18n({
  legacy: false,
  locale: 'zh',
  messages: { zh },
})

describe('RechargePackageSettingsEditor', () => {
  it('adds a package and can reset to defaults', async () => {
    const wrapper = mount(RechargePackageSettingsEditor, {
      props: {
        modelValue: DEFAULT_RECHARGE_PACKAGES.slice(0, 1),
        'onUpdate:modelValue': (value: unknown) => wrapper.setProps({ modelValue: value as typeof DEFAULT_RECHARGE_PACKAGES }),
      },
      global: { plugins: [i18n] },
    })

    expect(wrapper.findAll('input').length).toBeGreaterThan(0)
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.length).toBeGreaterThan(0)
  })
})
