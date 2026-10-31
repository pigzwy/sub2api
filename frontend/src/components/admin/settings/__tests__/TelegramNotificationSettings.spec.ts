import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TelegramNotificationSettings from '../TelegramNotificationSettings.vue'
import { getTelegramNotificationSettings, updateTelegramNotificationSettings } from '@/api/admin/telegramNotification'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

vi.mock('@/api/admin/telegramNotification', () => ({
  getTelegramNotificationSettings: vi.fn(),
  updateTelegramNotificationSettings: vi.fn(),
}))

beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(getTelegramNotificationSettings).mockResolvedValue({ enabled: true, chat_id: '-123', bot_token_configured: true })
  vi.mocked(updateTelegramNotificationSettings).mockResolvedValue({ enabled: true, chat_id: '-456', bot_token_configured: true })
})

describe('Telegram notification settings', () => {
  it('keeps the saved token hidden and preserves it when saving an empty token', async () => {
    const wrapper = mount(TelegramNotificationSettings)
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('[data-testid="telegram-token"]').element.value).toBe('')
    await wrapper.get('[data-testid="telegram-chat-id"]').setValue('-456')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(updateTelegramNotificationSettings).toHaveBeenCalledWith({ enabled: true, chat_id: '-456', bot_token: '', bot_token_configured: true })
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('requires credentials for first-time enablement and clears a newly saved token', async () => {
    vi.mocked(getTelegramNotificationSettings).mockResolvedValue({ enabled: false, chat_id: '', bot_token_configured: false })
    const wrapper = mount(TelegramNotificationSettings)
    await flushPromises()
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('button').trigger('click')
    expect(updateTelegramNotificationSettings).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.get('[data-testid="telegram-token"]').setValue('123:secret')
    await wrapper.get('[data-testid="telegram-chat-id"]').setValue('-456')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('[data-testid="telegram-token"]').element.value).toBe('')
    wrapper.unmount()
  })

  it('blocks saving after a load error and supports retry', async () => {
    vi.mocked(getTelegramNotificationSettings).mockRejectedValueOnce(new Error('load failed'))
    const wrapper = mount(TelegramNotificationSettings)
    await flushPromises()
    expect(wrapper.find('[data-testid="telegram-token"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('load failed')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="telegram-token"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
