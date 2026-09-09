/** 分组倍率换算成常见「X 折」展示（0.8x → 8）。 */
export function formatZhe(rate: number): string {
  const z = Math.round(rate * 100) / 10
  return Number.isInteger(z) ? String(z) : z.toFixed(1)
}

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
