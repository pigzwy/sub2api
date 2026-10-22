import { beforeEach, describe, expect, it } from 'vitest'
import { updateFavicon } from '@/utils/branding'

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = '<link rel="icon" href="/logo.svg">'
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe('https://example.com/custom-logo.png')
  })

  it('keeps a png logo as png instead of the default svg icon type', () => {
    updateFavicon('data:image/png;base64,abc')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.type).toBe('image/png')
    expect(link?.getAttribute('href')).toBe('data:image/png;base64,abc')
  })

  it('does not mark a hosted logo url as an ico', () => {
    updateFavicon('/api/v1/settings/public/logo/abc123')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('type')).toBeNull()
    expect(link?.getAttribute('href')).toBe('/api/v1/settings/public/logo/abc123')
  })

  it('ignores unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe('/logo.svg')
  })
})
