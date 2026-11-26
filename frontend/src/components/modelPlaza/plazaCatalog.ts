/** 官方价按 $1 = ¥1 展示；分组价 = 官方价 × 分组倍率。 */
export const PLAZA_USD_TO_CNY = 1

/** 分组倍率换算成常见「X 折」展示（0.8x → 8）。 */
export function formatZhe(rate: number): string {
  const z = Math.round(rate * 100) / 10
  return Number.isInteger(z) ? String(z) : z.toFixed(1)
}

/** 相对官方价的折数：官方价按 $1 = ¥1，故 0.8x → 8 折，1x → 10 折。 */
export function formatCatalogZhe(rate: number, fx = PLAZA_USD_TO_CNY): string {
  return formatZhe(rate / fx)
}

/**
 * 金额保留 2 位小数。不足 1 分的非零金额保留 1 位有效数字（0.0042 → 0.004），
 * 否则极低的单价（如低倍率分组的缓存读取）会被显示成 ¥0.00，看起来像免费。
 */
function round2(value: number): number {
  const magnitude = Math.abs(value)
  if (magnitude > 0 && magnitude < 0.01) return Number(value.toPrecision(1))
  return Math.round(value * 100) / 100
}

export function perMillionUsd(perToken: number | null | undefined): number | null {
  if (perToken == null) return null
  return perToken * 1_000_000
}

/** 未取整的分组价。折扣比较和「省多少」用它；展示用 groupYuan。 */
export function groupYuanExact(perToken: number | null | undefined, rate: number): number | null {
  const usd = perMillionUsd(perToken)
  return usd == null ? null : usd * rate
}

/** 未取整的官方价。折扣比较和「省多少」用它；展示用 officialYuan。 */
export function officialYuanExact(perToken: number | null | undefined, fx = PLAZA_USD_TO_CNY): number | null {
  const usd = perMillionUsd(perToken)
  return usd == null ? null : usd * fx
}

export function groupYuan(perToken: number | null | undefined, rate: number): number | null {
  const exact = groupYuanExact(perToken, rate)
  return exact == null ? null : round2(exact)
}

export function officialYuan(perToken: number | null | undefined, fx = PLAZA_USD_TO_CNY): number | null {
  const exact = officialYuanExact(perToken, fx)
  return exact == null ? null : round2(exact)
}

export function requestGroupYuan(perUnit: number | null | undefined, rate: number): number | null {
  if (perUnit == null) return null
  return round2(perUnit * rate)
}

export function requestOfficialYuan(perUnit: number | null | undefined, fx = PLAZA_USD_TO_CNY): number | null {
  if (perUnit == null) return null
  return round2(perUnit * fx)
}

/**
 * 分组价是否确实低于官方价。传未取整的价格：不足 1 分的价格取整后只剩 1 位有效数字，
 * 拿取整值比较会把 8 折算成 5 折，或把更便宜的价格判成相等。容差只用来吸收浮点误差。
 */
export function isCheaperPrice(group: number | null, official: number | null): boolean {
  if (group == null || official == null || official <= 0) return false
  return group < official * (1 - 1e-9)
}

/** 相对官方价省了百分之几；传未取整的价格。不足 1% 视为没有折扣，不显示「省 0%」。 */
export function savingsPercent(group: number | null, official: number | null): number | null {
  if (!isCheaperPrice(group, official)) return null
  const percent = Math.round((1 - group! / official!) * 100)
  return percent > 0 ? percent : null
}

export function formatYuan(amount: number | null): string {
  if (amount == null) return '-'
  const magnitude = Math.abs(amount)
  if (magnitude > 0 && magnitude < 0.01) {
    // 小数位数刚好够显示第一位有效数字，最多 8 位。
    const digits = Math.min(8, Math.ceil(-Math.log10(magnitude)))
    return `¥${amount.toFixed(digits)}`
  }
  return `¥${amount.toFixed(2)}`
}

