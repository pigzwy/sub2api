import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

import {
  supportsVoicePricingPlatform,
  voicePricingPlatforms,
} from "../groupsVoicePricing";

const groupsViewSource = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), "../GroupsView.vue"),
  "utf8",
);

describe("groups voice pricing platform support", () => {
  it("allows native Grok and composite groups", () => {
    expect(supportsVoicePricingPlatform("grok")).toBe(true);
    expect(supportsVoicePricingPlatform("composite")).toBe(true);
    expect(voicePricingPlatforms).toEqual(new Set(["grok", "composite"]));
  });

  it("does not expose group voice pricing on unrelated platforms", () => {
    for (const platform of [
      "openai",
      "anthropic",
      "gemini",
      "antigravity",
      "kimi",
      "zhipu",
      "deepseek",
    ]) {
      expect(supportsVoicePricingPlatform(platform)).toBe(false);
    }
  });

  it("wires the predicate into both group forms and keeps search Grok-only", () => {
    expect(groupsViewSource).toContain(
      'v-if="supportsVoicePricingPlatform(createForm.platform)"',
    );
    expect(groupsViewSource).toContain(
      'v-if="supportsVoicePricingPlatform(editForm.platform)"',
    );
    expect(groupsViewSource).toContain('data-testid="create-audio-realtime-price"');
    expect(groupsViewSource).toContain('data-testid="create-audio-tts-price"');
    expect(groupsViewSource).toContain('data-testid="create-audio-stt-price"');
    expect(groupsViewSource).toContain('data-testid="edit-audio-realtime-price"');
    expect(groupsViewSource).toContain('data-testid="edit-audio-tts-price"');
    expect(groupsViewSource).toContain('data-testid="edit-audio-stt-price"');
    expect(groupsViewSource).toContain(
      'v-if="createForm.platform === \'grok\'"',
    );
    expect(groupsViewSource).toContain(
      'v-if="editForm.platform === \'grok\'"',
    );
    expect(groupsViewSource).toContain(
      'if (createForm.platform !== "grok")',
    );
    expect(groupsViewSource).toContain(
      'if (editForm.platform === "grok")',
    );
    expect(groupsViewSource).toContain(
      "payload.search_price_per_1k = -1",
    );
  });
});
