import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import { formatPaymentAmount } from '@/components/payment/currency'
import RechargePackageGrid from '@/components/payment/RechargePackageGrid.vue'
import RechargeCheckoutDialog from '@/components/payment/RechargeCheckoutDialog.vue'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import type { CheckoutInfoResponse, MethodLimit, SubscriptionPlan } from '@/types/payment'

const routeState = vi.hoisted(() => ({
  path: '/purchase',
  query: {} as Record<string, unknown>,
}))

const routerReplace = vi.hoisted(() => vi.fn())
const routerPush = vi.hoisted(() => vi.fn())
const routerResolve = vi.hoisted(() => vi.fn(() => ({ href: '/payment/stripe?mock=1' })))
const createOrder = vi.hoisted(() => vi.fn())
const refreshUser = vi.hoisted(() => vi.fn())
const fetchActiveSubscriptions = vi.hoisted(() => vi.fn().mockResolvedValue(undefined))
const showError = vi.hoisted(() => vi.fn())
const showInfo = vi.hoisted(() => vi.fn())
const showWarning = vi.hoisted(() => vi.fn())
const getCheckoutInfo = vi.hoisted(() => vi.fn())
const quoteOrder = vi.hoisted(() => vi.fn())
const bridgeInvoke = vi.hoisted(() => vi.fn())
const translate = vi.hoisted(() => vi.fn((key: string) => key))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({
      replace: routerReplace,
      push: routerPush,
      resolve: routerResolve,
    }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: translate,
    }),
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: {
      username: 'demo-user',
      balance: 0,
    },
    refreshUser,
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    createOrder,
  }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: [],
    fetchActiveSubscriptions,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showInfo,
    showWarning,
  }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getCheckoutInfo,
    quoteOrder,
  },
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => true,
}))

function checkoutInfoFixture(overrides: Partial<CheckoutInfoResponse> = {}) {
  const wxpayMethod: MethodLimit = {
    daily_limit: 0,
    daily_used: 0,
    daily_remaining: 0,
    single_min: 0,
    single_max: 0,
    fee_rate: 0,
    available: true,
  }
  const data: CheckoutInfoResponse = {
    methods: {
      wxpay: wxpayMethod,
    },
    global_min: 0,
    global_max: 0,
    plans: [],
    balance_disabled: false,
    balance_recharge_multiplier: 1,
    subscription_usd_to_cny_rate: 0,
    usdt_usd_to_cny_rate: 0,
    recharge_fee_rate: 0,
    help_text: '',
    help_image_url: '',
    stripe_publishable_key: '',
  }

  return {
    data: { ...data, ...overrides },
  }
}

function checkoutInfoWithPlansFixture(options: {
  checkout?: Partial<CheckoutInfoResponse>
  method?: Partial<MethodLimit>
  plan?: Partial<SubscriptionPlan>
} = {}) {
  const base = checkoutInfoFixture(options.checkout).data
  const plan: SubscriptionPlan = {
    id: 7,
    group_id: 3,
    name: 'Starter',
    description: '',
    price: 128,
    original_price: 0,
    validity_days: 30,
    validity_unit: 'day',
    rate_multiplier: 1,
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    features: [],
    group_platform: 'openai',
    sort_order: 1,
    for_sale: true,
    group_name: 'OpenAI',
    ...options.plan,
  }

  return {
    data: {
      ...base,
      methods: {
        ...base.methods,
        wxpay: {
          ...base.methods.wxpay,
          ...options.method,
        },
      },
      plans: [plan],
    },
  }
}

function quoteOrderFixture(data: { amount?: number; payment_type?: string; order_type?: string }, extras: Record<string, unknown> = {}) {
  const amount = Number(data.amount) || 0
  const paymentType = String(data.payment_type || '')
  const currency = extras.currency
    || (paymentType === 'infini' || paymentType === 'stripe' ? 'USD' : paymentType === 'usdt_trc20' ? 'USDT' : 'CNY')
  return {
    data: {
      order_type: data.order_type || 'balance',
      payment_type: paymentType,
      package_amount: amount.toFixed(2),
      credit_amount: extras.credit_amount ?? amount.toFixed(2),
      pay_amount: extras.pay_amount ?? amount.toFixed(2),
      pay_amount_value: extras.pay_amount_value ?? Number(extras.pay_amount ?? amount),
      fee_amount: extras.fee_amount ?? '0.00',
      fee_rate: extras.fee_rate ?? 0,
      fx_rate: extras.fx_rate ?? 0,
      fx_converted: extras.fx_converted ?? false,
      currency,
      ...extras,
    },
  }
}