/** 分类 Tab 顺序。Image/Video（composite）固定在最后；只展示数据里实际出现的平台。 */
export const PLAZA_PLATFORM_ORDER = [
  'anthropic',
  'openai',
  'grok',
  'gemini',
  'zhipu',
  'kimi',
  'deepseek',
  'antigravity',
] as const

export function sortPlazaPlatforms(platforms: string[]): string[] {
  const rank = (platform: string) => {
    if (platform === 'composite') return 10_000
    const index = PLAZA_PLATFORM_ORDER.indexOf(platform as (typeof PLAZA_PLATFORM_ORDER)[number])
    return index === -1 ? 99 : index
  }
  return [...platforms].sort((a, b) => rank(a) - rank(b) || a.localeCompare(b))
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
  minimax: 'modelPlaza.catalog.platforms.minimax',
  antigravity: 'modelPlaza.catalog.platforms.antigravity',
  composite: 'modelPlaza.catalog.platforms.composite',
}

export function plazaTabLabel(platform: string, t: (key: string) => string): string {
  const key = PLATFORM_I18N_KEY[platform]
  if (!key) return platform
  const label = t(key)
  return label === key ? platform : label
}

/** 从模型 id 抽出版本数字（`claude-fable-5-1` → [5, 1]，`gpt-5.6` → [5, 6]）。缺段按 0。 */
export function extractPlazaVersionParts(name: string): number[] {
  return (name.match(/\d+/g) ?? []).map((part) => Number(part))
}

/**
 * 模型广场展示序：版本号从新到旧，同版本再按名称升序。
 * 日期后缀比无日期新（缺段当 0）；不改渠道定价列表本身的录入顺序。
 */
export function comparePlazaModelNames(left: string, right: string): number {
  const leftParts = extractPlazaVersionParts(left)
  const rightParts = extractPlazaVersionParts(right)
  const length = Math.max(leftParts.length, rightParts.length)
  for (let index = 0; index < length; index += 1) {
    const leftPart = leftParts[index] ?? 0
    const rightPart = rightParts[index] ?? 0
    if (leftPart !== rightPart) {
      return rightPart - leftPart
    }
  }
  return left.localeCompare(right)
}

export function comparePlazaModels(
  left: { name: string; platform?: string },
  right: { name: string; platform?: string },
): number {
  const byName = comparePlazaModelNames(left.name, right.name)
  if (byName !== 0) return byName
  return (left.platform ?? '').localeCompare(right.platform ?? '')
}

/** 上下文档位按下限升序（后端已升序，这里兜底）。 */
export function sortPlazaIntervals<T extends { min_tokens: number }>(intervals: T[]): T[] {
  return [...intervals].sort((left, right) => left.min_tokens - right.min_tokens)
}

function formatPlazaTokenCount(count: number): string {
  const trim = (value: number) => String(Math.round(value * 100) / 100)
  if (count >= 1_000_000) return `${trim(count / 1_000_000)}M`
  if (count >= 1_000) return `${trim(count / 1_000)}K`
  return String(count)
}

/**
 * 档位标签：优先用后端/管理员给的 tier_label；否则有上限为「≤上限」，末档为「>下限」。
 * 与独立页价格表的写法一致，相邻的 ≤100K / ≤200K 即表示 (100K, 200K]。
 */
export function plazaTierLabel(interval: {
  tier_label?: string
  min_tokens: number
  max_tokens: number | null
}): string {
  if (interval.tier_label) return interval.tier_label
  return interval.max_tokens == null
    ? `>${formatPlazaTokenCount(interval.min_tokens)}`
    : `≤${formatPlazaTokenCount(interval.max_tokens)}`
}

/** 「00:30–08:30」；整分钟的 HH:mm:ss 省略秒。 */
export function formatPlazaTimeWindow(period: { start_time: string; end_time: string }): string {
  const clock = (value: string) => value.replace(/^(\d{2}:\d{2}):00$/, '$1')
  return `${clock(period.start_time)}–${clock(period.end_time)}`
}
