/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { DEFAULT_PAGE_SIZE, filterEntries, pageEntries } from "./SiteLogs.parse";

const sample = [
  { time: "2026-09-18 13:47:01", env: "local", level: "info", message: "Job processed" },
  { time: "2026-09-18 13:46:56", env: "local", level: "error", message: "SQLSTATE Access denied", context: "#0 Connection.php" },
];

describe("pageEntries", () => {
  it("pages from 10 by default", () => {
    const rows = Array.from({ length: 15 }, (_, i) => i + 1);
    expect(DEFAULT_PAGE_SIZE).toBe(10);
    expect(pageEntries(rows, 1)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
    expect(pageEntries(rows, 2)).toEqual([11, 12, 13, 14, 15]);
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
