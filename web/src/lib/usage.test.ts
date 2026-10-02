/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { formatRate } from "./usage";

describe("formatRate", () => {
  it("keeps small byte rates precise", () => {
    expect(formatRate(532.48)).toBe("532.48 B");
    expect(formatRate(1690)).toBe("1.65 KB");
  });
});
