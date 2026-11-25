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
        if (key === 'modelPlaza.table.tierToggle') return `阶梯价 ${params?.count} 档`
        if (key === 'modelPlaza.table.timeToggle') return `分时价 ${params?.count} 段`
        if (key === 'modelPlaza.table.cacheWrite1hLine') return `1 小时档 ${params?.amount}`
        if (key === 'modelPlaza.table.peakLine') return `高峰 ${params?.amount}`
        if (key === 'modelPlaza.table.timePeriodLabel') return `${params?.window} ×${params?.multiplier}`
        if (key === 'modelPlaza.table.timePricingWeekdays') return '工作日'
        if (key === 'modelPlaza.table.timePricingRowHintPeak') return `|peak ${params?.window} x${params?.multiplier}`
        if (key === 'modelPlaza.table.reasoningMultiplierBadge') return `${params?.effort} ×${params?.multiplier}`
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
  extraProps: Record<string, unknown> = {},
) {
  return mount(PlazaPriceSheet, {
    props: {
      models,
      rateMultiplier,
      priceMode,
      ...extraProps,
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

  describe('tiers, time-based prices and group peak', () => {
    const tier = (min: number, max: number | null, input: number, output: number) => ({
      min_tokens: min,
      max_tokens: max,
      input_price: input,
      output_price: output,
      cache_write_price: null,
      cache_read_price: null,
      per_request_price: null,
    })
    const tiered: PlazaModel = {
      ...model,
      name: 'gpt-tiered',
      pricing: {
        ...model.pricing!,
        intervals: [tier(272000, null, 1e-5, 3.75e-5), tier(0, 272000, 5e-6, 2.5e-5)],
      },
      official_pricing: {
        ...model.official_pricing!,
        intervals: [tier(0, 272000, 5e-6, 2.5e-5), tier(272000, null, 1e-5, 3.75e-5)],
      },
    }
    const detailCells = (wrapper: ReturnType<typeof mountSheet>) =>
      wrapper.findAll('[data-testid="plaza-detail-row"]').map((row) =>
        row.findAll('td').map((cell) => cell.text().replace(/\s+/g, ' ')),
      )

    it('keeps tiers collapsed behind a toggle and lists every tier in order once opened', async () => {
      const wrapper = mountSheet('group', [tiered])
      const toggle = wrapper.get('[data-testid="plaza-detail-toggle"]')
      expect(toggle.text()).toContain('阶梯价 2 档')
      expect(toggle.attributes('aria-expanded')).toBe('false')
      expect(wrapper.findAll('[data-testid="plaza-detail-row"]')).toHaveLength(0)

      await toggle.trigger('click')
      expect(toggle.attributes('aria-expanded')).toBe('true')
      const cells = detailCells(wrapper)
      expect(cells.map((row) => row[0])).toEqual(['≤272K', '>272K'])
      expect(cells[0][1]).toContain('¥4.00')
      expect(cells[0][1]).toContain('官方价格 ¥5.00')
      expect(cells[1][1]).toContain('¥8.00')
      expect(cells[1][1]).toContain('官方价格 ¥10.00')
      expect(cells[1][2]).toContain('¥30.00')
      expect(wrapper.get('[data-testid="plaza-detail-hint"]').text()).toBe('modelPlaza.table.tierHint')

      await toggle.trigger('click')
      expect(wrapper.findAll('[data-testid="plaza-detail-row"]')).toHaveLength(0)
    })

    it('does not offer a toggle for a single tier or an untiered model', () => {
      const single: PlazaModel = {
        ...model,
        pricing: { ...model.pricing!, intervals: [tier(0, null, 5e-6, 2.5e-5)] },
      }
      expect(mountSheet('group', [single]).find('[data-testid="plaza-detail-toggle"]').exists()).toBe(false)
      expect(mountSheet('group', [model]).find('[data-testid="plaza-detail-toggle"]').exists()).toBe(false)
    })

    it('leaves the official reference blank when tier counts differ', async () => {
      const mismatched: PlazaModel = {
        ...tiered,
        official_pricing: { ...model.official_pricing!, intervals: [tier(0, null, 5e-6, 2.5e-5)] },
      }
      const wrapper = mountSheet('group', [mismatched])
      await wrapper.get('[data-testid="plaza-detail-toggle"]').trigger('click')
      const cells = detailCells(wrapper)
      expect(cells[1][1]).toContain('¥8.00')
      expect(cells[1][1]).not.toContain('官方价格')
    })

    it('lists the official tiers in official mode', async () => {
      const wrapper = mountSheet('official', [tiered])
      await wrapper.get('[data-testid="plaza-detail-toggle"]').trigger('click')
      const cells = detailCells(wrapper)
      expect(cells.map((row) => row[0])).toEqual(['≤272K', '>272K'])
      expect(cells[1][1]).toContain('¥10.00')
      expect(cells[1][1]).not.toContain('官方价格')
    })

    it('marks marginal long-context billing and explains it', async () => {
      const wrapper = mountSheet('group', [{ ...tiered, long_context_basis: 'marginal' }])
      expect(wrapper.text()).toContain('modelPlaza.table.marginalBadge')
      await wrapper.get('[data-testid="plaza-detail-toggle"]').trigger('click')
      expect(wrapper.get('[data-testid="plaza-detail-hint"]').text()).toBe('modelPlaza.table.tierHintMarginal')
    })

    it('expands time-based periods at base price × rate × period multiplier', async () => {
      const timed: PlazaModel = {
        ...model,
        time_pricing: {
          timezone: 'Asia/Shanghai',
          weekdays_only: true,
          periods: [{ start_time: '00:30:00', end_time: '08:30:00', multiplier: 0.5 }],
        },
      }
      const wrapper = mountSheet('group', [timed], 0.8, { peakWindow: '09:00-18:00 ×2', peakRateMultiplier: 2 })
      const toggle = wrapper.get('[data-testid="plaza-detail-toggle"]')
      expect(toggle.text()).toContain('分时价 1 段')
      await toggle.trigger('click')
      const cells = detailCells(wrapper)
      expect(cells).toHaveLength(1)
      expect(cells[0][0]).toBe('工作日 00:30–08:30 ×0.5')
      expect(cells[0][1]).toContain('¥2.00')
      expect(cells[0][2]).toContain('¥10.00')
      // 分时行是不含高峰因子的口径，只在说明里披露重叠部分再乘高峰倍率。
      expect(cells[0][1]).not.toContain('高峰')
      const hint = wrapper.get('[data-testid="plaza-detail-hint"]').text()
      expect(hint).toContain('modelPlaza.table.timePricingRowHintWeekdays')
      expect(hint).toContain('|peak 09:00-18:00 ×2 x2')
    })

    it('hides time-based periods in official mode', () => {
      const timed: PlazaModel = {
        ...model,
        time_pricing: { timezone: 'UTC', periods: [{ start_time: '00:00', end_time: '08:00', multiplier: 0.5 }] },
      }
      expect(mountSheet('official', [timed]).find('[data-testid="plaza-detail-toggle"]').exists()).toBe(false)
    })

    it('adds the peak price under each token price only when the group has a peak factor', () => {
      const row = (wrapper: ReturnType<typeof mountSheet>) =>
        wrapper.get('[data-testid="plaza-model-row"]').findAll('td').map((cell) => cell.text().replace(/\s+/g, ' '))

      const peak = row(mountSheet('group', [model], 0.8, { peakRateMultiplier: 1.5 }))
      expect(peak[1]).toContain('¥4.00')
      expect(peak[1]).toContain('高峰 ¥6.00')
      expect(peak[2]).toContain('高峰 ¥30.00')
      expect(peak[4]).toContain('高峰 ¥0.60')

      for (const factor of [null, 1, 0]) {
        expect(mountSheet('group', [model], 0.8, { peakRateMultiplier: factor }).text()).not.toContain('高峰')
      }
      expect(mountSheet('official', [model], 0.8, { peakRateMultiplier: 1.5 }).text()).not.toContain('高峰')
    })

    it('does not apply the peak factor to per-image or per-request prices', () => {
      const media: PlazaModel = {
        ...model,
        name: 'media-model',
        pricing: { ...model.pricing!, billing_mode: 'image', per_request_price: 0.2 },
      }
      const text = mountSheet('group', [media], 0.8, { peakRateMultiplier: 2 }).text()
      expect(text).toContain('¥0.16')
      expect(text).not.toContain('高峰')
    })

    it('shows the 1-hour cache write price only when it differs from the default one', () => {
      const cacheCell = (wrapper: ReturnType<typeof mountSheet>) =>
        wrapper.get('[data-testid="plaza-model-row"]').findAll('td')[3].text().replace(/\s+/g, ' ')
      const withOneHour = (groupPrice: number | null, officialPrice: number | null): PlazaModel => ({
        ...model,
        pricing: { ...model.pricing!, cache_write_1h_price: groupPrice },
        official_pricing: { ...model.official_pricing!, cache_write_1h_price: officialPrice },
      })

      expect(cacheCell(mountSheet('group', [withOneHour(1e-5, 1.2e-5)]))).toContain('1 小时档 ¥8.00')
      expect(cacheCell(mountSheet('official', [withOneHour(1e-5, 1.2e-5)]))).toContain('1 小时档 ¥12.00')
      expect(cacheCell(mountSheet('group', [withOneHour(6.25e-6, null)]))).not.toContain('1 小时档')
      expect(cacheCell(mountSheet('group', [model]))).not.toContain('1 小时档')
    })

    it('lists configured reasoning-effort multipliers as badges', () => {
      const wrapper = mountSheet('group', [{
        ...model,
        pricing: { ...model.pricing!, reasoning_effort_multipliers: { high: 1.5, low: 0 } },
      }])
      expect(wrapper.text()).toContain('high ×1.5')
      expect(wrapper.text()).not.toContain('low ×')
    })
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
