import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaPriceSheet from '../PlazaPriceSheet.vue'
import type { PlazaModel } from '@/api/modelPlaza'

const { copyToClipboard } = vi.hoisted(() => ({ copyToClipboard: vi.fn() }))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

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

function mountSheet(
  priceMode: 'group' | 'official' = 'group',
  models: PlazaModel[] = [model],
  rateMultiplier = 0.8,
) {
  return mount(PlazaPriceSheet, {
    props: {
      models,
      rateMultiplier,
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
    expect(text).toContain('官方价格 ¥5.00')
    expect(text).toContain('¥20.00')
    expect(text).toContain('官方价格 ¥25.00')
    expect(text).toContain('省 20%')
  })

  it('shows official amounts as the primary value in official mode', () => {
    const wrapper = mountSheet('official')
    const text = wrapper.text().replace(/\s+/g, ' ')
    expect(text).toContain('¥5.00')
    expect(text).not.toContain('官方价格 ¥5.00')
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
    expect(wrapper.get('[data-testid="plaza-media-price"]').attributes('colspan')).toBe('4')
    expect(wrapper.text()).not.toContain('官方价格 ¥')
    expect(wrapper.text()).not.toContain('省 ')
    await wrapper.setProps({ priceMode: 'official' })
    expect(wrapper.text()).not.toContain('¥')
  })

  it('shows image resolution tiers instead of input and output columns', () => {
    const image: PlazaModel = {
      ...model,
      name: 'gpt-image-2.5',
      pricing: {
        ...model.pricing!,
        billing_mode: 'image',
        input_price: 0,
        output_price: 0,
        per_request_price: 0,
        image_output_price: 0,
        intervals: [
          { min_tokens: 0, max_tokens: null, tier_label: '1K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.1 },
          { min_tokens: 0, max_tokens: null, tier_label: '2K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.2 },
          { min_tokens: 0, max_tokens: null, tier_label: '4K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.25 },
        ],
      },
    }
    const wrapper = mountSheet('group', [image])
    const text = wrapper.get('[data-testid="plaza-media-price"]').text()
    expect(text).toContain('1K')
    expect(text).toContain('¥0.08')
    expect(text).toContain('2K')
    expect(text).toContain('¥0.16')
    expect(text).toContain('4K')
    expect(text).toContain('¥0.20')
    expect(text).not.toContain('¥0.00')
    expect(wrapper.get('[data-testid="plaza-media-price"]').attributes('colspan')).toBe('4')
    expect(wrapper.text()).toContain('modelPlaza.table.imagePrice')
    expect(wrapper.text()).not.toContain('modelPlaza.table.inputPrice')
  })

  it('shows video resolution prices per second', () => {
    const video: PlazaModel = {
      ...model,
      name: 'grok-imagine-video',
      pricing: {
        ...model.pricing!,
        billing_mode: 'video',
        input_price: null,
        output_price: null,
        per_request_price: null,
        intervals: [
          { min_tokens: 0, max_tokens: null, tier_label: '480p', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.05 },
          { min_tokens: 0, max_tokens: null, tier_label: '720p', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.07 },
        ],
      },
    }
    const text = mountSheet('group', [video]).get('[data-testid="plaza-media-price"]').text()
    expect(text).toContain('480p')
    expect(text).toContain('¥0.04')
    expect(text).toContain('720p')
    expect(text).toContain('modelPlaza.table.perUnitVideo')
    expect(text).not.toContain('modelPlaza.table.perUnitRequest')
  })

  it('does not substitute output savings when input reference is missing', () => {
    const wrapper = mountSheet('group', [{
      ...model,
      official_pricing: { ...model.official_pricing!, input_price: null },
    }])
    expect(wrapper.text()).not.toContain('省 ')
  })

  it('strikes through the official price only when the group price is lower', () => {
    const discounted = mountSheet('group', [model], 0.8)
    const struck = discounted.findAll('p.line-through').map((node) => node.text())
    expect(struck).toContain('官方价格 ¥5.00')
    expect(struck).toHaveLength(4)

    expect(mountSheet('group', [model], 1).find('p.line-through').exists()).toBe(false)
    expect(mountSheet('group', [model], 3).find('p.line-through').exists()).toBe(false)
  })

  it('drops the comparison column when no model is cheaper than official', () => {
    const cheaper = mountSheet('group', [model], 0.8)
    expect(cheaper.findAll('thead th')).toHaveLength(6)
    expect(cheaper.text()).toContain('modelPlaza.catalog.inputSavings')

    for (const wrapper of [mountSheet('group', [model], 1.5), mountSheet('official', [model], 0.8)]) {
      expect(wrapper.findAll('thead th')).toHaveLength(5)
      expect(wrapper.findAll('tbody tr td')).toHaveLength(5)
      expect(wrapper.text()).not.toContain('modelPlaza.catalog.inputSavings')
    }
  })

  it('keeps the comparison column for the whole table when only some models are cheaper', () => {
    const wrapper = mountSheet('group', [
      model,
      { ...model, name: 'claude-opus-3', official_pricing: { ...model.official_pricing!, input_price: 1e-6 } },
    ])
    expect(wrapper.findAll('thead th')).toHaveLength(6)
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    for (const row of rows) expect(row.findAll('td')).toHaveLength(6)
  })

  it('copies the model id through the shared clipboard helper', async () => {
    copyToClipboard.mockClear()
    const wrapper = mountSheet()
    await wrapper.get('[data-testid="plaza-copy-model"]').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('claude-opus-4')
  })

  it('always discloses the simplified pricing scope', () => {
    const wrapper = mountSheet()
    expect(wrapper.get('[data-testid="plaza-price-note"]').text()).toBe('modelPlaza.catalog.displayPriceNote')
  })

  it('lists models newest-version first', () => {
    const wrapper = mountSheet('group', [
      { ...model, name: 'claude-haiku-4-5' },
      { ...model, name: 'claude-opus-4-6' },
      { ...model, name: 'claude-opus-4-8' },
      { ...model, name: 'claude-fable-5-1' },
    ])
    const names = wrapper.findAll('tbody tr').map((row) => row.find('td').text())
    expect(names).toEqual([
      'claude-fable-5-1',
      'claude-opus-4-8',
      'claude-opus-4-6',
      'claude-haiku-4-5',
    ])
  })
})
