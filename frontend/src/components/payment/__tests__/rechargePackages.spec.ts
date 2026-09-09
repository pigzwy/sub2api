import { describe, expect, it } from 'vitest'
import {
  creditedRechargeAmount,
  filterRechargePackages,
  isUsdtPaymentMethod,
  maxRechargeBonus,
  paymentMethodLane,
  rechargeBonusAmount,
} from '@/components/payment/rechargePackages'

describe('rechargePackages', () => {
  it('keeps package amounts inside the checkout min/max window', () => {
    expect(filterRechargePackages(100, 2000).map((pkg) => pkg.amount)).toEqual([100, 500, 1000, 1500, 2000])
  })

  it('derives bonus credit from the existing recharge multiplier only', () => {
    expect(creditedRechargeAmount(100, 1.049)).toBe(104.9)
    expect(rechargeBonusAmount(100, 1.049)).toBe(4.9)
    expect(rechargeBonusAmount(50, 1)).toBe(0)
    expect(maxRechargeBonus(filterRechargePackages(0, 0), 1.05)).toBe(250)
  })

  it('classifies USDT methods without treating Stripe USD as crypto', () => {
    expect(isUsdtPaymentMethod('usdt_trc20', 'USDT')).toBe(true)
    expect(isUsdtPaymentMethod('infini', 'USD')).toBe(true)
    expect(isUsdtPaymentMethod('stripe', 'USD')).toBe(false)
    expect(paymentMethodLane('alipay', 'CNY')).toBe('rmb')
    expect(paymentMethodLane('usdt_trc20')).toBe('usdt')
  })
})
