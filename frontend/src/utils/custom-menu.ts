/**
 * Helpers for how a custom menu item opens.
 *
 * A menu item either renders inside the app (a markdown page, or an iframe
 * embed) or hands the target URL to the browser (same tab / new tab). Both the
 * sidebar and the custom page view need the same answer, so the decision lives
 * here rather than being re-derived at each call site.
 */

import type { CustomMenuItem, CustomMenuOpenMode } from '@/types'

const OPEN_MODES: readonly CustomMenuOpenMode[] = ['iframe', 'self', 'blank']

/** Markdown-backed items render in-app regardless of the configured mode. */
export function isMarkdownMenuItem(item: Pick<CustomMenuItem, 'url' | 'page_slug'>): boolean {
  return !!item.page_slug || !!item.url?.startsWith('md:')
}

/**
 * Resolves the effective open mode. Items saved before this option existed have
 * no `open_mode`, so an absent or unrecognized value means `iframe` — the
 * historical behavior. Markdown items are forced to `iframe` (i.e. in-app)
 * because there is no external URL to hand to the browser.
 */
export function resolveOpenMode(item: Pick<CustomMenuItem, 'url' | 'page_slug' | 'open_mode'>): CustomMenuOpenMode {
  if (isMarkdownMenuItem(item)) return 'iframe'
  const mode = item.open_mode
  return mode && OPEN_MODES.includes(mode) ? mode : 'iframe'
}

/**
 * The URL a `self`/`blank` item navigates to, or '' when the item should not
 * leave the app (markdown, iframe mode, or a non-http target).
 *
 * This is the URL exactly as configured — deliberately NOT `buildEmbeddedUrl`,
 * which appends the caller's auth token, user id and `ui_mode=embedded`. Those
 * belong in an iframe src, which never reaches the address bar; a same-tab or
 * new-tab navigation would put the token in the URL bar, browser history and
 * any onward referrer. An admin who needs the target to receive parameters can
 * put them in the configured URL.
 */
export function externalHref(item: Pick<CustomMenuItem, 'url' | 'page_slug' | 'open_mode'>): string {
  const mode = resolveOpenMode(item)
  if (mode === 'iframe') return ''
  const url = item.url?.trim() ?? ''
  return url.startsWith('http://') || url.startsWith('https://') ? url : ''
}
