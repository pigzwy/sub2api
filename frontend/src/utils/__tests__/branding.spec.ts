import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { rememberSiteBrand, SITE_BRAND_STORAGE_KEY, updateFavicon } from '@/utils/branding'

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

describe('rememberSiteBrand', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  beforeEach(() => {
    localStorage.clear()
    document.head.innerHTML = '<title></title>'
    document.title = ''
  })

  it('saves the site name and logo for the next refresh', () => {
    rememberSiteBrand('Acme', '/api/v1/settings/public/logo/abc')

    expect(JSON.parse(localStorage.getItem(SITE_BRAND_STORAGE_KEY) || '')).toEqual({
      name: 'Acme',
      logo: '/api/v1/settings/public/logo/abc',
    })
  })

  it('does not interrupt settings when storage is full', () => {
    vi.stubGlobal('localStorage', {
      setItem: () => { throw new DOMException('Storage full', 'QuotaExceededError') },
    })
    expect(() => rememberSiteBrand('Acme', '/logo.svg')).not.toThrow()
  })

  it('does not interrupt settings when clearing storage is blocked', () => {
    vi.stubGlobal('localStorage', {
      removeItem: () => { throw new DOMException('Storage blocked', 'SecurityError') },
    })
    expect(() => rememberSiteBrand('', '')).not.toThrow()
  })

  it('works when browser storage is unavailable', () => {
    vi.stubGlobal('localStorage', undefined)
    expect(() => rememberSiteBrand('Acme', '/logo.svg')).not.toThrow()
  })

  it('applies the saved brand before the default tab icon is parsed', () => {
    localStorage.setItem(SITE_BRAND_STORAGE_KEY, JSON.stringify({
      name: 'Acme',
      logo: '/api/v1/settings/public/logo/abc',
    }))
    const boot = readFileSync(resolve(process.cwd(), 'public/branding-boot.js'), 'utf8')
    // The boot file is a classic script, so evaluate it in the page.
    // eslint-disable-next-line no-eval
    eval(boot)

    expect(document.title).toBe('Acme')
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe('/api/v1/settings/public/logo/abc')
  })
})
