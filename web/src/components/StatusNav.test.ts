/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { fmtMs, fmtUs } from "./StatusNav";

describe("status formatters", () => {
  it("formats redis durations", () => {
    expect(fmtUs(250)).toBe("250 µs");
    expect(fmtUs(15000)).toBe("15.0 ms");
    expect(fmtUs(2_500_000)).toBe("2.50 s");
  });

  it("formats sql durations", () => {
    expect(fmtMs(2.5)).toBe("2.50 ms");
    expect(fmtMs(12.5)).toBe("12.5 ms");
    expect(fmtMs(2500)).toBe("2.50 s");
  });
});
