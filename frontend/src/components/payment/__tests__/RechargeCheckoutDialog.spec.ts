import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RechargeCheckoutDialog from '@/components/payment/RechargeCheckoutDialog.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}))

function mountDialog(overrides: Record<string, unknown> = {}) {
  return mount(RechargeCheckoutDialog, {
    props: {
      open: true,
      payAmountLabel: '¥50.00',
      creditAmountLabel: '$50.00',
      feeAmountLabel: '¥0.00',
      feeRate: 0,
      multiplier: 1,
      currency: 'CNY',
      selected: 'alipay',
      lane: 'rmb',
      rmbMethods: [{ type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true }],
      usdtMethods: [{ type: 'usdt_trc20', display_name: 'USDT', fee_rate: 0, available: true }],
      submitting: false,
      ...overrides,
    },
  })
}

describe('RechargeCheckoutDialog', () => {
  it('selects a method without confirming payment', async () => {
    const wrapper = mountDialog()

    expect(document.body.querySelector('[data-testid="pay-lane-usdt"]')).not.toBeNull()
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-alipay"]')?.click()
    expect(wrapper.emitted('select')?.[0]).toEqual(['alipay'])
    expect(wrapper.emitted('confirm')).toBeUndefined()
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-confirm"]')?.click()
    expect(wrapper.emitted('confirm')?.[0]).toEqual(['alipay'])
    await document.body.querySelector<HTMLButtonElement>('[data-testid="pay-lane-usdt"]')?.click()
    expect(wrapper.emitted('update:lane')?.[0]).toEqual(['usdt'])
    wrapper.unmount()
  })

  it('hides the lane switch when only RMB methods are configured', () => {
    const wrapper = mountDialog({
      selected: 'stripe',
      rmbMethods: [{ type: 'stripe', display_name: 'Stripe', fee_rate: 0, available: true }],
      usdtMethods: [],
    })

    expect(document.body.querySelector('[data-testid="pay-lane-usdt"]')).toBeNull()
    expect(document.body.querySelector('[data-testid="checkout-method-stripe"]')?.textContent).toContain('Stripe')
    wrapper.unmount()
  })

  it('does not confirm when the selected method is unavailable or submit is in flight', async () => {
    const unavailable = mountDialog({
      selected: 'stripe',
      rmbMethods: [
        { type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true },
        { type: 'stripe', display_name: 'Stripe', fee_rate: 0, available: false },
      ],
      usdtMethods: [],
    })
    expect(document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-confirm"]')?.disabled).toBe(true)
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-stripe"]')?.click()
    expect(unavailable.emitted('select')).toBeUndefined()
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-confirm"]')?.click()
    expect(unavailable.emitted('confirm')).toBeUndefined()
    unavailable.unmount()

    const submitting = mountDialog({ submitting: true })
    expect(document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-confirm"]')?.disabled).toBe(true)
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-alipay"]')?.click()
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-confirm"]')?.click()
    expect(submitting.emitted('select')).toBeUndefined()
    expect(submitting.emitted('confirm')).toBeUndefined()
    submitting.unmount()
  })
})
