/** 官方价按 $1 = ¥7 展示；分组价 = 官方美元 × 倍率（与参考页 ÷7 后同一口径）。 */
export const PLAZA_USD_TO_CNY = 7

/** 分组倍率换算成常见「X 折」展示（0.8x → 8）。 */
export function formatZhe(rate: number): string {
  const z = Math.round(rate * 100) / 10
  return Number.isInteger(z) ? String(z) : z.toFixed(1)
}

/** 相对官方人民币的折数：0.8x → 1.1 折，2x → 2.9 折。 */
export function formatCatalogZhe(rate: number, fx = PLAZA_USD_TO_CNY): string {
  return formatZhe(rate / fx)
}

function round2(value: number): number {
  return Math.round(value * 100) / 100
}

export function perMillionUsd(perToken: number | null | undefined): number | null {
  if (perToken == null) return null
  return perToken * 1_000_000
}

export function groupYuan(perToken: number | null | undefined, rate: number): number | null {
  const usd = perMillionUsd(perToken)
  if (usd == null) return null
  return round2(usd * rate)
}

export function officialYuan(perToken: number | null | undefined, fx = PLAZA_USD_TO_CNY): number | null {
  const usd = perMillionUsd(perToken)
  if (usd == null) return null
  return round2(usd * fx)
}

export function requestGroupYuan(perUnit: number | null | undefined, rate: number): number | null {
  if (perUnit == null) return null
  return round2(perUnit * rate)
}

export function requestOfficialYuan(perUnit: number | null | undefined, fx = PLAZA_USD_TO_CNY): number | null {
  if (perUnit == null) return null
  return round2(perUnit * fx)
}

export function savingsPercent(group: number | null, official: number | null): number | null {
  if (group == null || official == null || official <= 0 || group >= official) return null
  return Math.round((1 - group / official) * 100)
}

export function formatYuan(amount: number | null): string {
  if (amount == null) return '-'
  return `¥${amount.toFixed(2)}`
}

/** 分类 Tab 顺序与参考页一致；只展示数据里实际出现的平台。 */
export const PLAZA_PLATFORM_ORDER = [
  'anthropic',
  'openai',
  'grok',
  'gemini',
  'zhipu',
  'kimi',
  'deepseek',
  'antigravity',
  'composite',
] as const

export function sortPlazaPlatforms(platforms: string[]): string[] {
  return [...platforms].sort((a, b) => {
    const ai = PLAZA_PLATFORM_ORDER.indexOf(a as (typeof PLAZA_PLATFORM_ORDER)[number])
    const bi = PLAZA_PLATFORM_ORDER.indexOf(b as (typeof PLAZA_PLATFORM_ORDER)[number])
    return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi) || a.localeCompare(b)
  })
}

/** 分类用参考页品牌名（Claude / ChatGPT）；分组卡片仍只用后台分组 name。 */
const PLATFORM_I18N_KEY: Record<string, string> = {
  anthropic: 'modelPlaza.catalog.platforms.anthropic',
  openai: 'modelPlaza.catalog.platforms.openai',
  grok: 'modelPlaza.catalog.platforms.grok',
  gemini: 'modelPlaza.catalog.platforms.gemini',
  zhipu: 'modelPlaza.catalog.platforms.zhipu',
  kimi: 'modelPlaza.catalog.platforms.kimi',
  deepseek: 'modelPlaza.catalog.platforms.deepseek',
  antigravity: 'modelPlaza.catalog.platforms.antigravity',
  composite: 'modelPlaza.catalog.platforms.composite',
}

export function plazaTabLabel(platform: string, t: (key: string) => string): string {
  const key = PLATFORM_I18N_KEY[platform]
  if (!key) return platform
  const label = t(key)
  return label === key ? platform : label
}
