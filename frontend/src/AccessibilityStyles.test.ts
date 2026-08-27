import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

const styles = readFileSync(`${process.cwd()}/src/styles.css`, "utf8");

function hex(name: string) {
  const value = styles.match(new RegExp(`${name}:\\s*(#[0-9a-fA-F]{6})`))?.[1];
  if (!value) throw new Error(`missing color token ${name}`);
  return value;
}

function luminance(value: string) {
  const channels = [1, 3, 5].map((start) => Number.parseInt(value.slice(start, start + 2), 16) / 255).map((channel) => (
    channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  ));
  return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
}

function contrast(foreground: string, background: string) {
  const a = luminance(foreground);
  const b = luminance(background);
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

describe("global button contrast", () => {
  it("keeps light and disabled buttons above 4.5:1", () => {
    expect(contrast(hex("--button-light-text"), hex("--button-light-bg"))).toBeGreaterThanOrEqual(4.5);
    expect(contrast(hex("--button-disabled-text"), hex("--button-disabled-bg"))).toBeGreaterThanOrEqual(4.5);
  });

  it("explicitly fixes the welcome-panel surface button and opacity-only disabled states", () => {
    expect(styles).toContain(".memory-welcome .button.surface");
    expect(styles).toMatch(/button:disabled[^}]*opacity:1/);
    expect(styles).toContain(".plugin-toggle:disabled");
    expect(styles).toContain(".strategy-node-wrap>button.disabled");
    expect(styles).toMatch(/\.button\s*\{[^}]*min-height:\s*44px/);
  });
});
