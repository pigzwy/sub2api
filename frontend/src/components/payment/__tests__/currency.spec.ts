import { describe, expect, it } from 'vitest'
import { currencySymbol, formatPaymentAmount, formatPaymentNumber, normalizePaymentCurrency } from '../currency'

describe('formatPaymentAmount', () => {
  it('uses the currency default fraction digits', () => {
    expect(formatPaymentAmount(100, 'JPY', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'KRW', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'HKD', 'en-US')).toContain('.00')
  })
})

describe('formatPaymentNumber', () => {
  it('keeps currency precision for package prices', () => {
    expect(formatPaymentNumber(10.49, 'CNY', 'en-US')).toBe('10.49')
    expect(formatPaymentNumber(10.5, 'CNY', 'en-US')).toBe('10.50')
    expect(formatPaymentNumber(10.49, 'JPY', 'en-US')).toBe('10')
  })
})

describe('normalizePaymentCurrency', () => {
  it('keeps USDT as a display currency for crypto lanes', () => {
    expect(normalizePaymentCurrency('usdt')).toBe('USDT')
    expect(formatPaymentAmount(50, 'USDT', 'en-US')).toContain('50')
  })
})

describe('currencySymbol', () => {
  it('maps common payment currencies and falls back safely', () => {
    expect(currencySymbol('USD')).toBe('$')
    expect(currencySymbol('cny')).toBe('¥')
    expect(currencySymbol('EUR')).toBe('€')
    expect(currencySymbol('')).toBe('¥')
    expect(currencySymbol('XYZ')).toBe('XYZ')
  })
})