function jsapiOrderFixture(resumeToken: string) {
  return {
    order_id: 123,
    amount: 88,
    pay_amount: 88,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    payment_type: 'wxpay',
    out_trade_no: 'sub2_jsapi_123',
    result_type: 'jsapi_ready' as const,
    resume_token: resumeToken,
    jsapi: {
      appId: 'wx123',
      timeStamp: '1712345678',
      nonceStr: 'nonce',
      package: 'prepay_id=wx123',
      signType: 'RSA',
      paySign: 'signed',
    },
  }
}

function oauthOrderFixture() {
  return {
    order_id: 456,
    amount: 128,
    pay_amount: 128,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    payment_type: 'wxpay',
    result_type: 'oauth_required' as const,
    oauth: {
      authorize_url: '/api/v1/auth/oauth/wechat/payment/start?payment_type=wxpay&redirect=%2Fpurchase%3Ffrom%3Dwechat',
      appid: 'wx123',
      scope: 'snsapi_base',
      redirect_url: '/auth/wechat/payment/callback',
    },
  }
}

async function mountSubscriptionConfirm(options: Parameters<typeof checkoutInfoWithPlansFixture>[0] = {}) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = {
    tab: 'subscription',
    group: '3',
  }
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoWithPlansFixture(options))
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: {
          template: '<div><slot /></div>',
        },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

async function mountSubscriptionPlanList(planCount: number) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = { tab: 'subscription' }
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  const basePlan = checkoutInfoWithPlansFixture().data.plans[0]
  const plans = Array.from({ length: planCount }, (_, index) => ({
    ...basePlan,
    id: index + 1,
    name: `Plan ${index + 1}`,
  }))
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ plans }))
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: {
          template: '<div><slot /></div>',
        },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

describe('PaymentView help text', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    createOrder.mockReset()
    window.localStorage.clear()
  })

  async function mountHelp(help_text: string, help_image_url = '') {
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ help_text, help_image_url }))
    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    return wrapper
  }

  it('renders headings, emphasis, links, and lists in payment help without starting checkout', async () => {
    const wrapper = await mountHelp('## Recharge help\n\n**Read first**\n\n- [Contact support](https://example.com/help)')
    const help = wrapper.get('.markdown-body')
    expect(help.get('h2').text()).toBe('Recharge help')
    expect(help.get('strong').text()).toBe('Read first')
    expect(help.get('li a').attributes('href')).toBe('https://example.com/help')
    expect(createOrder).not.toHaveBeenCalled()
  })

  it('removes scripts, event handlers, and unsafe URLs from rendered help', async () => {
    const wrapper = await mountHelp([
      '<script>alert(1)</script>',
      '<img src="https://example.com/help.png" onerror="alert(1)">',
      '[Unsafe](javascript:alert%281%29)',
      '[Support](https://example.com/help)',
    ].join('\n\n'))
    const help = wrapper.get('.markdown-body')
    expect(help.find('script').exists()).toBe(false)
    expect(help.get('img').attributes('onerror')).toBeUndefined()
    expect(help.findAll('a').map(link => link.attributes('href'))).toEqual([undefined, 'https://example.com/help'])
  })

  it('keeps plain-text soft line breaks and the separate help image preview', async () => {
    const wrapper = await mountHelp('First line\nSecond line', 'https://example.com/help.png')
    const help = wrapper.get('.markdown-body')
    expect(help.get('p').text()).toBe('First line\nSecond line')
    expect(help.find('br').exists()).toBe(false)
    await wrapper.get('img').trigger('click')
    expect(wrapper.findAll('img')).toHaveLength(2)
    expect(wrapper.findAll('img')[1].attributes('src')).toBe('https://example.com/help.png')
  })

  it('keeps image-only help without an empty Markdown container', async () => {
    const wrapper = await mountHelp('', 'https://example.com/help.png')
    expect(wrapper.find('.markdown-body').exists()).toBe(false)
    expect(wrapper.get('img').attributes('src')).toBe('https://example.com/help.png')
  })
})

