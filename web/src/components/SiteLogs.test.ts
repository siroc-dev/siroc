/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { DEFAULT_PAGE_SIZE, filterEntries, linesToEntries, pageEntries } from "./SiteLogs.parse";

const sample = [
  { time: "2026-09-18 13:47:01", env: "local", level: "info", message: "Job processed" },
  { time: "2026-09-18 13:46:56", env: "local", level: "error", message: "SQLSTATE Access denied", context: "#0 Connection.php" },
];

describe("pageEntries", () => {
  it("pages from 20 by default", () => {
    const rows = Array.from({ length: 25 }, (_, i) => i + 1);
    expect(DEFAULT_PAGE_SIZE).toBe(20);
    expect(pageEntries(rows, 1)).toEqual(rows.slice(0, 20));
    expect(pageEntries(rows, 2)).toEqual([21, 22, 23, 24, 25]);
  });
});

describe("linesToEntries", () => {
  it("returns newest first", () => {
    const rows = linesToEntries("2026-10-02 00:04:00 older fail\n2026-10-02 00:05:00 newer ok\n");
    expect(rows[0].time).toBe("2026-10-02 00:05:00");
    expect(rows[0].level).toBe("info");
    expect(rows[1].level).toBe("error");
  });
});

describe("filterEntries", () => {
  it("filters by level and search", () => {
    const rows = filterEntries(sample, "sqlstate", ["error"]);
    expect(rows).toHaveLength(1);
    expect(rows[0].level).toBe("error");
    expect(filterEntries(sample, "missing", []).length).toBe(0);
  });
});
