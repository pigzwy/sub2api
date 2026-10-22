import { sanitizeUrl } from '@/utils/url'

export const SITE_BRAND_STORAGE_KEY = 'sub2api_site_brand'

const maxStoredLogoLength = 64 * 1024

export function rememberSiteBrand(name: string, logo: string): void {
  if (typeof localStorage === 'undefined') return
  const brand = {
    name: name.trim(),
    logo: sanitizeUrl(logo, { allowRelative: true, allowDataUrl: true }),
  }
  if (brand.logo.startsWith('data:') && brand.logo.length > maxStoredLogoLength) {
    brand.logo = ''
  }
  if (!brand.name && !brand.logo) {
    localStorage.removeItem(SITE_BRAND_STORAGE_KEY)
    return
  }
  localStorage.setItem(SITE_BRAND_STORAGE_KEY, JSON.stringify(brand))
}

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
