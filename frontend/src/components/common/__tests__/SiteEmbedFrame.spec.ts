import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import SiteEmbedFrame from '../SiteEmbedFrame.vue'

const ORIGIN = 'https://www.pigcode.ai'
const BASE = `${ORIGIN}/status.html`

function mountFrame(locale = 'zh') {
  const i18n = createI18n({ legacy: false, locale, messages: { zh: {}, en: {} } })
  const wrapper = mount(SiteEmbedFrame, {
    global: { plugins: [i18n] },
    props: {
      baseUrl: BASE,
      heightMessageType: 'pigcode-status-height',
      title: 'Group Status',
      fallbackHint: 'fallback',
      loadTimeoutText: 'timeout',
      retryLabel: 'retry',
      openExternalLabel: 'open',
    },
    attachTo: document.body,
  })
  return { wrapper, i18n }
}

function postFromEmbed(data: unknown, origin = ORIGIN) {
  window.dispatchEvent(new MessageEvent('message', { data, origin }))
}

describe('SiteEmbedFrame', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    document.documentElement.classList.remove('dark')
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('opens the page in embed mode with theme and language, and never a user id or token', () => {
    document.documentElement.classList.add('dark')
    const { wrapper } = mountFrame('en')
    const src = new URL(wrapper.find('iframe').attributes('src')!)
    expect(src.origin + src.pathname).toBe(BASE)
    expect(Object.fromEntries(src.searchParams)).toEqual({ embed: '1', theme: 'dark', lang: 'en-US' })
    wrapper.unmount()
  })

  it('resizes to the height the embedded page reports', async () => {
    const { wrapper } = mountFrame()
    postFromEmbed({ type: 'pigcode-status-height', height: 1834.2 })
    await nextTick()
    expect(wrapper.find('iframe').attributes('style')).toContain('height: 1835px')
    expect(wrapper.find('iframe').attributes('scrolling')).toBe('no')
    wrapper.unmount()
  })

  it('ignores messages from other origins and of other types', async () => {
    const { wrapper } = mountFrame()
    postFromEmbed({ type: 'pigcode-status-height', height: 3000 }, 'https://evil.example')
    postFromEmbed({ type: 'pigcode-pricing-height', height: 3000 })
    postFromEmbed({ type: 'pigcode-status-height', height: -5 })
    await nextTick()
    expect(wrapper.find('iframe').attributes('style')).not.toContain('3000px')
    expect(wrapper.find('iframe').attributes('scrolling')).toBe('auto')
    wrapper.unmount()
  })

  it('clamps absurd heights', async () => {
    const { wrapper } = mountFrame()
    postFromEmbed({ type: 'pigcode-status-height', height: 9_999_999 })
    await nextTick()
    expect(wrapper.find('iframe').attributes('style')).toContain('height: 20000px')
    wrapper.unmount()
  })

  it('offers retry and open-in-new-window when the page never answers', async () => {
    const { wrapper } = mountFrame()
    // 页面加载了，但 8 秒内没有上报高度：保留 iframe，同时给出提示条
    await wrapper.find('iframe').trigger('load')
    vi.advanceTimersByTime(8000)
    await nextTick()
    expect(wrapper.text()).toContain('fallback')
    const link = wrapper.find('a')
    expect(link.attributes('href')).toBe(BASE)
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toContain('noopener')
    wrapper.unmount()
  })

  it('shows the timeout panel when the page did not load at all', async () => {
    const { wrapper } = mountFrame()
    vi.advanceTimersByTime(8000)
    await nextTick()
    expect(wrapper.text()).toContain('timeout')
    expect(wrapper.find('iframe').exists()).toBe(false)
    await wrapper.find('button').trigger('click')
    await nextTick()
    expect(wrapper.find('iframe').exists()).toBe(true)
    wrapper.unmount()
  })

  it('a late height report clears the fallback', async () => {
    const { wrapper } = mountFrame()
    await wrapper.find('iframe').trigger('load')
    vi.advanceTimersByTime(8000)
    await nextTick()
    expect(wrapper.text()).toContain('fallback')
    postFromEmbed({ type: 'pigcode-status-height', height: 900 })
    await nextTick()
    expect(wrapper.text()).not.toContain('fallback')
    wrapper.unmount()
  })
})
