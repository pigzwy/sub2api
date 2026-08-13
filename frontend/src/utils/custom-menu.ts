/**
 * Helpers for how a custom menu item opens.
 *
 * A menu item either renders inside the app (a markdown page, or an iframe
 * embed) or hands the target URL to the browser (same tab / new tab). Both the
 * sidebar and the custom page view need the same answer, so the decision lives
 * here rather than being re-derived at each call site.
 */

import type { CustomMenuItem, CustomMenuOpenMode } from '@/types'
import { buildEmbeddedUrl } from './embedded-url'

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
 * The configured target of a `self`/`blank` item, or '' when the item should
 * not leave the app (markdown, iframe mode, or a non-http target).
 *
 * This is the raw URL. Call sites that actually navigate should use
 * {@link buildExternalHref}, which decorates it the same way an iframe src is
 * decorated.
 */
export function externalHref(item: Pick<CustomMenuItem, 'url' | 'page_slug' | 'open_mode'>): string {
  const mode = resolveOpenMode(item)
  if (mode === 'iframe') return ''
  const url = item.url?.trim() ?? ''
  return url.startsWith('http://') || url.startsWith('https://') ? url : ''
}

/** Caller context needed to decorate an external target, mirroring the iframe src. */
export interface ExternalHrefContext {
  userId?: number
  token?: string | null
  theme?: 'light' | 'dark'
  lang?: string
}

/**
 * The URL a `self`/`blank` item actually navigates to: the configured target
 * carrying the same `user_id` / `token` / `theme` / `lang` / `ui_mode` /
 * `src_*` query parameters that an embedded iframe receives, so a target that
 * works embedded keeps working in a tab.
 *
 * Note this puts the auth token in the address bar and browser history, unlike
 * an iframe src. That is the accepted trade for handing the target the same
 * session it already gets when embedded — the recipient origin is unchanged,
 * only its visibility to the user's own browser is.
 */
export function buildExternalHref(
  item: Pick<CustomMenuItem, 'url' | 'page_slug' | 'open_mode'>,
  ctx: ExternalHrefContext = {},
): string {
  const raw = externalHref(item)
  if (!raw) return ''
  return buildEmbeddedUrl(raw, ctx.userId, ctx.token, ctx.theme ?? 'light', ctx.lang)
}
