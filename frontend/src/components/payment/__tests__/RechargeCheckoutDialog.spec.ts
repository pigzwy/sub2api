import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RechargeCheckoutDialog from '@/components/payment/RechargeCheckoutDialog.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}))

describe('RechargeCheckoutDialog', () => {
  it('shows the RMB/USDT switch only when both lanes exist', async () => {
    const wrapper = mount(RechargeCheckoutDialog, {
      props: {
        open: true,
        payAmountLabel: '¥50.00',
        creditAmountLabel: '$50.00',
        feeAmountLabel: '¥0.00',
        feeRate: 0,
        multiplier: 1,
        currency: 'CNY',
        lane: 'rmb',
        rmbMethods: [{ type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true }],
        usdtMethods: [{ type: 'usdt_trc20', display_name: 'USDT', fee_rate: 0, available: true }],
        submitting: false,
      },
    })

    expect(document.body.querySelector('[data-testid="pay-lane-usdt"]')).not.toBeNull()
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-alipay"]')?.click()
    expect(wrapper.emitted('confirm')?.[0]).toEqual(['alipay'])
    await document.body.querySelector<HTMLButtonElement>('[data-testid="pay-lane-usdt"]')?.click()
    expect(wrapper.emitted('update:lane')?.[0]).toEqual(['usdt'])
    wrapper.unmount()
  })

  it('hides the lane switch when only RMB methods are configured', () => {
    const wrapper = mount(RechargeCheckoutDialog, {
      props: {
        open: true,
        payAmountLabel: '¥50.00',
        creditAmountLabel: '$50.00',
        feeAmountLabel: '¥0.00',
        feeRate: 0,
        multiplier: 1,
        currency: 'CNY',
        lane: 'rmb',
        rmbMethods: [{ type: 'stripe', display_name: 'Stripe', fee_rate: 0, available: true }],
        usdtMethods: [],
        submitting: false,
      },
    })

    expect(document.body.querySelector('[data-testid="pay-lane-usdt"]')).toBeNull()
    expect(document.body.querySelector('[data-testid="checkout-method-stripe"]')?.textContent).toContain('Stripe')
    wrapper.unmount()
  })
})
