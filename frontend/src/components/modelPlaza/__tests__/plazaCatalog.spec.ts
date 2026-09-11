import { describe, expect, it } from 'vitest'
import {
  comparePlazaModelNames,
  formatCatalogZhe,
  formatZhe,
  groupYuan,
  officialYuan,
  plazaTabLabel,
  savingsPercent,
} from '../plazaCatalog'

describe('plazaCatalog', () => {
  it('formats group rates as 折', () => {
    expect(formatZhe(0.8)).toBe('8')
    expect(formatZhe(0.11)).toBe('1.1')
    expect(formatZhe(1)).toBe('10')
  })

  it('formats catalog 折 against official CNY', () => {
    expect(formatCatalogZhe(0.8)).toBe('1.1')
    expect(formatCatalogZhe(2)).toBe('2.9')
    expect(formatCatalogZhe(5)).toBe('7.1')
  })

  it('converts token prices to the catalog yuan pair', () => {
    expect(groupYuan(5e-6, 0.8)).toBe(4)
    expect(officialYuan(5e-6)).toBe(35)
    expect(savingsPercent(4, 35)).toBe(89)
  })

  it('maps category tabs to the reference brand labels', () => {
    const t = (key: string) => ({
      'modelPlaza.catalog.platforms.anthropic': 'Claude',
      'modelPlaza.catalog.platforms.openai': 'ChatGPT',
    }[key] ?? key)
    expect(plazaTabLabel('anthropic', t)).toBe('Claude')
    expect(plazaTabLabel('openai', t)).toBe('ChatGPT')
    expect(plazaTabLabel('unknown', t)).toBe('unknown')
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

  it('treats a trailing date as newer than the same id without one', () => {
    expect(comparePlazaModelNames('gpt-5.6-20251001', 'gpt-5.6')).toBeLessThan(0)
    expect(comparePlazaModelNames('gpt-5.6', 'gpt-5.5')).toBeLessThan(0)
  })

  it('falls back to name order when ids have no version numbers', () => {
    expect(comparePlazaModelNames('claude-opus', 'claude-sonnet')).toBeLessThan(0)
  })
})
