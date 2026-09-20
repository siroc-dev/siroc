import { render, screen, waitFor } from "@testing-library/react";
import { App as AntdApp } from "antd";
import { MemoryRouter, Outlet, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Sites } from "./Sites";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
    send: vi.fn(),
  },
}));

import { api } from "@/lib/api";

function renderSites(admin = true, user = "admin") {
  return render(
    <AntdApp>
      <MemoryRouter initialEntries={["/sites"]}>
        <Routes>
          <Route element={<Outlet context={{ user, admin }} />}>
            <Route path="/sites" element={<Sites />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </AntdApp>,
  );
}

describe("Sites page", () => {
  beforeEach(() => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url.includes("/api/ssl/letsencrypt")) {
        return { email: "", server: "production", keyType: "ecdsa" };
      }
      return null;
    });
  });

  it("renders when list APIs return null instead of arrays", async () => {
    renderSites();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Websites" })).toBeTruthy();
    });
    expect(screen.getByRole("button", { name: "New site" })).toBeTruthy();
    expect(screen.getByText("No websites yet. Install nginx, Apache, and PHP first.")).toBeTruthy();
  });

  it("lets a hosting user toggle account and per-site WAF", async () => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url === "/api/accounts") return [{ username: "a01", wafEnabled: true }];
      if (url === "/api/sites") {
        return [
          { id: 1, username: "a01", domain: "a01.test", docRoot: "/home/a01/public_html", phpVersion: "8.4", enabled: true, aliases: [], ssl: true, wafEnabled: true },
        ];
      }
      return [];
    });
    vi.mocked(api.put).mockResolvedValue({ ok: true });
    renderSites(false, "a01");
    await waitFor(() => {
      expect(screen.getByText("Protect my websites")).toBeTruthy();
    });
    expect(screen.getByText("a01.test")).toBeTruthy();
    const switches = screen.getAllByRole("switch");
    expect(switches.length).toBeGreaterThanOrEqual(2);
    switches[0].click();
    await waitFor(() => {
      expect(api.put).toHaveBeenCalledWith("/api/accounts/a01/waf", { enabled: false });
    });
  });
});
