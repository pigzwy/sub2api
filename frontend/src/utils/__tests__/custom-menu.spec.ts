import { describe, it, expect } from 'vitest'
import { externalHref, isMarkdownMenuItem, resolveOpenMode } from '../custom-menu'
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

  // The href must be the raw configured URL: buildEmbeddedUrl appends the auth
  // token, which must not reach the address bar or browser history.
  it('does not append embedded parameters', () => {
    const href = externalHref(item({ open_mode: 'blank' }))
    expect(href).not.toContain('token=')
    expect(href).not.toContain('ui_mode=')
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
