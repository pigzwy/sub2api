import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ContactDialog from '../ContactDialog.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function mountContact(image: string, show = true) {
  return mount(ContactDialog, {
    props: {
      show,
      source: JSON.stringify({
        version: 1,
        enabled: true,
        tabs: [{ id: 'service', cards: [{ title: 'Support', image, caption: 'Scan' }] }],
      }),
    },
    global: {
      stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' } },
    },
  })
}

describe('contact images', () => {
  it.each([
    '/api/v1/settings/public/contact-image/0123456789abcdef',
    'data:image/png;base64,AAAA',
    'https://cdn.example.com/qr.png',
  ])('renders compact and legacy image URLs: %s', (image) => {
    const wrapper = mountContact(image)
    expect(wrapper.get('img').attributes('src')).toBe(image)
    wrapper.unmount()
  })

  it.each(['javascript:alert(1)', '//untrusted.example/qr.png', 'data:text/html,test'])('rejects unsafe URL: %s', (image) => {
    const wrapper = mountContact(image)
    expect(wrapper.find('img').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not render an image until the dialog opens', async () => {
    const wrapper = mountContact('/api/v1/settings/public/contact-image/0123456789abcdef', false)
    expect(wrapper.find('img').exists()).toBe(false)
    await wrapper.setProps({ show: true })
    expect(wrapper.find('img').exists()).toBe(true)
    wrapper.unmount()
  })
})
