import { describe, it, expect } from 'vitest'
import { buildExternalHref, externalHref, isMarkdownMenuItem, resolveOpenMode } from '../custom-menu'
import type { CustomMenuItem } from '@/types'

function item(overrides: Partial<CustomMenuItem> = {}): CustomMenuItem {
  return {
    id: 'demo',
    label: 'Demo',
    icon_svg: '',
    url: 'https://example.test/page',
    visibility: 'user',
    sort_order: 0,
    ...overrides,
  }
}

describe('resolveOpenMode', () => {
  // Items saved before this option existed carry no open_mode and must keep
  // rendering in the iframe.
  it('defaults to iframe when open_mode is absent', () => {
    expect(resolveOpenMode(item())).toBe('iframe')
  })

  it('returns the configured mode', () => {
    expect(resolveOpenMode(item({ open_mode: 'iframe' }))).toBe('iframe')
    expect(resolveOpenMode(item({ open_mode: 'self' }))).toBe('self')
    expect(resolveOpenMode(item({ open_mode: 'blank' }))).toBe('blank')
  })

  it('falls back to iframe for an unrecognized value', () => {
    expect(resolveOpenMode(item({ open_mode: 'popup' as never }))).toBe('iframe')
  })

  it('forces iframe for markdown items regardless of open_mode', () => {
    expect(resolveOpenMode(item({ url: 'md:guide', open_mode: 'blank' }))).toBe('iframe')
    expect(resolveOpenMode(item({ url: '', page_slug: 'guide', open_mode: 'self' }))).toBe('iframe')
  })
})

describe('isMarkdownMenuItem', () => {
  it('detects both markdown forms', () => {
    expect(isMarkdownMenuItem(item({ url: 'md:guide' }))).toBe(true)
    expect(isMarkdownMenuItem(item({ url: '', page_slug: 'guide' }))).toBe(true)
    expect(isMarkdownMenuItem(item())).toBe(false)
  })
})

describe('externalHref', () => {
  it('is empty for iframe mode so the item stays on the in-app route', () => {
    expect(externalHref(item({ open_mode: 'iframe' }))).toBe('')
    expect(externalHref(item())).toBe('')
  })

  it('returns the configured URL for self and blank', () => {
    expect(externalHref(item({ open_mode: 'self' }))).toBe('https://example.test/page')
    expect(externalHref(item({ open_mode: 'blank' }))).toBe('https://example.test/page')
  })

  it('rejects non-http targets so they fall back to the in-app route', () => {
    expect(externalHref(item({ url: 'javascript:alert(1)', open_mode: 'blank' }))).toBe('')
    expect(externalHref(item({ url: '/internal/path', open_mode: 'self' }))).toBe('')
    expect(externalHref(item({ url: '', open_mode: 'blank' }))).toBe('')
  })

  it('is empty for markdown items even when an external mode is set', () => {
    expect(externalHref(item({ url: 'md:guide', open_mode: 'blank' }))).toBe('')
  })
})

describe('buildExternalHref', () => {
  const ctx = { userId: 7, token: 'jwt-abc', theme: 'dark' as const, lang: 'zh' }

  // The whole point of the external modes carrying parameters: a target that
  // works embedded must keep working when opened in a tab.
  it('passes the same parameters an iframe src would carry', () => {
    const href = buildExternalHref(item({ open_mode: 'blank' }), ctx)
    const url = new URL(href)
    expect(url.origin + url.pathname).toBe('https://example.test/page')
    expect(url.searchParams.get('user_id')).toBe('7')
    expect(url.searchParams.get('token')).toBe('jwt-abc')
    expect(url.searchParams.get('theme')).toBe('dark')
    expect(url.searchParams.get('lang')).toBe('zh')
    expect(url.searchParams.get('ui_mode')).toBe('embedded')
  })

  it('preserves query parameters already present on the configured URL', () => {
    const href = buildExternalHref(
      item({ url: 'https://example.test/page?tenant=acme', open_mode: 'self' }),
      ctx,
    )
    const url = new URL(href)
    expect(url.searchParams.get('tenant')).toBe('acme')
    expect(url.searchParams.get('token')).toBe('jwt-abc')
  })

  it('omits credentials that are not available', () => {
    const href = buildExternalHref(item({ open_mode: 'blank' }), {})
    const url = new URL(href)
    expect(url.searchParams.has('user_id')).toBe(false)
    expect(url.searchParams.has('token')).toBe(false)
    expect(url.searchParams.get('theme')).toBe('light')
  })

  it('stays empty wherever externalHref is empty', () => {
    expect(buildExternalHref(item({ open_mode: 'iframe' }), ctx)).toBe('')
    expect(buildExternalHref(item({ url: 'md:guide', open_mode: 'blank' }), ctx)).toBe('')
    expect(buildExternalHref(item({ url: 'javascript:alert(1)', open_mode: 'blank' }), ctx)).toBe('')
  })
})
