export interface FirstOutputProtectionForm {
  limitSeconds: number | ''
  cooldownMinutes: number | ''
  windowMinutes: number | ''
}

export const firstOutputFields = [
  { key: 'limitSeconds', credential: 'temp_unschedulable_first_output_limit_seconds', defaultValue: 30, max: 3600, label: 'firstOutputLimitInput' },
  { key: 'cooldownMinutes', credential: 'temp_unschedulable_first_output_cooldown_minutes', defaultValue: 5, max: 1440, label: 'firstOutputCooldownInput' },
  { key: 'windowMinutes', credential: 'temp_unschedulable_first_output_window_minutes', defaultValue: 10, max: 1440, label: 'firstOutputWindowInput' }
] as const

export function isFirstOutputValueValid(value: unknown, max: number): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 1 && value <= max
}

export function readFirstOutputProtectionConfig(credentials?: Record<string, unknown>): FirstOutputProtectionForm {
  const config: FirstOutputProtectionForm = { limitSeconds: 30, cooldownMinutes: 5, windowMinutes: 10 }
  for (const field of firstOutputFields) {
    const value = credentials?.[field.credential]
    config[field.key] = isFirstOutputValueValid(value, field.max) ? value : field.defaultValue
  }
  return config
}

export function writeFirstOutputProtectionConfig(credentials: Record<string, unknown>, enabled: boolean, config: FirstOutputProtectionForm): boolean {
  if (enabled && !firstOutputFields.every(field => isFirstOutputValueValid(config[field.key], field.max))) return false
  credentials.temp_unschedulable_first_output_enabled = enabled
  if (enabled) {
    for (const field of firstOutputFields) credentials[field.credential] = config[field.key]
  }
  return true
}

export function clearFirstOutputProtectionConfig(credentials: Record<string, unknown>) {
  delete credentials.temp_unschedulable_first_output_enabled
  for (const field of firstOutputFields) delete credentials[field.credential]
}
