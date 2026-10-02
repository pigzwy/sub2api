import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import RechargeBonusSettings from '../RechargeBonusSettings.vue'
import zh from '@/i18n/locales/zh'

const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh } })

describe('RechargeBonusSettings', () => {
  it('retains unsaved tiers across mode switches and keeps empty input invalid', async () => {
    const wrapper = mount(RechargeBonusSettings, {
      props: { mode: 'percentage', tiers: [{ min_amount: 200, bonus_percent: 20 }] },
      global: { plugins: [i18n] },
    })
    await wrapper.get('[data-testid="bonus-percent"]').setValue('25')
    await wrapper.get('[data-testid="recharge-bonus-mode"]').setValue('fixed')
    expect(wrapper.emitted('update:mode')?.[0]).toEqual(['fixed'])
    await wrapper.setProps({ mode: 'fixed' })
    expect(wrapper.find('[data-testid="bonus-percent"]').exists()).toBe(false)
    await wrapper.setProps({ mode: 'percentage' })
    expect((wrapper.get('[data-testid="bonus-percent"]').element as HTMLInputElement).value).toBe('25')
    const threshold = wrapper.get('[data-testid="bonus-threshold"]')
    await threshold.setValue('')
    expect((threshold.element as HTMLInputElement).validity.valueMissing).toBe(true)
    expect(wrapper.props('tiers')[0]!.min_amount).toBe('') // must not silently coerce to an all-amount threshold
    wrapper.unmount()
  })
})
