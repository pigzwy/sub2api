export const voicePricingPlatforms = new Set(["grok", "composite"]);

/**
 * Native Grok and composite groups can carry Grok-routed voice pricing.
 * This is deliberately separate from the Realtime access switch: the switch
 * controls admission, while these fields describe the group's rate card.
 */
export const supportsVoicePricingPlatform = (platform: string): boolean =>
  voicePricingPlatforms.has(platform);