describe('PaymentView subscription plan grid', () => {
  it.each([3, 4, 6])('keeps %i plans on the existing mobile/tablet/desktop grid', async (planCount) => {
    const wrapper = await mountSubscriptionPlanList(planCount)
    const cards = wrapper.findAllComponents(SubscriptionPlanCard)

    expect(cards).toHaveLength(planCount)
    expect([...(cards[0].element.parentElement?.classList ?? [])]).toEqual(expect.arrayContaining([
      'grid',
      'grid-cols-1',
      'sm:grid-cols-2',
      'lg:grid-cols-3',
    ]))
  })
})

describe('PaymentView recharge rate preview', () => {
  beforeEach(() => {
    window.localStorage.clear()
    createOrder.mockReset()
    quoteOrder.mockReset().mockImplementation(async (data) => quoteOrderFixture(data))
  })

  it('uses the selected payment method currency in both locale templates', async () => {
    translate.mockClear()
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      balance_recharge_multiplier: 0.5,
      methods: {
        stripe: {
          ...checkoutInfoFixture().data.methods.wxpay,
          currency: 'USD',
        },
      },
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
          RechargeCheckoutDialog: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(RechargePackageGrid).vm.$emit('select', 10)
    await flushPromises()

    expect(translate).toHaveBeenCalledWith('payment.rechargeRatePreview', {
      currency: 'USD',
      usd: '0.50',
    })
    expect(en.payment.rechargeRatePreview).toBe('Current rate: 1 {currency} = {usd} USD')
    expect(zh.payment.rechargeRatePreview).toBe('当前倍率：1 {currency} = {usd} USD')
  })

  it('submits the existing balance order payload for a selected package', async () => {
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture())
    createOrder.mockReset().mockResolvedValue({
      order_id: 88,
      amount: 50,
      pay_amount: 50,
      qr_code: 'qr',
      expires_at: '2099-01-01T00:10:00.000Z',
      payment_type: 'wxpay',
      result_type: 'qr_ready',
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(RechargePackageGrid).vm.$emit('select', 50)
    await flushPromises()
    wrapper.getComponent(RechargeCheckoutDialog).vm.$emit('confirm', 'wxpay')
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      amount: 50,
      order_type: 'balance',
      payment_type: 'wxpay',
    }))
  })

  it('renders recharge cards from checkout-info instead of hardcoded packages', async () => {
    window.localStorage.clear()
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      balance_recharge_packages: [
        { id: 'trial', amount: 80, bonus: 5, credit: 85, name: '试用', description: '后台配置', name_en: 'Trial', description_en: 'From admin' },
      ],
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    expect(wrapper.getComponent(RechargePackageGrid).props('packages')).toEqual([
      expect.objectContaining({ id: 'trial', amount: 80, name: '试用' }),
    ])
  })

  it('credits pay amount times rate plus that package bonus only', async () => {
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      balance_recharge_multiplier: 0.14,
      balance_recharge_packages: [
        { id: 'cny100', amount: 100, bonus: 0, name: '基础', description: '' },
        { id: 'cny200', amount: 200, bonus: 5, name: '加赠', description: '' },
      ],
    }))
    quoteOrder.mockImplementation(async (data) => {
      const credit = data.amount === 200 ? '33.00' : data.amount === 100 ? '14.00' : Number(data.amount || 0).toFixed(2)
      return quoteOrderFixture(data, { credit_amount: credit })
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(RechargePackageGrid).vm.$emit('select', 100)
    await flushPromises()

    expect(wrapper.getComponent(RechargeCheckoutDialog).props('creditAmountLabel')).toBe('$14.00')

    wrapper.getComponent(RechargeCheckoutDialog).vm.$emit('close')
    await flushPromises()
    wrapper.getComponent(RechargePackageGrid).vm.$emit('select', 200)
    await flushPromises()

    expect(wrapper.getComponent(RechargeCheckoutDialog).props('creditAmountLabel')).toBe('$33.00')
  })

  it('keeps checkout amount currency aligned with the selected pay lane', async () => {
    window.localStorage.clear()
    const method: MethodLimit = {
      daily_limit: 0,
      daily_used: 0,
      daily_remaining: 0,
      single_min: 0,
      single_max: 0,
      fee_rate: 0,
      available: true,
    }
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      methods: {
        alipay: { ...method, currency: 'CNY' },
        usdt_trc20: { ...method, currency: 'USDT' },
      },
    }))
    createOrder.mockReset().mockResolvedValue({
      order_id: 91,
      amount: 50,
      pay_amount: 50,
      qr_code: 'qr',
      expires_at: '2099-01-01T00:10:00.000Z',
      payment_type: 'usdt_trc20',
      result_type: 'qr_ready',
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(RechargePackageGrid).vm.$emit('select', 50)
    await flushPromises()

    const dialog = wrapper.getComponent(RechargeCheckoutDialog)
    expect(wrapper.get('[data-testid="selected-payment-method"]').text()).toBe('alipay')
    expect(dialog.props('currency')).toBe('CNY')
    expect(dialog.props('payAmountLabel')).toBe(formatPaymentAmount(50, 'CNY'))

    dialog.vm.$emit('update:lane', 'usdt')
    await flushPromises()

    expect(wrapper.get('[data-testid="selected-payment-method"]').text()).toBe('usdt_trc20')
    expect(dialog.props('currency')).toBe('USDT')
    expect(dialog.props('payAmountLabel')).toBe(formatPaymentAmount(50, 'USDT'))
    expect(dialog.props('lane')).toBe('usdt')

    dialog.vm.$emit('confirm', 'usdt_trc20')
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      amount: 50,
      order_type: 'balance',
      payment_type: 'usdt_trc20',
    }))
  })

  it('converts Infini USDT pay amount by the USD/CNY rate without changing credit or order amount', async () => {
    window.localStorage.clear()
    const method: MethodLimit = {
      daily_limit: 0,
      daily_used: 0,
      daily_remaining: 0,
      single_min: 0,
      single_max: 0,
      fee_rate: 0,
      available: true,
    }
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      subscription_usd_to_cny_rate: 6.67,
      methods: {
        alipay: { ...method, currency: 'CNY' },
        infini: { ...method, currency: 'USD' },
      },
    }))
    createOrder.mockReset().mockResolvedValue({
      order_id: 93,
      amount: 50,
      pay_amount: 7.5,
      qr_code: 'qr',
      expires_at: '2099-01-01T00:10:00.000Z',
      payment_type: 'infini',
      result_type: 'qr_ready',
    })
    quoteOrder.mockImplementation(async (data) => {
      if (data.payment_type === 'infini') {
        return quoteOrderFixture(data, {
          pay_amount: '7.50',
          pay_amount_value: 7.5,
          credit_amount: '50.00',
          fx_rate: 6.67,
          fx_converted: true,
          currency: 'USD',
        })
      }
      return quoteOrderFixture(data, { currency: 'CNY', credit_amount: '50.00' })
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(RechargePackageGrid).vm.$emit('select', 50)
    await flushPromises()

    const dialog = wrapper.getComponent(RechargeCheckoutDialog)
    expect(dialog.props('payAmountLabel')).toBe(formatPaymentAmount(50, 'CNY'))
    expect(dialog.props('creditAmountLabel')).toBe('$50.00')

    dialog.vm.$emit('update:lane', 'usdt')
    await flushPromises()

    expect(wrapper.get('[data-testid="selected-payment-method"]').text()).toBe('infini')
    expect(dialog.props('currency')).toBe('USD')
    expect(dialog.props('payAmountLabel')).toBe(formatPaymentAmount(7.5, 'USD'))
    expect(dialog.props('creditAmountLabel')).toBe('$50.00')
    expect(dialog.props('fxRateLabel')).toBe('payment.fxRateLabel')

    dialog.vm.$emit('confirm', 'infini')
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      amount: 50,
      order_type: 'balance',
      payment_type: 'infini',
    }))
  })

  it('updates CNY/USD preview before creating a Stripe order', async () => {
    window.localStorage.clear()
    const method: MethodLimit = {
      daily_limit: 0,
      daily_used: 0,
      daily_remaining: 0,
      single_min: 0,
      single_max: 0,
      fee_rate: 0,
      available: true,
    }
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      subscription_usd_to_cny_rate: 6.67,
      methods: {
        alipay: { ...method, currency: 'CNY' },
        stripe: { ...method, currency: 'USD' },
      },
    }))
    createOrder.mockReset().mockResolvedValue({
      order_id: 92,
      amount: 100,
      pay_amount: 100,
      qr_code: 'qr',
      expires_at: '2099-01-01T00:10:00.000Z',
      payment_type: 'stripe',
      result_type: 'qr_ready',
    })

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(RechargePackageGrid).vm.$emit('select', 100)
    await flushPromises()

    const dialog = wrapper.getComponent(RechargeCheckoutDialog)
    expect(wrapper.get('[data-testid="selected-payment-method"]').text()).toBe('alipay')
    expect(dialog.props('currency')).toBe('CNY')
    expect(dialog.props('payAmountLabel')).toBe(formatPaymentAmount(100, 'CNY'))
    expect(dialog.props('selected')).toBe('alipay')

    dialog.vm.$emit('select', 'stripe')
    await flushPromises()

    expect(createOrder).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="selected-payment-method"]').text()).toBe('stripe')
    expect(dialog.props('currency')).toBe('USD')
    expect(dialog.props('payAmountLabel')).toBe(formatPaymentAmount(100, 'USD'))
    expect(dialog.props('selected')).toBe('stripe')

    dialog.vm.$emit('confirm', 'stripe')
    dialog.vm.$emit('confirm', 'stripe')
    await flushPromises()

    expect(createOrder).toHaveBeenCalledTimes(1)
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      amount: 100,
      order_type: 'balance',
      payment_type: 'stripe',
    }))
  })

  it('does not create an order for an unavailable checkout method', async () => {
    window.localStorage.clear()
    const method: MethodLimit = {
      daily_limit: 0,
      daily_used: 0,
      daily_remaining: 0,
      single_min: 0,
      single_max: 0,
      fee_rate: 0,
      available: true,
    }
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      methods: {
        alipay: { ...method, currency: 'CNY' },
        stripe: { ...method, currency: 'USD', available: false },
      },
    }))
    createOrder.mockReset()

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(RechargePackageGrid).vm.$emit('select', 100)
    await flushPromises()

    const dialog = wrapper.getComponent(RechargeCheckoutDialog)
    dialog.vm.$emit('select', 'stripe')
    await flushPromises()
    expect(wrapper.get('[data-testid="selected-payment-method"]').text()).toBe('alipay')

    dialog.vm.$emit('confirm', 'stripe')
    await flushPromises()
    expect(createOrder).not.toHaveBeenCalled()
  })

  it('uses a wide page shell and a compact bonus notice', async () => {
    window.localStorage.clear()
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      balance_recharge_packages: [
        { id: 'trial', amount: 80, bonus: 5, credit: 85, name: '试用', description: '' },
      ],
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()

    const shell = wrapper.find('.max-w-7xl')
    expect(shell.exists()).toBe(true)
    expect(shell.classes()).toEqual(expect.arrayContaining(['w-full', 'space-y-5']))

    const banner = wrapper.get('[data-testid="recharge-bonus-banner"]')
    expect(banner.classes()).toEqual(expect.arrayContaining(['flex', 'w-fit']))
    expect(banner.classes()).not.toEqual(expect.arrayContaining(['inline-flex', 'justify-between']))
    expect(banner.text()).toContain('payment.bonusBannerTitle')
    expect(banner.text()).not.toContain('payment.bonusBannerDesc')
    expect(banner.element.parentElement?.className).toContain('items-center')
    expect(banner.element.parentElement?.textContent).toContain('payment.tabPayAsYouGo')
    expect(wrapper.text()).toContain('payment.currentBalance')
    expect(wrapper.text()).toContain('0.00')
    const balanceCard = wrapper.get('[data-testid="recharge-balance-card"]')
    expect(balanceCard.classes()).toEqual(expect.arrayContaining(['h-12', 'rounded-2xl', 'shadow-card', 'w-fit', 'px-5']))
    expect(balanceCard.classes()).not.toEqual(expect.arrayContaining(['rounded-full']))
    expect(balanceCard.text()).toContain('payment.rechargeAccount')
    expect(balanceCard.find('.space-y-0\\.5').exists()).toBe(false)
  })
})

