import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaPriceSheet from '../PlazaPriceSheet.vue'
import type { PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'modelPlaza.table.officialLine') return `官方价格 ${params?.amount}`
        if (key === 'modelPlaza.table.savePercent') return `省 ${params?.percent}%`
        if (key === 'modelPlaza.table.unitPerMillionShort') return '/ 1M tokens'
        return key
      },
    }),
  }
})

const model: PlazaModel = {
  name: 'claude-opus-4',
  platform: 'anthropic',
  pricing: {
    billing_mode: 'token',
    input_price: 5e-6,
    output_price: 2.5e-5,
    cache_write_price: 6.25e-6,
    cache_read_price: 5e-7,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
  },
  official_pricing: {
    input_price: 5e-6,
    output_price: 2.5e-5,
    cache_write_price: 6.25e-6,
    cache_read_price: 5e-7,
  },
}

function mountSheet(priceMode: 'group' | 'official' = 'group', models: PlazaModel[] = [model]) {
  return mount(PlazaPriceSheet, {
    props: {
      models,
      rateMultiplier: 0.8,
      priceMode,
    },
    global: { stubs: { Icon: true } },
  })
}

describe('PlazaPriceSheet', () => {
  it('stacks group yuan over official yuan and shows savings', () => {
    const wrapper = mountSheet('group')
    const text = wrapper.text().replace(/\s+/g, ' ')
    expect(text).toContain('claude-opus-4')
    expect(text).toContain('¥4.00')
    expect(text).toContain('官方价格 ¥35.00')
    expect(text).toContain('¥20.00')
    expect(text).toContain('官方价格 ¥175.00')
    expect(text).toContain('省 89%')
  })

  it('shows official amounts as the primary value in official mode', () => {
    const wrapper = mountSheet('official')
    const text = wrapper.text().replace(/\s+/g, ' ')
    expect(text).toContain('¥35.00')
    expect(text).not.toContain('官方价格 ¥35.00')
    expect(text).not.toContain('省 ')
  })

  it.each(['image', 'per_request'] as const)('does not compare %s prices with official token prices', async (billingMode) => {
    const media: PlazaModel = {
      ...model,
      name: 'media-model',
      pricing: { ...model.pricing!, billing_mode: billingMode, per_request_price: 0.2 },
    }
    const wrapper = mountSheet('group', [media])
    expect(wrapper.text()).toContain('¥0.16')
    expect(wrapper.text()).not.toContain('官方价格 ¥')
    expect(wrapper.text()).not.toContain('省 ')
    await wrapper.setProps({ priceMode: 'official' })
    expect(wrapper.text()).not.toContain('¥')
  })

  it('does not substitute output savings when input reference is missing', () => {
    const wrapper = mountSheet('group', [{
      ...model,
      official_pricing: { ...model.official_pricing!, input_price: null },
    }])
    expect(wrapper.text()).not.toContain('省 ')
  })

  it('always discloses the simplified pricing scope', () => {
    const wrapper = mountSheet()
    expect(wrapper.get('[data-testid="plaza-price-note"]').text()).toBe('modelPlaza.catalog.displayPriceNote')
  })
})
