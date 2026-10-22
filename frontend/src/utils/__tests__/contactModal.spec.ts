import { describe, expect, it } from 'vitest'
import {
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
    expect(raw.startsWith('{')).toBe(true)
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
