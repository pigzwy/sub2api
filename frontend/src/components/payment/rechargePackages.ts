import type { RechargePackage, RechargePackageBadge } from '@/types/payment'

export type { RechargePackage, RechargePackageBadge }

export const DEFAULT_RECHARGE_PACKAGES: RechargePackage[] = [
  { id: 'starter', amount: 50, name: '体验', description: '适合初次体验', name_en: 'Starter', description_en: 'For a first try' },
  { id: 'standard', amount: 100, name: '标准', description: '开发者常用', name_en: 'Standard', description_en: 'Popular with developers' },
  { id: 'advanced', amount: 500, badge: 'popular', name: '进阶', description: '进阶用户首选', name_en: 'Advanced', description_en: 'For growing usage' },
  { id: 'pro', amount: 1000, badge: 'bestValue', name: '专业', description: '专业团队推荐', name_en: 'Pro', description_en: 'For professional teams' },
  { id: 'team', amount: 1500, name: '团队', description: '小团队协作', name_en: 'Team', description_en: 'For small teams' },
  { id: 'business', amount: 2000, name: '商务', description: '商务规模使用', name_en: 'Business', description_en: 'For business workloads' },
  { id: 'premium', amount: 3000, name: '尊享', description: '高频重度使用', name_en: 'Premium', description_en: 'For heavy usage' },
  { id: 'enterprise', amount: 5000, name: '企业', description: '企业级用量', name_en: 'Enterprise', description_en: 'For enterprise volume' },
]

/** @deprecated Use DEFAULT_RECHARGE_PACKAGES; kept for existing imports */
export const RECHARGE_PACKAGES = DEFAULT_RECHARGE_PACKAGES

export type PaymentMethodLane = 'rmb' | 'usdt'

function roundMoney(value: number): number {
  return Math.round(value * 100) / 100
}

export function normalizeRechargeMultiplier(multiplier: number): number {
  return Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
}

export function creditedRechargeAmount(amount: number, multiplier: number): number {
  return roundMoney(amount * normalizeRechargeMultiplier(multiplier))
}

export function rechargeBonusAmount(amount: number, multiplier: number): number {
  return roundMoney(creditedRechargeAmount(amount, multiplier) - amount)
}

export function packageCreditAmount(
  pkg: RechargePackage,
  _packages: RechargePackage[],
  multiplier: number,
): number {
  if (Number.isFinite(pkg.credit)) {
    return roundMoney(Number(pkg.credit))
  }
  return roundMoney(creditedRechargeAmount(pkg.amount, multiplier) + (Number(pkg.bonus) || 0))
}

export function packageBonusAmount(
  pkg: RechargePackage,
  _packages: RechargePackage[] = [],
  _multiplier = 1,
): number {
  return roundMoney(Number(pkg.bonus) || 0)
}

export function filterRechargePackages(
  packages: RechargePackage[],
  min: number,
  max: number,
): RechargePackage[] {
  return packages.filter((pkg) =>
    (min <= 0 || pkg.amount >= min) && (max <= 0 || pkg.amount <= max),
  )
}

export function maxRechargeBonus(
  packages: RechargePackage[],
  multiplier: number,
): number {
  return packages.reduce((max, pkg) => Math.max(max, packageBonusAmount(pkg, packages, multiplier)), 0)
}

export function resolveRechargePackages(packages?: RechargePackage[] | null): RechargePackage[] {
  if (!Array.isArray(packages) || packages.length === 0) {
    return DEFAULT_RECHARGE_PACKAGES.map((pkg) => ({ ...pkg }))
  }
  return packages
    .filter((pkg) => pkg && Number.isFinite(pkg.amount) && pkg.amount > 0 && String(pkg.name || '').trim())
    .map((pkg) => ({
      id: String(pkg.id || `pkg-${Math.round(pkg.amount * 100)}`),
      amount: Number(pkg.amount),
      bonus: Number(pkg.bonus) || 0,
      credit: Number.isFinite(pkg.credit) ? Number(pkg.credit) : undefined,
      badge: pkg.badge === 'popular' || pkg.badge === 'bestValue' ? pkg.badge : '',
      name: String(pkg.name || '').trim(),
      description: String(pkg.description || '').trim(),
      name_en: String(pkg.name_en || '').trim() || undefined,
      description_en: String(pkg.description_en || '').trim() || undefined,
    }))
}

export function localizedPackageName(pkg: RechargePackage, locale?: string): string {
  if (String(locale || '').toLowerCase().startsWith('en') && pkg.name_en) {
    return pkg.name_en
  }
  return pkg.name
}

export function localizedPackageDescription(pkg: RechargePackage, locale?: string): string {
  if (String(locale || '').toLowerCase().startsWith('en') && pkg.description_en) {
    return pkg.description_en
  }
  return pkg.description
}

export function isUsdtPaymentMethod(type: string, currency?: string | null): boolean {
  const normalizedType = type.trim().toLowerCase()
  const normalizedCurrency = String(currency || '').trim().toUpperCase()
  return normalizedCurrency === 'USDT'
    || normalizedType.includes('usdt')
    || normalizedType.includes('infini')
    || normalizedType.includes('stablecoin')
}

// paymentMethodLane only separates USDT/crypto from everything else.
// The "rmb" lane can still mix CNY and USD methods (e.g. Alipay + Stripe).
export function paymentMethodLane(type: string, currency?: string | null): PaymentMethodLane {
  return isUsdtPaymentMethod(type, currency) ? 'usdt' : 'rmb'
}

export function createEmptyRechargePackage(): RechargePackage {
  return {
    id: `pkg-${Date.now().toString(36)}`,
    amount: 50,
    bonus: 0,
    badge: '',
    name: '',
    description: '',
    name_en: '',
    description_en: '',
  }
}
