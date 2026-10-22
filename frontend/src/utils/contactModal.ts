export const CONTACT_TAB_IDS = ['service', 'support', 'business'] as const

export type ContactTabId = (typeof CONTACT_TAB_IDS)[number]

export interface ContactCard {
  title: string
  image: string
  caption: string
}

export interface ContactTab {
  id: ContactTabId
  cards: ContactCard[]
  email: string
}

export interface ContactModal {
  tabs: ContactTab[]
}

function emptyCard(): ContactCard {
  return { title: '', image: '', caption: '' }
}

export function emptyContactModal(): ContactModal {
  return {
    tabs: CONTACT_TAB_IDS.map((id) => ({
      id,
      cards: [emptyCard(), emptyCard()],
      email: '',
    })),
  }
}

function cardText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

export function isContactCardEmpty(card: ContactCard): boolean {
  return !card.title.trim() && !card.image.trim() && !card.caption.trim()
}

export function visibleContactCards(tab: ContactTab): ContactCard[] {
  return tab.cards.filter((card) => !isContactCardEmpty(card))
}

export function contactTabHasContent(tab: ContactTab): boolean {
  return Boolean(tab.email.trim()) || visibleContactCards(tab).length > 0
}

function applyParsedTabs(modal: ContactModal, tabs: unknown): ContactModal {
  if (!Array.isArray(tabs)) return modal
  for (const tab of tabs) {
    if (!tab || typeof tab !== 'object') continue
    const row = tab as { id?: unknown; email?: unknown; cards?: unknown }
    const target = modal.tabs.find((item) => item.id === row.id)
    if (!target) continue
    target.email = cardText(row.email)
    if (!Array.isArray(row.cards)) continue
    const cards = row.cards.slice(0, 2).map((card) => {
      if (!card || typeof card !== 'object') return emptyCard()
      const item = card as { title?: unknown; image?: unknown; caption?: unknown }
      return {
        title: cardText(item.title),
        image: cardText(item.image),
        caption: cardText(item.caption),
      }
    })
    target.cards = [cards[0] ?? emptyCard(), cards[1] ?? emptyCard()]
  }
  return modal
}

export function parseContactModal(raw: string | null | undefined): ContactModal {
  const text = (raw ?? '').trim()
  const modal = emptyContactModal()
  if (!text) return modal
  if (text.startsWith('{')) {
    try {
      const data = JSON.parse(text) as { version?: unknown; tabs?: unknown }
      if (data?.version === 1) return applyParsedTabs(modal, data.tabs)
    } catch {
      // A plain note that happens to start with "{" stays readable as text.
    }
  }
  modal.tabs[0].email = text
  return modal
}

export function serializeContactModal(modal: ContactModal): string {
  const tabs = CONTACT_TAB_IDS.map((id) => {
    const tab = modal.tabs.find((item) => item.id === id)
    return {
      id,
      email: tab?.email.trim() ?? '',
      cards: (tab?.cards ?? [])
        .slice(0, 2)
        .map((card) => ({
          title: card.title.trim(),
          image: card.image.trim(),
          caption: card.caption.trim(),
        }))
        .filter((card) => card.title || card.image || card.caption),
    }
  })
  const serviceEmail = tabs[0]?.email ?? ''
  const hasCards = tabs.some((tab) => tab.cards.length > 0)
  const hasMoreEmail = tabs.slice(1).some((tab) => tab.email)
  if (!hasCards && !hasMoreEmail) return serviceEmail
  return JSON.stringify({ version: 1, tabs })
}

export function hasContactContent(raw: string | null | undefined): boolean {
  return parseContactModal(raw).tabs.some(contactTabHasContent)
}

/** Plain note, or the first email in a structured popup. Empty when the popup only has QR codes. */
export function contactSummary(raw: string | null | undefined): string {
  const text = (raw ?? '').trim()
  if (!text.startsWith('{')) return text
  return parseContactModal(text).tabs.map((tab) => tab.email.trim()).find(Boolean) ?? ''
}
