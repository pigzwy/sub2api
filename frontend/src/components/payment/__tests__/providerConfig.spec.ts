import { describe, expect, it } from 'vitest'
import {
  PAYMENT_CURRENCY_OPTIONS,
  PROVIDER_CONFIG_FIELDS,
  isBuiltInAlipayMethod,
  isBuiltInWxpayMethod,
  parseEasyPayCustomMethods,
  serializeEasyPayCustomMethods,
} from '@/components/payment/providerConfig'

function findField(providerKey: string, key: string) {
  const fields = PROVIDER_CONFIG_FIELDS[providerKey] || []
  return fields.find(field => field.key === key)
}

describe('PROVIDER_CONFIG_FIELDS.wxpay', () => {
  it('keeps admin form validation aligned with backend-required credentials', () => {
    expect(findField('wxpay', 'publicKeyId')?.optional).toBeFalsy()
    expect(findField('wxpay', 'certSerial')?.optional).toBeFalsy()
  })

  it('only keeps the simplified visible credential set in the admin form', () => {
    expect(findField('wxpay', 'mpAppId')).toBeUndefined()
    expect(findField('wxpay', 'h5AppName')).toBeUndefined()
    expect(findField('wxpay', 'h5AppUrl')).toBeUndefined()
  })
})

describe('PROVIDER_CONFIG_FIELDS.airwallex', () => {
  it('adds currency config with CNY as the default', () => {
    const currency = findField('airwallex', 'currency')

    expect(currency?.defaultValue).toBe('CNY')
    expect(currency?.hintKey).toBe('admin.settings.payment.field_paymentCurrencyHint')
    expect(currency?.options).toBe(PAYMENT_CURRENCY_OPTIONS)
  })

  it('marks accountId as optional and explains when it can be left blank', () => {
    const accountId = findField('airwallex', 'accountId')

    expect(accountId?.optional).toBe(true)
    expect(accountId?.clearable).toBe(true)
    expect(accountId?.hintKey).toBe('admin.settings.payment.field_accountIdHint')
  })

  it('explains that apiBase must match the Airwallex key environment', () => {
    expect(findField('airwallex', 'apiBase')?.hintKey).toBe('admin.settings.payment.field_airwallexApiBaseHint')
  })
})

describe('PROVIDER_CONFIG_FIELDS.infini', () => {
  it('defaults hosted checkout to USD crypto and Infini API hosts', () => {
    const currency = findField('infini', 'currency')
    expect(currency?.defaultValue).toBe('USD')
    expect(currency?.options).toEqual([{ value: 'USD', label: 'USD' }])
    expect(currency?.hintKey).toBe('admin.settings.payment.field_infiniCurrencyHint')
    expect(findField('infini', 'apiBase')?.defaultValue).toBe('https://openapi.infini.money')
    expect(findField('infini', 'apiBase')?.hintKey).toBe('admin.settings.payment.field_infiniApiBaseHint')
    expect(findField('infini', 'payMethods')?.defaultValue).toBe('1')
    expect(findField('infini', 'secretKey')?.sensitive).toBe(true)
    expect(findField('infini', 'webhookSecret')?.sensitive).toBe(true)
  })

  it('offers the Ed25519 private key for V2 auth and keeps the legacy secret optional', () => {
    // 两种凭据二选一，由后端校验至少填一个；表单层面都不强制必填，
    // 否则已迁移到 V2 的实例会被要求再填一个已停用的 HMAC 密钥。
    const privateKey = findField('infini', 'privateKey')
    expect(privateKey?.sensitive).toBe(true)
    expect(privateKey?.optional).toBe(true)
    expect(privateKey?.hintKey).toBe('admin.settings.payment.field_infiniPrivateKeyHint')
    const secretKey = findField('infini', 'secretKey')
    expect(secretKey?.optional).toBe(true)
    expect(secretKey?.hintKey).toBe('admin.settings.payment.field_infiniSecretKeyHint')
    // Key ID 和 Webhook Secret 两种认证都需要，仍是必填。
    expect(findField('infini', 'keyId')?.optional).toBeFalsy()
    expect(findField('infini', 'webhookSecret')?.optional).toBeFalsy()
    // 私钥排在旧密钥前面，表单里先看到推荐的填法。
    const keys = (PROVIDER_CONFIG_FIELDS.infini || []).map((field) => field.key)
    expect(keys.indexOf('privateKey')).toBeLessThan(keys.indexOf('secretKey'))
  })
})

describe('PROVIDER_CONFIG_FIELDS.stripe', () => {
  it('adds currency config with CNY as the default', () => {
    const currency = findField('stripe', 'currency')

    expect(currency?.defaultValue).toBe('CNY')
    expect(currency?.hintKey).toBe('admin.settings.payment.field_paymentCurrencyHint')
    expect(currency?.options).toBe(PAYMENT_CURRENCY_OPTIONS)
  })
})

describe('EasyPay custom methods config', () => {
  it('parses customMethods from the JSON string stored in provider config', () => {
    expect(parseEasyPayCustomMethods(
      '[{"type":"ldc","upstreamType":"epay","displayName":"LDC"},{"type":"usdt_trc20","upstreamType":"usdt","displayName":"USDT-TRC20"}]',
    )).toEqual([
      { type: 'ldc', upstreamType: 'epay', displayName: 'LDC' },
      { type: 'usdt_trc20', upstreamType: 'usdt', displayName: 'USDT-TRC20' },
    ])
  })

  it('serializes non-empty custom methods into the config string format', () => {
    expect(serializeEasyPayCustomMethods([
      { type: 'ldc', upstreamType: 'epay', displayName: 'LDC' },
      { type: '  ', upstreamType: 'ignored', displayName: 'Ignored' },
      { type: 'usdt_trc20', upstreamType: 'usdt', displayName: '' },
    ])).toBe('[{"type":"ldc","upstreamType":"epay","displayName":"LDC"},{"type":"usdt_trc20","upstreamType":"usdt","displayName":""}]')
  })

  it('returns an empty string for invalid or empty custom methods', () => {
    expect(parseEasyPayCustomMethods('not-json')).toEqual([])
    expect(serializeEasyPayCustomMethods([{ type: '', upstreamType: 'epay', displayName: 'LDC' }])).toBe('')
  })
})

describe('built-in payment method helpers', () => {
  it('only treats exact built-in aliases as Alipay or WeChat Pay', () => {
    expect(isBuiltInAlipayMethod('alipay')).toBe(true)
    expect(isBuiltInAlipayMethod('alipay_direct')).toBe(true)
    expect(isBuiltInAlipayMethod('card_alipay')).toBe(false)

    expect(isBuiltInWxpayMethod('wxpay')).toBe(true)
    expect(isBuiltInWxpayMethod('wxpay_direct')).toBe(true)
    expect(isBuiltInWxpayMethod('card_wxpay')).toBe(false)
  })
})
