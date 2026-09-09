import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import type { ModelPlazaGroup, ModelPlazaResponse } from '@/api/modelPlaza'

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: true }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

function group(partial: Partial<ModelPlazaGroup> & Pick<ModelPlazaGroup, 'id' | 'name' | 'platform'>): ModelPlazaGroup {
  return {
    description: '',
    subscription_type: 'standard',
    rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    long_context_pricing_enabled: true,
    models: [{
      name: 'claude-sonnet',
      platform: partial.platform,
      pricing: {
        billing_mode: 'token',
        input_price: 3e-6,
        output_price: 1.5e-5,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [],
      },
      official_pricing: {
        input_price: 3e-6,
        output_price: 1.5e-5,
        cache_write_price: null,
        cache_read_price: null,
      },
    }],
    ...partial,
  }
}

const response: ModelPlazaResponse = {
  description: '',
  groups: [
    group({ id: 1, name: 'Claude Plus', platform: 'anthropic', rate_multiplier: 0.8 }),
    group({ id: 2, name: 'Claude Max', platform: 'anthropic', rate_multiplier: 1.5 }),
    group({ id: 4, name: '精品线路', platform: 'anthropic', rate_multiplier: 0.5 }),
    group({
      id: 3,
      name: 'GPT Plus',
      platform: 'openai',
      rate_multiplier: 1,
      models: [{
        name: 'gpt-5',
        platform: 'openai',
        pricing: {
          billing_mode: 'token',
          input_price: 1e-6,
          output_price: 5e-6,
          cache_write_price: null,
          cache_read_price: null,
          image_input_price: null,
          image_output_price: null,
          per_request_price: null,
          intervals: [],
        },
        official_pricing: {
          input_price: 1e-6,
          output_price: 5e-6,
          cache_write_price: null,
          cache_read_price: null,
        },
      }],
    }),
  ],
}

const i18n = createI18n({
  legacy: false,
  locale: 'zh',
  messages: {
    zh: {
      modelPlaza: {
        title: '模型价格',
        description: 'desc',
        loadFailed: 'fail',
        empty: 'empty',
        noSearchResult: 'none',
        anonymousHint: 'anon',
        catalog: {
          rule: 'rule',
          priceList: '价格列表',
          rateLine: '{rate}x 倍率',
          discountLine: '相当于约 {zhe} 折',
          markupLine: '高于官方参考价',
          sameLine: '与官方同价',
          zheBadge: '{zhe}折',
          platforms: {
            anthropic: 'Claude',
            openai: 'ChatGPT',
          },
        },
        filters: {
          platformLabel: '平台',
          groupLabel: '分组',
          rateLabel: '倍率',
          modelLabel: '模型',
          searchPlaceholder: '搜索',
          all: '全部',
        },
        badges: { exclusive: '专属', subscription: '订阅' },
        detail: { noModels: '无', peakNote: '', longContextDisabledNote: '' },
        table: {
          model: '模型',
          input: '输入',
          output: '输出',
          cache: '缓存',
          paidPrice: '实付',
          officialPrice: '官方',
          rate: '倍率',
          unitPerMillion: '$ / 1M',
        },
      },
    },
  },
})

function mountContent(embedded: boolean) {
  return mount(ModelPlazaContent, {
    props: { response, loading: false, embedded },
    global: {
      plugins: [i18n],
      stubs: {
        PlazaModelPricingTable: { template: '<div class="price-table-stub" />' },
        PlazaPriceSheet: { template: '<div class="price-sheet-stub" />' },
        Icon: true,
        GroupBadge: { template: '<span class="group-badge-stub" />' },
      },
    },
  })
}

describe('ModelPlazaContent catalog', () => {
  it('uses stacked filters on the public page', () => {
    const wrapper = mountContent(false)
    expect(wrapper.find('[data-testid="plaza-platform-tabs"]').exists()).toBe(false)
    expect(wrapper.findAll('.group-badge-stub').length).toBe(4)
  })

  it('lists every enabled group by its admin name', async () => {
    const wrapper = mountContent(true)
    const cards = wrapper.get('[data-testid="plaza-group-cards"]').text()
    expect(cards).toContain('精品线路')
    expect(cards).toContain('Claude Plus')
    expect(cards).toContain('GPT Plus')
    expect(wrapper.findAll('.price-sheet-stub')).toHaveLength(1)
    expect(wrapper.get('[data-testid="plaza-group-name"]').text()).toBe('精品线路')
  })

  it('switches category to the matching groups', async () => {
    const wrapper = mountContent(true)
    const tabs = wrapper.findAll('[data-testid="plaza-platform-tabs"] button')
    expect(tabs.length).toBeGreaterThanOrEqual(3)
    await tabs[tabs.length - 1].trigger('click')
    expect(wrapper.get('[data-testid="plaza-group-cards"]').text()).toContain('GPT Plus')
    expect(wrapper.get('[data-testid="plaza-group-cards"]').text()).not.toContain('Claude Plus')
  })
})
