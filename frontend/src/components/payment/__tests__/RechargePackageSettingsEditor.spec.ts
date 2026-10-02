import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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
  it('preserves a fixed gift while percentage mode disables its input', async () => {
    const packages = [{ id: 'test', amount: 200, bonus: 5, name: 'Test', description: '' }]
    const wrapper = mount(RechargePackageSettingsEditor, {
      props: { modelValue: packages, bonusDisabled: true },
      global: { plugins: [i18n] },
    })
    const bonus = wrapper.findAll('input[type="number"]')[1]!
    expect((bonus.element as HTMLInputElement).disabled).toBe(true)
    expect((bonus.element as HTMLInputElement).value).toBe('5')
    expect(wrapper.text()).toContain(i18n.global.t('admin.settings.payment.fixedBonusInactive'))
    await wrapper.setProps({ bonusDisabled: false })
    expect((bonus.element as HTMLInputElement).disabled).toBe(false)
    expect(packages[0]!.bonus).toBe(5)
    wrapper.unmount()
  })

  it('adds a package and can reset to defaults', async () => {
    window.localStorage.clear()
    const wrapper = mount(RechargePackageSettingsEditor, {
      props: {
        modelValue: DEFAULT_RECHARGE_PACKAGES.slice(0, 1),
        'onUpdate:modelValue': (value: unknown) => wrapper.setProps({ modelValue: value as typeof DEFAULT_RECHARGE_PACKAGES }),
      },
      global: { plugins: [i18n] },
    })

    expect(wrapper.get('[data-testid="recharge-package-toggle"]').attributes('aria-expanded')).toBe('false')
    await wrapper.get('[data-testid="recharge-package-toggle"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="recharge-package-toggle"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    await wrapper.get('[data-testid="recharge-package-add"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toHaveLength(2)
  })
})
