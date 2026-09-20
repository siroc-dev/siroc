import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_PAGE_SIZE, filterEntries, pageEntries, SiteLogs } from "./SiteLogs";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(),
  },
}));

import { api } from "@/lib/api";

const sample = {
  ok: true,
  domain: "a01.test",
  files: [
    { id: "nginx-access", label: "Nginx access", group: "nginx", kind: "access", exists: true, size: 42 },
    { id: "laravel:laravel-2026-09-18.log", label: "Laravel · laravel-2026-09-18.log", group: "laravel", kind: "app", exists: true, size: 80 },
  ],
  current: { id: "laravel:laravel-2026-09-18.log", label: "Laravel · laravel-2026-09-18.log", group: "laravel", kind: "app", exists: true, size: 80 },
  entries: [
    { time: "2026-09-18 13:47:01", env: "local", level: "info", message: "Job processed" },
    { time: "2026-09-18 13:46:56", env: "local", level: "error", message: "SQLSTATE Access denied", context: "#0 Connection.php" },
  ],
  counts: { info: 1, error: 1 },
  content: "[2026-09-18 13:46:56] local.ERROR: SQLSTATE Access denied\n",
};

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
    const rows = filterEntries(sample.entries, "sqlstate", ["error"]);
    expect(rows).toHaveLength(1);
    expect(rows[0].level).toBe("error");
    expect(filterEntries(sample.entries, "missing", []).length).toBe(0);
  });
});

describe("SiteLogs", () => {
  beforeEach(() => {
    vi.mocked(api.get).mockResolvedValue(sample);
  });

  it("renders parsed Laravel entries like a log viewer", async () => {
    render(<SiteLogs siteId={1} domain="a01.test" />);
    await waitFor(() => {
      expect(screen.getByText("SQLSTATE Access denied")).toBeTruthy();
    });
    expect(api.get).toHaveBeenCalledWith("/api/sites/1/logs");
    expect(screen.getByText("Job processed")).toBeTruthy();
    expect(screen.getByText("laravel-2026-09-18.log")).toBeTruthy();
    expect(screen.getByText(/ERROR/)).toBeTruthy();
  });

  it("paginates parsed entries 10 at a time", async () => {
    const entries = Array.from({ length: 15 }, (_, i) => ({
      time: `2026-09-20 00:00:${String(i).padStart(2, "0")}`,
      level: "info" as const,
      message: `Entry ${i + 1}`,
    }));
    vi.mocked(api.get).mockResolvedValue({
      ...sample,
      entries,
      counts: { info: 15 },
    });
    render(<SiteLogs siteId={1} domain="a01.test" />);
    await waitFor(() => {
      expect(screen.getByText("Entry 1")).toBeTruthy();
    });
    expect(screen.getByText("Entry 10")).toBeTruthy();
    expect(screen.queryByText("Entry 11")).toBeNull();
    expect(screen.getByText("1-10 of 15")).toBeTruthy();
  });
});
