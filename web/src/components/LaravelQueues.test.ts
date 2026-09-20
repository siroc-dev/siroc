import { describe, expect, it } from "vitest";
import { fmtWhen } from "./LaravelQueues";

describe("fmtWhen", () => {
  it("formats listening and empty values", () => {
    expect(fmtWhen()).toBe("—");
    expect(fmtWhen("listening")).toBe("Listening");
    expect(fmtWhen("waiting")).toBe("Waiting");
    expect(fmtWhen("11 hours from now")).toBe("11 hours from now");
  });

  it("formats RFC3339 timestamps", () => {
    const got = fmtWhen("2026-09-19T12:00:00Z");
    expect(got).not.toBe("2026-09-19T12:00:00Z");
    expect(got).not.toBe("—");
  });
});
