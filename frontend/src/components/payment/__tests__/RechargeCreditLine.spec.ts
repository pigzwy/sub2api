import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import RechargeCreditLine from '../RechargeCreditLine.vue'

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: {
    en: {
      payment: {
        getCredit: 'Get {amount} credit',
        getCreditLead: 'Get',
        getCreditTrail: 'credit',
      },
    },
  },
})

describe('RechargeCreditLine', () => {
  it('keeps a plain credit sentence when there is no bonus', () => {
    const wrapper = mount(RechargeCreditLine, {
      props: { creditAmount: 50, bonus: 0 },
      global: { plugins: [i18n] },
    })

    expect(wrapper.find('s').exists()).toBe(false)
    expect(wrapper.text().replace(/\s+/g, ' ')).toContain('$50.00')
    expect(wrapper.find('.text-amber-500').exists()).toBe(false)
  })

  it('highlights only the credited total when there is a bonus', () => {
    const wrapper = mount(RechargeCreditLine, {
      props: { creditAmount: 102.99, bonus: 2.99 },
      global: { plugins: [i18n] },
    })

    expect(wrapper.find('s').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('$100.00')
    expect(wrapper.get('.text-amber-500').text()).toBe('$102.99')
    expect(wrapper.classes()).toContain('whitespace-nowrap')
  })
})
