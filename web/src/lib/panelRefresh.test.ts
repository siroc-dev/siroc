import { describe, expect, it } from "vitest";
import { shouldReloadForVersion } from "./panelRefresh";

describe("panel refresh after update", () => {
  it("waits until the running version changes and the agent answers", () => {
    expect(shouldReloadForVersion("0.2.75", "0.2.75", false, true)).toBe(false);
    expect(shouldReloadForVersion("0.2.75", "0.2.76", false, false)).toBe(false);
    expect(shouldReloadForVersion("0.2.75", "0.2.76", false, true)).toBe(true);
  });

  it("refreshes after the panel was down and the agent is back", () => {
    expect(shouldReloadForVersion("0.2.75", "0.2.75", true, false)).toBe(false);
    expect(shouldReloadForVersion("0.2.75", "0.2.75", true, true)).toBe(true);
    expect(shouldReloadForVersion("0.2.75", "", true, true)).toBe(false);
  });
});
