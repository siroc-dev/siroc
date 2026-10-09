import { fireEvent, render, screen } from "@testing-library/react";
import { App as AntdApp } from "antd";
import { describe, expect, it, vi } from "vitest";
import { SiteSettings, type SiteSettingsSite } from "./SiteSettings";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(async () => ({
      ok: true,
      subject: "test.com",
      issuer: "Test CA",
      notBefore: "2026-01-01T00:00:00Z",
      notAfter: "2027-01-01T00:00:00Z",
      dnsNames: ["test.com", "www.test.com"],
      serial: "abc",
    })),
  },
}));

const site: SiteSettingsSite = {
  id: 1,
  username: "demo",
  domain: "test.com",
  docRoot: "/home/demo/domains/test.com/public_html",
  phpVersion: "8.3",
  aliases: ["www.test.com"],
  ssl: false,
  kind: "php",
  createdAt: "2025-07-09T16:13:54Z",
};

function renderSettings(section: "domain" | "php" | "maintenance" | "ssl" = "domain", extra?: Partial<SiteSettingsSite>) {
  const onPatch = vi.fn(async () => undefined);
  render(
    <AntdApp>
      <SiteSettings
        site={{ ...site, ...extra }}
        section={section}
        busy={false}
        phpVersions={[{ value: "8.3", label: "PHP 8.3" }]}
        publicName
        onClose={() => undefined}
        onPatch={onPatch}
        onIssueSSL={() => undefined}
        onLocalSSL={() => undefined}
        onDisableSSL={() => undefined}
        onGit={() => undefined}
        onLogs={() => undefined}
        onSSH={() => undefined}
        onLaravel={() => undefined}
      />
    </AntdApp>,
  );
  return { onPatch };
}

describe("SiteSettings", () => {
  it("opens on Domain Manager with the primary name locked", () => {
    renderSettings();
    expect(screen.getByText("Site modification [test.com] -- Time added [2025-07-09 16:13:54]")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Domain Manager" })).toBeTruthy();
    expect(screen.getByText("www.test.com")).toBeTruthy();
    expect(screen.getByText("Inoperable")).toBeTruthy();
  });

  it("shows certificate details on the SSL tab", async () => {
    renderSettings("ssl", { ssl: true, sslKind: "custom" });
    expect(await screen.findByText(/Issuer: Test CA/)).toBeTruthy();
    expect(screen.getByText(/Names: test.com, www.test.com/)).toBeTruthy();
    expect(screen.getByText("Custom certificate")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Install certificate" })).toBeTruthy();
  });

  it("switches to PHP and maintenance", () => {
    renderSettings();
    fireEvent.click(screen.getByRole("button", { name: "PHP version" }));
    expect(screen.getByRole("button", { name: "Save" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Maintenance Mode" }));
    expect(screen.getByText(/HTTP 503/)).toBeTruthy();
  });
});
