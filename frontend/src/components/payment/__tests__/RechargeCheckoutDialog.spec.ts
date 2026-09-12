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
  it('starts payment when a method button is clicked', async () => {
    const wrapper = mountDialog()

    expect(document.body.querySelector('[data-testid="pay-lane-usdt"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="checkout-confirm"]')).toBeNull()
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-alipay"]')?.click()
    expect(wrapper.emitted('select')?.[0]).toEqual(['alipay'])
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

  it('shows Infini on the USDT lane as a hosted checkout method', () => {
    const wrapper = mountDialog({
      selected: 'infini',
      lane: 'usdt',
      currency: 'USD',
      payAmountLabel: '$50.00',
      rmbMethods: [{ type: 'stripe', display_name: 'Stripe', fee_rate: 0, available: true }],
      usdtMethods: [{ type: 'infini', display_name: 'INFINI Stablecoin Payment', fee_rate: 0, available: true }],
    })

    expect(document.body.querySelector('[data-testid="checkout-method-infini"]')?.textContent).toContain('INFINI Stablecoin Payment')
    expect(document.body.querySelector('[data-testid="checkout-confirm"]')).toBeNull()
    wrapper.unmount()
  })

  it('does not start payment when the method is unavailable or submit is in flight', async () => {
    const unavailable = mountDialog({
      selected: 'stripe',
      rmbMethods: [
        { type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true },
        { type: 'stripe', display_name: 'Stripe', fee_rate: 0, available: false },
      ],
      usdtMethods: [],
    })
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-stripe"]')?.click()
    expect(unavailable.emitted('select')).toBeUndefined()
    expect(unavailable.emitted('confirm')).toBeUndefined()
    unavailable.unmount()

    const submitting = mountDialog({ submitting: true })
    expect(document.body.querySelector('[data-testid="checkout-method-alipay"]')?.textContent).toContain('common.processing')
    await document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-alipay"]')?.click()
    expect(submitting.emitted('select')).toBeUndefined()
    expect(submitting.emitted('confirm')).toBeUndefined()
    submitting.unmount()
  })

  it('renders the accepted-brand row at the bottom of the dialog', () => {
    const wrapper = mountDialog({
      error: 'quote failed',
    })

    const dialog = document.body.querySelector('[data-testid="recharge-checkout-dialog"]')
    const brands = dialog?.querySelector('[data-testid="supported-methods"]')
    expect(brands).not.toBeNull()
    expect(brands?.textContent).toContain('payment.supportedMethods')
    expect(brands?.getAttribute('data-lane')).toBe('rmb')
    expect(brands?.querySelectorAll('img')).toHaveLength(6)
    expect(Array.from(brands?.querySelectorAll('img') ?? []).map((img) => img.getAttribute('alt'))).toEqual([
      'Alipay',
      'WeChat Pay',
      'Visa',
      'Mastercard',
      'Apple Pay',
      'USD',
    ])

    const children = Array.from(dialog?.firstElementChild?.children ?? [])
    expect(children.at(-1)).toBe(brands)
    expect(dialog?.textContent?.indexOf('quote failed') ?? -1).toBeLessThan(
      dialog?.textContent?.indexOf('payment.supportedMethods') ?? -1,
    )
    wrapper.unmount()
  })

  it('swaps the footer to USDT chain marks when the USDT lane is selected', () => {
    const wrapper = mountDialog({
      selected: 'infini',
      lane: 'usdt',
      rmbMethods: [{ type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true }],
      usdtMethods: [{ type: 'infini', display_name: 'INFINI Stablecoin Payment', fee_rate: 0, available: true }],
    })

    const brands = document.body.querySelector('[data-testid="supported-methods"]')
    expect(brands?.getAttribute('data-lane')).toBe('usdt')
    expect(Array.from(brands?.querySelectorAll('img') ?? []).map((img) => img.getAttribute('alt'))).toEqual([
      'TRON',
      'Ethereum',
      'BNB Chain',
      'Polygon',
      'Solana',
      'Arbitrum',
      'Base',
    ])
    wrapper.unmount()
  })
})
