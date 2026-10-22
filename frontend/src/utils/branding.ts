import { sanitizeUrl } from '@/utils/url'

function faviconType(url: string): string | undefined {
  const value = url.trim().toLowerCase()
  if (value.startsWith('data:')) {
    const match = /^data:(image\/[a-z0-9.+-]+)/.exec(value)
    return match?.[1]
  }
  if (value.includes('.svg')) return 'image/svg+xml'
  if (value.includes('.png')) return 'image/png'
  if (/\.jpe?g(?:$|[?#])/.test(value)) return 'image/jpeg'
  if (value.includes('.webp')) return 'image/webp'
  if (value.includes('.gif')) return 'image/gif'
  if (value.includes('.ico')) return 'image/x-icon'
  return undefined
}

export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitizedLogoUrl) {
    return
  }

  document.querySelectorAll('link[rel="icon"], link[rel="shortcut icon"]').forEach((node) => node.remove())
  const link = document.createElement('link')
  link.rel = 'icon'
  const type = faviconType(sanitizedLogoUrl)
  if (type) link.type = type
  link.href = sanitizedLogoUrl
  document.head.appendChild(link)
}
