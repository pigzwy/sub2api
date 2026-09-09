import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AmountInput from '@/components/payment/AmountInput.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

describe('AmountInput', () => {
  it('renders formatted quick amounts with a currency prefix', () => {
    const wrapper = mount(AmountInput, {
      props: {
        modelValue: null,
        amounts: [10, 1000],
      },
    })

    expect(wrapper.get('[data-testid="quick-amount-10"]').text()).toContain('$')
    expect(wrapper.get('[data-testid="quick-amount-10"]').text()).toContain('10')
    expect(wrapper.get('[data-testid="quick-amount-1000"]').text()).toContain('1,000')
  })

  it('marks the selected preset and emits the clicked amount', async () => {
    const wrapper = mount(AmountInput, {
      props: {
        modelValue: 10,
        amounts: [10, 20],
      },
    })

    expect(wrapper.get('[data-testid="quick-amount-10"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="quick-amount-20"]').attributes('aria-pressed')).toBe('false')

    await wrapper.get('[data-testid="quick-amount-20"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([20])
  })

  it('highlights the custom input when the amount is not a preset', async () => {
    const wrapper = mount(AmountInput, {
      props: {
        modelValue: 33,
        amounts: [10, 20],
      },
    })

    const input = wrapper.get('[data-testid="custom-amount-input"]')
    expect(input.classes()).toContain('border-primary-500')

    await input.setValue('12.5')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([12.5])
  })
})
