import { apiClient } from '../client'

export interface TelegramNotificationSettings {
  enabled: boolean
  chat_id: string
  bot_token?: string
  bot_token_configured: boolean
}

export async function getTelegramNotificationSettings(): Promise<TelegramNotificationSettings> {
  return (await apiClient.get<TelegramNotificationSettings>('/admin/settings/telegram-notification')).data
}

export async function updateTelegramNotificationSettings(settings: TelegramNotificationSettings): Promise<TelegramNotificationSettings> {
  return (await apiClient.put<TelegramNotificationSettings>('/admin/settings/telegram-notification', settings)).data
}
