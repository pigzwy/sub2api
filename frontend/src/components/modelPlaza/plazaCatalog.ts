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

/** 分类标签与「分组管理」平台名同一套，不把平台名伪装成分组名。 */
const PLATFORM_I18N_KEY: Record<string, string> = {
  anthropic: 'admin.groups.platforms.anthropic',
  openai: 'admin.groups.platforms.openai',
  grok: 'admin.groups.platforms.grok',
  gemini: 'admin.groups.platforms.gemini',
  zhipu: 'admin.groups.platforms.zhipu',
  kimi: 'admin.groups.platforms.kimi',
  deepseek: 'admin.groups.platforms.deepseek',
  antigravity: 'admin.groups.platforms.antigravity',
  composite: 'admin.groups.platforms.composite',
}

export function plazaTabLabel(platform: string, t: (key: string) => string): string {
  const key = PLATFORM_I18N_KEY[platform]
  if (!key) return platform
  const label = t(key)
  return label === key ? platform : label
}
