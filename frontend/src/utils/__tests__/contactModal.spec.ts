import { describe, expect, it } from 'vitest'
import { sanitizeUrl } from '../url'
import {
  contactMenuEnabled,
  contactSummary,
  hasContactContent,
  parseContactModal,
  serializeContactModal,
  visibleContactCards,
} from '../contactModal'

describe('contactModal', () => {
  it('keeps a plain note as the service email', () => {
    const modal = parseContactModal('QQ: 123456')
    expect(modal.tabs[0].email).toBe('QQ: 123456')
    expect(serializeContactModal(modal)).toBe('QQ: 123456')
    expect(contactSummary('QQ: 123456')).toBe('QQ: 123456')
    expect(hasContactContent('QQ: 123456')).toBe(true)
    expect(contactMenuEnabled('QQ: 123456')).toBe(false)
  })

  it('round-trips qr cards without leaking the raw json into the summary', () => {
    const raw = serializeContactModal({
      tabs: [
        {
          id: 'service',
          email: 'admin@example.com',
          cards: [
            { title: '微信客服', image: 'https://cdn.example.com/a.png', caption: '扫码添加客服' },
            { title: '', image: '', caption: '' },
          ],
        },
        {
          id: 'support',
          email: '',
          cards: [
            { title: '售后群', image: 'data:image/png;base64,aaaa', caption: '扫码' },
            { title: '', image: '', caption: '' },
          ],
        },
        { id: 'business', email: '', cards: [] },
      ],
    })

    const modal = parseContactModal(raw)
    expect(modal.tabs[0].email).toBe('admin@example.com')
    expect(visibleContactCards(modal.tabs[0])).toEqual([
      { title: '微信客服', image: 'https://cdn.example.com/a.png', caption: '扫码添加客服' },
    ])
    expect(visibleContactCards(modal.tabs[1])).toHaveLength(1)
    expect(contactSummary(raw)).toBe('admin@example.com')
    expect(hasContactContent(raw)).toBe(true)
    expect(contactMenuEnabled(raw)).toBe(false)
    expect(raw.startsWith('{')).toBe(true)
  })

  it('keeps an uploaded image for the contact dialog', () => {
    const image = 'data:image/png;base64,iVBORw0KGgo='
    const raw = serializeContactModal({
      tabs: [
        { id: 'service', email: '', cards: [{ title: '微信客服', image, caption: '扫码添加客服' }] },
        { id: 'support', email: '', cards: [] },
        { id: 'business', email: '', cards: [] },
      ],
    })
    expect(parseContactModal(raw).tabs[0].cards[0].image).toBe(image)
    expect(sanitizeUrl(image, { allowDataUrl: true })).toBe(image)
    expect(hasContactContent(raw)).toBe(true)
    expect(contactMenuEnabled(raw)).toBe(false)
  })

  it('shows the sidebar item only after the switch is turned on', () => {
    const hidden = serializeContactModal({
      enabled: false,
      tabs: [
        { id: 'service', email: 'qq: 1', cards: [] },
        { id: 'support', email: '', cards: [] },
        { id: 'business', email: '', cards: [] },
      ],
    })
    expect(hidden).toBe('qq: 1')
    expect(contactMenuEnabled(hidden)).toBe(false)

    const shown = serializeContactModal({
      enabled: true,
      tabs: [
        { id: 'service', email: 'qq: 1', cards: [] },
        { id: 'support', email: '', cards: [] },
        { id: 'business', email: '', cards: [] },
      ],
    })
    expect(JSON.parse(shown).enabled).toBe(true)
    expect(contactMenuEnabled(shown)).toBe(true)
    expect(parseContactModal(shown).tabs[0].email).toBe('qq: 1')
  })

  it('treats an empty value as no contact menu', () => {
    expect(hasContactContent('')).toBe(false)
    expect(hasContactContent('   ')).toBe(false)
    expect(serializeContactModal(parseContactModal(''))).toBe('')
  })

  it('keeps an unreadable brace-prefixed note as text', () => {
    expect(parseContactModal('{not json').tabs[0].email).toBe('{not json')
    expect(contactSummary('{not json')).toBe('{not json')
  })
})