describe('PaymentView subscription confirmation amounts', () => {
  it('keeps subscription CNY conversion on the subscription rate when a USDT rate is also set', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 1,
        subscription_usd_to_cny_rate: 7.15,
        usdt_usd_to_cny_rate: 6.67,
      },
      method: {
        currency: 'CNY',
      },
      plan: {
        price: 9.99,
      },
    })

    const text = wrapper.text()
    expect(text).toContain(formatPaymentAmount(71.43, 'CNY'))
    expect(text).not.toContain(formatPaymentAmount(66.63, 'CNY'))
  })

  it('shows converted CNY pay amount using the subscription rate, not the balance multiplier', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 0.14,
        subscription_usd_to_cny_rate: 7.15,
      },
      method: {
        currency: 'CNY',
      },
      plan: {
        price: 9.99,
        original_price: 12.99,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(71.43, 'CNY')
    const convertedOriginalPrice = formatPaymentAmount(92.88, 'CNY')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(convertedOriginalPrice)
    expect(text).not.toContain(formatPaymentAmount(9.99, 'CNY'))
    // 换算必须使用订阅汇率（×7.15），而不是余额倍率（÷0.14 = 71.36）
    expect(text).not.toContain(formatPaymentAmount(71.36, 'CNY'))
    expect(wrapper.findAll('button').some(button => button.text().includes(convertedPrice))).toBe(true)
  })

  it('keeps plan price when the subscription rate is not configured or payment currency is not CNY', async () => {
    // opt-in 回归锁：即使余额倍率已配置，未配置订阅汇率时 CNY 订阅仍按 price 直付
    const cnyWrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 0.14,
        subscription_usd_to_cny_rate: 0,
      },
      method: {
        currency: 'CNY',
      },
      plan: {
        price: 7.99,
      },
    })

    expect(cnyWrapper.text()).toContain(formatPaymentAmount(7.99, 'CNY'))
    expect(cnyWrapper.text()).not.toContain(formatPaymentAmount(57.07, 'CNY'))
    expect(cnyWrapper.text()).not.toContain(formatPaymentAmount(57.13, 'CNY'))

    const usdWrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_cny_rate: 7.15,
      },
      method: {
        currency: 'USD',
      },
      plan: {
        price: 7.99,
        original_price: 9.99,
      },
    })

    expect(usdWrapper.text()).toContain(formatPaymentAmount(7.99, 'USD'))
    expect(usdWrapper.text()).toContain(formatPaymentAmount(9.99, 'USD'))
  })

  it('adds fee rate after CNY rate conversion to match backend pay_amount', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_cny_rate: 7.15,
        recharge_fee_rate: 2.5,
      },
      method: {
        currency: 'CNY',
      },
      plan: {
        price: 9.99,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(71.43, 'CNY')
    const fee = formatPaymentAmount(1.79, 'CNY')
    const total = formatPaymentAmount(73.22, 'CNY')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(fee)
    expect(text).toContain(total)
    expect(wrapper.findAll('button').some(button => button.text().includes(total))).toBe(true)
  })
})

