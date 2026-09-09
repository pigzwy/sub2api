import { describe, expect, it } from 'vitest'
import {
  DEFAULT_RECHARGE_PACKAGES,
  creditedRechargeAmount,
  filterRechargePackages,
  isUsdtPaymentMethod,
  maxRechargeBonus,
  packageBonusAmount,
  packageCreditAmount,
  paymentMethodLane,
  rechargeBonusAmount,
  resolveRechargePackages,
} from '@/components/payment/rechargePackages'

describe('rechargePackages', () => {
  it('keeps package amounts inside the checkout min/max window', () => {
    expect(filterRechargePackages(DEFAULT_RECHARGE_PACKAGES, 100, 2000).map((pkg) => pkg.amount)).toEqual([100, 500, 1000, 1500, 2000])
  })

  it('derives bonus credit from the existing recharge multiplier only', () => {
    expect(creditedRechargeAmount(100, 1.049)).toBe(104.9)
    expect(rechargeBonusAmount(100, 1.049)).toBe(4.9)
    expect(rechargeBonusAmount(50, 1)).toBe(0)
    expect(maxRechargeBonus(filterRechargePackages(DEFAULT_RECHARGE_PACKAGES, 0, 0), 1.05)).toBe(250)
  })

  it('uses per-package bonus when any gift is configured', () => {
    const packages = resolveRechargePackages([
      { id: 'starter', amount: 50, bonus: 0, name: '体验', description: '' },
      { id: 'standard', amount: 100, bonus: 2.99, name: '标准', description: '' },
    ])
    expect(packageCreditAmount(packages[0], packages, 1.05)).toBe(50)
    expect(packageBonusAmount(packages[0], packages, 1.05)).toBe(0)
    expect(packageCreditAmount(packages[1], packages, 1.05)).toBe(102.99)
    expect(maxRechargeBonus(packages, 1.05)).toBe(2.99)
  })

  it('falls back to built-in packages when checkout returns none', () => {
    expect(resolveRechargePackages([]).map((pkg) => pkg.amount)).toEqual(
      DEFAULT_RECHARGE_PACKAGES.map((pkg) => pkg.amount),
    )
  })

  it('classifies USDT methods without treating Stripe USD as crypto', () => {
    expect(isUsdtPaymentMethod('usdt_trc20', 'USDT')).toBe(true)
    expect(isUsdtPaymentMethod('infini', 'USD')).toBe(true)
    expect(isUsdtPaymentMethod('stripe', 'USD')).toBe(false)
    expect(paymentMethodLane('alipay', 'CNY')).toBe('rmb')
    expect(paymentMethodLane('usdt_trc20')).toBe('usdt')
  })
})
