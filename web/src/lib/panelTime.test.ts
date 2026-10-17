import { afterEach, describe, expect, it } from "vitest";
import { formatPanelTime, setPanelTimezone } from "./panelTime";

describe("formatPanelTime", () => {
  afterEach(() => setPanelTimezone(""));

  it("shows Last deploy in the server timezone", () => {
    setPanelTimezone("Asia/Bangkok");
    expect(formatPanelTime("2026-10-17T22:51:36Z")).toBe("2026-10-18 05:51:36");
  });

  it("keeps an empty value blank", () => {
    expect(formatPanelTime("")).toBe("");
    expect(formatPanelTime("0001-01-01T00:00:00Z")).toBe("");
  });
});