describe('PaymentView payment recovery', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined
  })

  it('restores a custom EasyPay method as the selected payment method', async () => {
    getCheckoutInfo.mockResolvedValue(checkoutInfoFixture({
      methods: {
        wxpay: checkoutInfoFixture().data.methods.wxpay,
        ldc: {
          daily_limit: 0,
          daily_used: 0,
          daily_remaining: 0,
          single_min: 0,
          single_max: 0,
          fee_rate: 0,
          available: true,
          display_name: 'LDC Pay',
        },
      },
    }))
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 888,
      amount: 66,
      qrCode: 'ldc-qr',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'ldc',
      payUrl: 'https://pay.example.com/ldc',
      outTradeNo: 'sub2_ldc_888',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 66,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: '',
      createdAt: Date.now(),
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: {
            template: '<div><slot /></div>',
          },
          PaymentStatusPanel: {
            template: '<button data-test="payment-done" @click="$emit(\'done\')" />',
          },
          PaymentMethodSelector: {
            props: ['selected'],
            template: '<div data-test="method-selector">{{ selected }}</div>',
          },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()
    await wrapper.find('[data-test="payment-done"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="selected-payment-method"]').text()).toBe('ldc')
  })
})

describe('PaymentView WeChat JSAPI flow', () => {
  beforeEach(() => {
    routeState.path = '/purchase'
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-123',
    }
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture())
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = {
      invoke: bridgeInvoke,
    }
  })

  it('resets payment state and redirects to /payment/result after JSAPI reports success', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-123'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:ok' })
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(routerPush).toHaveBeenCalledWith({
      path: '/payment/result',
      query: {
        order_id: '123',
        out_trade_no: 'sub2_jsapi_123',
        resume_token: 'resume-token-123',
      },
    })
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('resets payment state when JSAPI reports cancellation', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-cancel'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:cancel' })
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(showInfo).toHaveBeenCalledWith('payment.qr.cancelled')
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('clears stale recovery state when JSAPI never becomes available', async () => {
    vi.useFakeTimers()
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-missing-bridge'))
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      'payment.errors.wechatJsapiUnavailable payment.errors.wechatOpenInWeChatHint',
    )
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
    expect(wrapper.html()).not.toContain('payment-status-panel-stub')
  })

  it('clears a stale recovery snapshot before handling wechat resume callback params', async () => {
    createOrder.mockRejectedValueOnce(new Error('resume failed'))
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 999,
      amount: 66,
      qrCode: 'stale-qr',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/stale',
      outTradeNo: 'stale-out-trade-no',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 66,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: '',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }))

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      wechat_resume_token: 'resume-token-123',
    }))
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('keeps subscription resume context for token-only WeChat callbacks', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-subscription-7',
      payment_type: 'wxpay_direct',
      order_type: 'subscription',
      plan_id: '7',
    }
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())
    createOrder.mockResolvedValue(oauthOrderFixture())

    const originalLocation = window.location
    const locationState = {
      href: 'http://localhost/purchase',
      origin: 'http://localhost',
    }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState,
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      payment_type: 'wxpay',
      order_type: 'subscription',
      plan_id: 7,
      wechat_resume_token: 'resume-subscription-7',
    }))
    expect(locationState.href).toContain('/api/v1/auth/oauth/wechat/payment/start?')
    expect(new URL(locationState.href, 'http://localhost').searchParams.get('redirect')).toBe(
      '/purchase?from=wechat&payment_type=wxpay&order_type=subscription&plan_id=7',
    )

    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    })
  })

  it('falls back to QR flow when mobile WeChat payment is unavailable', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-h5',
      payment_type: 'wxpay_direct',
    }
    createOrder
      .mockRejectedValueOnce({ reason: 'WECHAT_H5_NOT_AUTHORIZED' })
      .mockResolvedValueOnce({
        order_id: 778,
        amount: 88,
        pay_amount: 88,
        fee_rate: 0,
        expires_at: '2099-01-01T00:10:00.000Z',
        payment_type: 'wxpay',
        qr_code: 'weixin://wxpay/bizpayurl?pr=fallback-native',
        out_trade_no: 'sub2_qr_778',
      })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenNthCalledWith(1, expect.objectContaining({
      payment_type: 'wxpay',
      is_mobile: true,
      wechat_resume_token: 'resume-token-h5',
    }))
    expect(createOrder).toHaveBeenNthCalledWith(2, expect.objectContaining({
      payment_type: 'wxpay',
      is_mobile: false,
      payment_source: 'hosted_redirect',
    }))
    expect(showWarning).toHaveBeenCalledWith('payment.errors.mobilePaymentFallbackToQr')
    expect(showError).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('weixin://wxpay/bizpayurl?pr=fallback-native')
  })
})
