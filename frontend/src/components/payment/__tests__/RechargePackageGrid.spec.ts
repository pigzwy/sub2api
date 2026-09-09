import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import RechargePackageGrid from '../RechargePackageGrid.vue'
import { formatPaymentNumber } from '../currency'

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { template: '<span />' },
}))

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: {
    en: {
      payment: {
        bestValue: 'Best value',
        popular: 'Popular',
        bonusTag: '+{amount} bonus',
        getCredit: 'Get {amount} credit',
        getCreditLead: 'Get',
        getCreditTrail: 'credit',
        neverExpires: 'Never expires',
        allModels: 'All models',
        rechargeNow: 'Recharge now',
      },
    },
  },
})

describe('RechargePackageGrid', () => {
  it('shows package prices with currency fraction digits instead of rounding to integers', () => {
    const wrapper = mount(RechargePackageGrid, {
      props: {
        packages: [
          { id: 'decimal', amount: 10.49, bonus: 0, name: 'Decimal', description: '' },
          { id: 'half', amount: 10.5, bonus: 0, name: 'Half', description: '' },
        ],
        multiplier: 1,
        currency: 'CNY',
      },
      global: { plugins: [i18n] },
    })

    expect(wrapper.get('[data-testid="recharge-package-10.49"]').text()).toContain(formatPaymentNumber(10.49, 'CNY'))
    expect(wrapper.get('[data-testid="recharge-package-10.5"]').text()).toContain(formatPaymentNumber(10.5, 'CNY'))
    expect(wrapper.get('[data-testid="recharge-package-10.49"]').text()).not.toContain('¥10\n')
    expect(wrapper.get('[data-testid="recharge-package-10.5"]').text()).not.toContain('11')
  })
})
