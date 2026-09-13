import { describe, expect, it } from 'vitest'
import {
  DEFAULT_RECHARGE_PACKAGES,
  balanceGatewayPayAmount,
  creditedRechargeAmount,
  filterRechargePackages,
  isUsdtPaymentMethod,
  maxRechargeBonus,
  packageBonusAmount,
  packageCreditAmount,
  paymentMethodLane,
  rechargeBonusAmount,
  resolveRechargePackages,
  resolveUsdtUsdToCnyRate,
  shouldConvertBalancePayAmountToUsd,
} from '@/components/payment/rechargePackages'

describe('rechargePackages', () => {
  it('keeps package amounts inside the checkout min/max window', () => {
    expect(filterRechargePackages(DEFAULT_RECHARGE_PACKAGES, 100, 2000).map((pkg) => pkg.amount)).toEqual([100, 500, 1000, 1500, 2000])
  })

  it('derives bonus credit from the existing recharge multiplier only', () => {
    expect(creditedRechargeAmount(100, 1.049)).toBe(104.9)
    expect(rechargeBonusAmount(100, 1.049)).toBe(4.9)
    expect(rechargeBonusAmount(50, 1)).toBe(0)
    expect(maxRechargeBonus(filterRechargePackages(DEFAULT_RECHARGE_PACKAGES, 0, 0), 1.05)).toBe(0)
  })

  it('adds per-package USD bonus on top of the recharge rate', () => {
    const packages = resolveRechargePackages([
      { id: 'starter', amount: 50, bonus: 0, name: '体验', description: '' },
      { id: 'standard', amount: 100, bonus: 2.99, name: '标准', description: '' },
    ])
    expect(packageCreditAmount(packages[0], packages, 1.05)).toBe(52.5)
    expect(packageBonusAmount(packages[0], packages, 1.05)).toBe(0)
    expect(packageCreditAmount(packages[1], packages, 1.05)).toBe(107.99)
    expect(maxRechargeBonus(packages, 1.05)).toBe(2.99)
  })

  it('keeps the CNY conversion rate when another package has a gift', () => {
    const packages = resolveRechargePackages([
      { id: 'cny100', amount: 100, bonus: 0, name: '基础', description: '' },
      { id: 'cny200', amount: 200, bonus: 5, name: '加赠', description: '' },
    ])
    expect(packageCreditAmount(packages[0], packages, 0.14)).toBe(14)
    expect(packageCreditAmount(packages[1], packages, 0.14)).toBe(33)
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

  it('converts Infini/USDT recharge pay amounts by the USD/CNY rate', () => {
    expect(shouldConvertBalancePayAmountToUsd('infini', 'USD', 6.67)).toBe(true)
    expect(shouldConvertBalancePayAmountToUsd('usdt_trc20', 'USDT', 6.67)).toBe(true)
    expect(shouldConvertBalancePayAmountToUsd('stripe', 'USD', 6.67)).toBe(false)
    expect(shouldConvertBalancePayAmountToUsd('alipay', 'CNY', 6.67)).toBe(false)
    expect(shouldConvertBalancePayAmountToUsd('infini', 'USD', 0)).toBe(false)
    expect(balanceGatewayPayAmount(50, 'infini', 'USD', 6.67)).toBe(7.5)
    expect(balanceGatewayPayAmount(50, 'infini', 'CNY', 6.67)).toBe(7.5)
    expect(balanceGatewayPayAmount(50, 'usdt_trc20', 'USDT', 6.67)).toBe(7.5)
    expect(balanceGatewayPayAmount(50, 'infini', 'USD', 0)).toBe(50)
    expect(balanceGatewayPayAmount(50, 'stripe', 'USD', 6.67)).toBe(50)
    expect(balanceGatewayPayAmount(50, 'alipay', 'CNY', 6.67)).toBe(50)
  })

  it('prefers the dedicated USDT rate and only falls back to the subscription rate', () => {
    expect(resolveUsdtUsdToCnyRate(6.67, 7.15)).toBe(6.67)
    expect(resolveUsdtUsdToCnyRate(0, 7.15)).toBe(7.15)
    expect(resolveUsdtUsdToCnyRate(0, 0)).toBe(0)
    expect(balanceGatewayPayAmount(50, 'infini', 'USD', resolveUsdtUsdToCnyRate(6.67, 7.15))).toBe(7.5)
    expect(balanceGatewayPayAmount(50, 'infini', 'USD', resolveUsdtUsdToCnyRate(0, 7.15))).toBe(6.99)
  })
})
