import { describe, expect, it } from 'vitest'
import {
  comparePlazaModelNames,
  formatCatalogZhe,
  formatPlazaTimeWindow,
  formatYuan,
  formatZhe,
  groupYuan,
  groupYuanExact,
  isCheaperPrice,
  officialYuan,
  officialYuanExact,
  plazaTabLabel,
  plazaTierLabel,
  savingsPercent,
  sortPlazaIntervals,
  sortPlazaPlatforms,
} from '../plazaCatalog'

describe('plazaCatalog', () => {
  it('keeps sub-cent prices visible instead of rounding them to zero', () => {
    expect(groupYuan(4e-8, 0.1)).toBe(0.004)
    expect(formatYuan(groupYuan(4e-8, 0.1))).toBe('¥0.004')
    expect(formatYuan(officialYuan(2.8e-9))).toBe('¥0.003')
    expect(formatYuan(0.0004)).toBe('¥0.0004')
    // 1 分及以上、以及真正的 0，仍是两位小数。
    expect(formatYuan(groupYuan(5e-6, 0.8))).toBe('¥4.00')
    expect(formatYuan(0.01)).toBe('¥0.01')
    expect(formatYuan(0)).toBe('¥0.00')
    expect(formatYuan(null)).toBe('-')
  })

  it('labels context tiers by their upper bound and the last one by its lower bound', () => {
    expect(plazaTierLabel({ min_tokens: 0, max_tokens: 272000 })).toBe('≤272K')
    expect(plazaTierLabel({ min_tokens: 272000, max_tokens: null })).toBe('>272K')
    expect(plazaTierLabel({ min_tokens: 0, max_tokens: 1_000_000 })).toBe('≤1M')
    expect(plazaTierLabel({ min_tokens: 0, max_tokens: 500 })).toBe('≤500')
    expect(plazaTierLabel({ tier_label: '2K', min_tokens: 0, max_tokens: null })).toBe('2K')
  })

  it('sorts tiers by lower bound without mutating the input', () => {
    const tiers = [{ min_tokens: 272000 }, { min_tokens: 0 }]
    expect(sortPlazaIntervals(tiers).map((tier) => tier.min_tokens)).toEqual([0, 272000])
    expect(tiers[0].min_tokens).toBe(272000)
  })

  it('formats time windows and drops zero seconds', () => {
    expect(formatPlazaTimeWindow({ start_time: '00:30:00', end_time: '08:30:00' })).toBe('00:30–08:30')
    expect(formatPlazaTimeWindow({ start_time: '00:30:15', end_time: '08:30' })).toBe('00:30:15–08:30')
  })

  it('formats group rates as 折', () => {
    expect(formatZhe(0.8)).toBe('8')
    expect(formatZhe(0.11)).toBe('1.1')
    expect(formatZhe(1)).toBe('10')
  })

  it('formats catalog 折 against official CNY', () => {
    expect(formatCatalogZhe(0.8)).toBe('8')
    expect(formatCatalogZhe(1)).toBe('10')
    expect(formatCatalogZhe(2)).toBe('20')
  })

  it('converts token prices to the catalog yuan pair', () => {
    expect(groupYuan(5e-6, 0.8)).toBe(4)
    expect(officialYuan(5e-6)).toBe(5)
    expect(savingsPercent(4, 5)).toBe(20)
  })

  it('compares sub-cent prices before rounding them for display', () => {
    // 官方 ¥0.0015、分组 8 折 ¥0.0012：展示各取 1 位有效数字，折扣仍按真实价格算。
    const group = groupYuanExact(1.5e-9, 0.8)
    const official = officialYuanExact(1.5e-9)
    expect(savingsPercent(group, official)).toBe(20)
    expect(isCheaperPrice(group, official)).toBe(true)

    // 官方 ¥0.0014、分组 ¥0.00112：取整后都显示 ¥0.001，但分组价确实更低。
    expect(formatYuan(groupYuan(1.4e-9, 0.8))).toBe(formatYuan(officialYuan(1.4e-9)))
    expect(isCheaperPrice(groupYuanExact(1.4e-9, 0.8), officialYuanExact(1.4e-9))).toBe(true)

    // 倍率为 1 时不算折扣，也不显示「省 0%」。
    expect(isCheaperPrice(groupYuanExact(3e-6, 1), officialYuanExact(3e-6))).toBe(false)
    expect(savingsPercent(4.999, 5)).toBeNull()
  })

  it('maps category tabs to the reference brand labels', () => {
    const t = (key: string) => ({
      'modelPlaza.catalog.platforms.anthropic': 'Claude',
      'modelPlaza.catalog.platforms.openai': 'ChatGPT',
      'modelPlaza.catalog.platforms.zhipu': 'Zhipu',
      'modelPlaza.catalog.platforms.composite': 'Image/Video',
      'modelPlaza.catalog.platforms.minimax': 'Minimax',
    }[key] ?? key)
    expect(plazaTabLabel('anthropic', t)).toBe('Claude')
    expect(plazaTabLabel('openai', t)).toBe('ChatGPT')
    expect(plazaTabLabel('zhipu', t)).toBe('Zhipu')
    expect(plazaTabLabel('minimax', t)).toBe('Minimax')
    expect(plazaTabLabel('composite', t)).toBe('Image/Video')
    expect(plazaTabLabel('unknown', t)).toBe('unknown')
  })

  it('keeps Image/Video last among category tabs', () => {
    expect(sortPlazaPlatforms(['composite', 'minimax', 'anthropic', 'openai'])).toEqual([
      'anthropic',
      'openai',
      'minimax',
      'composite',
    ])
  })

  it('sorts plaza models newest-version first', () => {
    const names = [
      'claude-haiku-4-5',
      'claude-opus-4-6',
      'claude-opus-4-8',
      'claude-fable-5-1',
    ]
    names.sort(comparePlazaModelNames)
    expect(names).toEqual([
      'claude-fable-5-1',
      'claude-opus-4-8',
      'claude-opus-4-6',
      'claude-haiku-4-5',
    ])
  })

  it('sorts OpenAI and Grok ids newest-version first', () => {
    const openai = ['gpt-5', 'gpt-5.5', 'gpt-5.6-sol']
    openai.sort(comparePlazaModelNames)
    expect(openai).toEqual(['gpt-5.6-sol', 'gpt-5.5', 'gpt-5'])

    const grok = ['grok-4.3', 'grok-4.6', 'grok-4.5']
    grok.sort(comparePlazaModelNames)
    expect(grok).toEqual(['grok-4.6', 'grok-4.5', 'grok-4.3'])
  })

  it('treats a trailing date as newer than the same id without one', () => {
    expect(comparePlazaModelNames('gpt-5.6-20251001', 'gpt-5.6')).toBeLessThan(0)
    expect(comparePlazaModelNames('gpt-5.6', 'gpt-5.5')).toBeLessThan(0)
  })

  it('falls back to name order when ids have no version numbers', () => {
    expect(comparePlazaModelNames('claude-opus', 'claude-sonnet')).toBeLessThan(0)
  })
})
