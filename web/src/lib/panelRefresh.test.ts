import { describe, expect, it } from "vitest";
import { shouldReloadForVersion } from "./panelRefresh";

describe("panel refresh after update", () => {
  it("waits until the running version changes", () => {
    expect(shouldReloadForVersion("0.2.75", "0.2.75", false)).toBe(false);
    expect(shouldReloadForVersion("0.2.75", "0.2.76", false)).toBe(true);
  });

  it("refreshes after the panel was down and is back", () => {
    expect(shouldReloadForVersion("0.2.75", "0.2.75", true)).toBe(true);
    expect(shouldReloadForVersion("0.2.75", "", true)).toBe(false);
  });
});
