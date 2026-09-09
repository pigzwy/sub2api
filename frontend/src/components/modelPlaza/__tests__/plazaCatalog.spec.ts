import { describe, expect, it } from 'vitest'
import { formatZhe, plazaTabLabel } from '../plazaCatalog'

describe('plazaCatalog', () => {
  it('formats group rates as 折', () => {
    expect(formatZhe(0.8)).toBe('8')
    expect(formatZhe(0.11)).toBe('1.1')
    expect(formatZhe(1)).toBe('10')
  })

  it('maps known platforms to catalog labels', () => {
    const t = (key: string) => ({
      'modelPlaza.catalog.platforms.anthropic': 'Claude',
      'modelPlaza.catalog.platforms.openai': 'ChatGPT',
    }[key] ?? key)
    expect(plazaTabLabel('anthropic', t)).toBe('Claude')
    expect(plazaTabLabel('openai', t)).toBe('ChatGPT')
    expect(plazaTabLabel('unknown', t)).toBe('unknown')
  })
})
