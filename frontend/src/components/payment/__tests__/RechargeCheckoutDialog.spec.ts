import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RechargeCheckoutDialog from '@/components/payment/RechargeCheckoutDialog.vue'
import enMisc from '@/i18n/locales/en/misc'
import zhMisc from '@/i18n/locales/zh/misc'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: string | Record<string, unknown>) => {
      if (params && typeof params === 'object' && 'amount' in params) {
        return `${key}:${String(params.amount)}`
      }
      return typeof params === 'string' ? params : key
    },
  }),
}))

function mountDialog(overrides: Record<string, unknown> = {}) {
  return mount(RechargeCheckoutDialog, {
    props: {
      open: true,
      payAmountLabel: '¥50.00',
      creditAmountLabel: '$50.00',
      extraBonusLabel: '',
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

afterEach(() => {
  document.body.querySelectorAll('[data-testid="recharge-checkout-dialog"]').forEach((el) => el.remove())
})

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

  it('uses a transparent method surface so brand marks stay visible', () => {
    const wrapper = mountDialog({
      selected: 'stripe',
      rmbMethods: [
        { type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true },
        { type: 'stripe', display_name: 'Stripe', fee_rate: 0, available: true },
      ],
      usdtMethods: [],
    })

    const alipay = document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-alipay"]')
    const stripe = document.body.querySelector<HTMLButtonElement>('[data-testid="checkout-method-stripe"]')
    expect(alipay?.getAttribute('data-surface')).toBe('transparent')
    expect(stripe?.getAttribute('data-surface')).toBe('transparent')
    expect(alipay?.className).toContain('bg-transparent')
    expect(stripe?.className).toContain('bg-transparent')
    expect(alipay?.className).not.toMatch(/btn-alipay|btn-stripe|btn-primary/)
    expect(stripe?.className).not.toMatch(/btn-alipay|btn-stripe|btn-primary/)
    expect(alipay?.querySelector('img')?.getAttribute('alt')).toBe('Alipay')
    expect(stripe?.querySelector('img')?.getAttribute('alt')).toBe('Stripe')
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

  it('shows Extra beside the base credit and hides it when there is no bonus', () => {
    const withBonus = mountDialog({
      payAmountLabel: '¥100.00',
      creditAmountLabel: '$100.00',
      extraBonusLabel: '$2.99',
    })

    const extra = document.body.querySelector('[data-testid="extra-bonus"]')
    expect(extra).not.toBeNull()
    expect(extra?.textContent).toContain('payment.extraBonus:$2.99')
    expect(document.body.textContent).toContain('payment.creditedBalance $100.00')
    expect(document.body.textContent).not.toContain('$102.99')
    withBonus.unmount()

    const withoutBonus = mountDialog({ extraBonusLabel: '' })
    expect(document.body.querySelector('[data-testid="extra-bonus"]')).toBeNull()
    withoutBonus.unmount()
  })

  it('localizes the Extra badge as +$2.99+送 in Chinese and Extra $2.99 in English', () => {
    expect(zhMisc.payment.extraBonus).toBe('+{amount}+送')
    expect(enMisc.payment.extraBonus).toBe('Extra {amount}')
    expect(zhMisc.payment.extraBonus.replace('{amount}', '$2.99')).toBe('+$2.99+送')
    expect(enMisc.payment.extraBonus.replace('{amount}', '$2.99')).toBe('Extra $2.99')
  })

  it('centers the accepted-brand row at the bottom of the dialog', () => {
    const wrapper = mountDialog({
      error: 'quote failed',
      rmbMethods: [
        { type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true },
        { type: 'wxpay', display_name: 'WeChat Pay', fee_rate: 0, available: true },
      ],
    })

    const dialog = document.body.querySelector('[data-testid="recharge-checkout-dialog"]')
    const brands = dialog?.querySelector('[data-testid="supported-methods"]')
    expect(brands).not.toBeNull()
    expect(brands?.className).toContain('justify-center')
    expect(brands?.textContent).toContain('payment.supportedMethods')
    expect(brands?.getAttribute('data-lane')).toBe('rmb')
    expect(brands?.querySelectorAll('img')).toHaveLength(1)
    expect(Array.from(brands?.querySelectorAll('img') ?? []).map((img) => img.getAttribute('alt'))).toEqual([
      'Alipay',
    ])

    const children = Array.from(dialog?.firstElementChild?.children ?? [])
    expect(children.at(-1)).toBe(brands)
    expect(dialog?.textContent?.indexOf('quote failed') ?? -1).toBeLessThan(
      dialog?.textContent?.indexOf('payment.supportedMethods') ?? -1,
    )
    wrapper.unmount()
  })

  it('shows only the configured Infini mark in the USDT lane', async () => {
    const wrapper = mountDialog({
      selected: 'infini',
      lane: 'usdt',
      rmbMethods: [{ type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true }],
      usdtMethods: [{ type: 'infini', display_name: 'INFINI Stablecoin Payment', fee_rate: 0, available: true }],
    })

    await wrapper.setProps({ lane: 'usdt' })
    const brands = document.body.querySelector('[data-testid="supported-methods"]')
    expect(brands?.getAttribute('data-lane')).toBe('usdt')
    expect(Array.from(brands?.querySelectorAll('img') ?? []).map((img) => img.getAttribute('alt'))).toEqual(['Infini'])
    wrapper.unmount()
  })

  it('shows Stripe card marks only when Stripe is an enabled RMB method', () => {
    const wrapper = mountDialog({
      selected: 'stripe',
      rmbMethods: [
        { type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true },
        { type: 'stripe', display_name: 'Stripe', fee_rate: 0, available: true },
      ],
      usdtMethods: [],
    })

    const brands = document.body.querySelector('[data-testid="supported-methods"]')
    expect(Array.from(brands?.querySelectorAll('img') ?? []).map((img) => img.getAttribute('alt'))).toEqual([
      'Alipay',
      'Visa',
      'Mastercard',
      'Apple Pay',
      'USD',
    ])
    wrapper.unmount()
  })

  it('hides the footer when the current lane has no matching brand marks', () => {
    const wrapper = mountDialog({
      selected: 'usdt_trc20',
      lane: 'usdt',
      rmbMethods: [{ type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true }],
      usdtMethods: [{ type: 'usdt_trc20', display_name: 'USDT', fee_rate: 0, available: true }],
    })

    expect(document.body.querySelector('[data-testid="supported-methods"]')).toBeNull()
    wrapper.unmount()
  })
})
