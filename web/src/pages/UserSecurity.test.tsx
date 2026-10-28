import { render, screen, waitFor } from "@testing-library/react";
import { App as AntdApp } from "antd";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { UserSecurity } from "./UserSecurity";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

import { api } from "@/lib/api";

describe("User security", () => {
  beforeEach(() => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url === "/api/accounts") return [{ username: "a01", wafEnabled: true }];
      if (url === "/api/sites") {
        return [{ id: 1, username: "a01", domain: "doujinzo.com", enabled: true, wafEnabled: true }];
      }
      if (url.includes("/waf/rules")) return { rules: [], total: 0, disabledIds: [] };
      return [];
    });
    vi.mocked(api.put).mockResolvedValue({ ok: true });
  });

  it("shows the account WAF controls", async () => {
    const { unmount } = render(
      <AntdApp>
        <UserSecurity />
      </AntdApp>,
    );
    await waitFor(() => {
      expect(screen.getByText("Protect my websites")).toBeTruthy();
    });
    expect(screen.getByText("Rule exceptions")).toBeTruthy();
    expect(screen.getByText("On for this account")).toBeTruthy();
    screen.getAllByRole("switch")[0].click();
    await waitFor(() => {
      expect(api.put).toHaveBeenCalledWith("/api/accounts/a01/waf", { enabled: false });
    });
    unmount();
  });
});
