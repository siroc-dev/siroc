/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { colorPreset, resolveDark } from "./theme";

describe("theme", () => {
  it("resolves auto from system preference", () => {
    expect(resolveDark("auto", true)).toBe(true);
    expect(resolveDark("auto", false)).toBe(false);
    expect(resolveDark("light", true)).toBe(false);
    expect(resolveDark("dark", false)).toBe(true);
  });

  it("uses custom hex and known presets", () => {
    expect(colorPreset("mint").primary).toBe("#10b981");
    expect(colorPreset("custom", "#abc123").primary).toBe("#abc123");
    expect(colorPreset("custom", "nope").primary).toBe("#4f46e5");
  });
});
