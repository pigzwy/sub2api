export type RechargePackageId =
  | 'starter'
  | 'standard'
  | 'advanced'
  | 'pro'
  | 'team'
  | 'business'
  | 'premium'
  | 'enterprise'

export type RechargePackageBadge = 'popular' | 'bestValue'

export interface RechargePackage {
  id: RechargePackageId
  amount: number
  badge?: RechargePackageBadge
}

export const RECHARGE_PACKAGES: RechargePackage[] = [
  { id: 'starter', amount: 50 },
  { id: 'standard', amount: 100 },
  { id: 'advanced', amount: 500, badge: 'popular' },
  { id: 'pro', amount: 1000, badge: 'bestValue' },
  { id: 'team', amount: 1500 },
  { id: 'business', amount: 2000 },
  { id: 'premium', amount: 3000 },
  { id: 'enterprise', amount: 5000 },
]

export type PaymentMethodLane = 'rmb' | 'usdt'

export function normalizeRechargeMultiplier(multiplier: number): number {
  return Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
}

export function creditedRechargeAmount(amount: number, multiplier: number): number {
  return Math.round(amount * normalizeRechargeMultiplier(multiplier) * 100) / 100
}

export function rechargeBonusAmount(amount: number, multiplier: number): number {
  return Math.round((creditedRechargeAmount(amount, multiplier) - amount) * 100) / 100
}

export function filterRechargePackages(min: number, max: number): RechargePackage[] {
  return RECHARGE_PACKAGES.filter((pkg) =>
    (min <= 0 || pkg.amount >= min) && (max <= 0 || pkg.amount <= max),
  )
}

export function maxRechargeBonus(packages: RechargePackage[], multiplier: number): number {
  return packages.reduce((max, pkg) => Math.max(max, rechargeBonusAmount(pkg.amount, multiplier)), 0)
}

export function isUsdtPaymentMethod(type: string, currency?: string | null): boolean {
  const normalizedType = type.trim().toLowerCase()
  const normalizedCurrency = String(currency || '').trim().toUpperCase()
  return normalizedCurrency === 'USDT'
    || normalizedType.includes('usdt')
    || normalizedType.includes('infini')
    || normalizedType.includes('stablecoin')
}

export function paymentMethodLane(type: string, currency?: string | null): PaymentMethodLane {
  return isUsdtPaymentMethod(type, currency) ? 'usdt' : 'rmb'
}
