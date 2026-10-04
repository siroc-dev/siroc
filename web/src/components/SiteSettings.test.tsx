import { fireEvent, render, screen } from "@testing-library/react";
import { App as AntdApp } from "antd";
import { describe, expect, it, vi } from "vitest";
import { SiteSettings, type SiteSettingsSite } from "./SiteSettings";

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

function renderSettings(section: "domain" | "php" | "maintenance" = "domain") {
  const onPatch = vi.fn(async () => undefined);
  render(
    <AntdApp>
      <SiteSettings
        site={site}
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

  it("switches to PHP and maintenance", () => {
    renderSettings();
    fireEvent.click(screen.getByRole("button", { name: "PHP version" }));
    expect(screen.getByRole("button", { name: "Save" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Maintenance Mode" }));
    expect(screen.getByText(/HTTP 503/)).toBeTruthy();
  });
});
