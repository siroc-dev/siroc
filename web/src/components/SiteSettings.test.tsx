import { fireEvent, render, screen } from "@testing-library/react";
import { App as AntdApp } from "antd";
import { describe, expect, it, vi } from "vitest";
import { SiteSettings, type SiteSection, type SiteSettingsSite } from "./SiteSettings";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(async (url: string) => {
      if (String(url).includes("/config")) {
        return { ok: true, nginx: "server { listen 80; }\n", apache: "", custom: false };
      }
      if (String(url).includes("/php")) {
        return { settings: { memoryLimit: "256M", maxExecutionTime: 60, timezone: "UTC" } };
      }
      return {
        ok: true,
        subject: "test.com",
        issuer: "Test CA",
        notBefore: "2026-01-01T00:00:00Z",
        notAfter: "2027-01-01T00:00:00Z",
        dnsNames: ["test.com", "www.test.com"],
        serial: "abc",
      };
    }),
    post: vi.fn(async (url: string) => {
      if (String(url).includes("/app")) {
        return {
          ok: true,
          composerRoot: "/home/demo/domains/test.com",
          laravel: { hasComposer: true, composerPkgs: [{ name: "laravel/framework", constraint: "^11.0" }] },
        };
      }
      return {};
    }),
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

function renderSettings(section: SiteSection = "domain", extra?: Partial<SiteSettingsSite>) {
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
  it("pages the domain list after twenty names", () => {
    const aliases = Array.from({ length: 25 }, (_, i) => `a${i}.test.com`);
    renderSettings("domain", { aliases });
    expect(screen.getByText("a0.test.com")).toBeTruthy();
    expect(screen.queryByText("a24.test.com")).toBeNull();
    expect(screen.getByText("Total 26")).toBeTruthy();
  });

  it("opens on Domain Manager with the primary name locked", () => {
    renderSettings();
    expect(screen.getByRole("button", { name: "Back" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Domains" })).toBeTruthy();
    expect(screen.getByText("Added 2025-07-09 16:13:54")).toBeTruthy();
    expect(screen.getByText("www.test.com")).toBeTruthy();
    expect(screen.getByText("Primary")).toBeTruthy();
    expect(screen.queryByText(/Site modification/)).toBeNull();
  });

  it("loads the nginx file with save ready", async () => {
    renderSettings("config");
    expect(await screen.findByDisplayValue(/listen 80/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Test" })).toBeNull();
    expect(screen.getByRole("button", { name: "Save" })).toHaveProperty("disabled", false);
  });

  it("shows the deployed certificate without a business-certificate tab", async () => {
    renderSettings("ssl", { ssl: true, sslKind: "custom" });
    expect(await screen.findByRole("tab", { name: "Certificate" })).toBeTruthy();
    expect(await screen.findByText(/Other certificate/)).toBeTruthy();
    expect(screen.getByText(/Test CA/)).toBeTruthy();
    expect(screen.getByText(/test.com, www.test.com/)).toBeTruthy();
    expect(screen.getByText("Redirect HTTP to HTTPS")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Disable SSL" })).toBeTruthy();
    expect(screen.queryByRole("tab", { name: "Business certificate" })).toBeNull();
    expect(screen.queryByRole("tab", { name: "Current Certs - [Deployed]" })).toBeNull();
    expect(screen.getByRole("tab", { name: "Let's Encrypt" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Paste a certificate" })).toBeTruthy();
  });

  it("switches to PHP and maintenance", () => {
    renderSettings();
    fireEvent.click(screen.getByRole("tab", { name: "PHP" }));
    expect(screen.getByText("memory_limit")).toBeTruthy();
    expect(screen.getByText("upload_max_filesize")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Save" })).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Maintenance" }));
    expect(screen.getByText(/HTTP 503/)).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Hotlink" }));
    expect(screen.getByText("www.test.com")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add" })).toBeTruthy();
  });

  it("shows composer packages and the composer.json directory", async () => {
    renderSettings("composer");
    expect(screen.getByText("composer.json directory")).toBeTruthy();
    expect(await screen.findByText("laravel/framework")).toBeTruthy();
    expect(screen.getByPlaceholderText("/home/demo/domains/test.com")).toBeTruthy();
  });
});
