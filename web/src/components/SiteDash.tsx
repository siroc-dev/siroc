import type { ReactNode } from "react";
import {
  CloudDownloadOutlined,
  CodeOutlined,
  DatabaseOutlined,
  FileTextOutlined,
  FolderOutlined,
  GlobalOutlined,
  InfoCircleOutlined,
  LaptopOutlined,
  LockOutlined,
  SafetyOutlined,
  SettingOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons";
import { Breadcrumb, Button, Space, Switch, Tag, Typography } from "antd";

export type SiteDashSite = {
  id: number;
  username: string;
  domain: string;
  displayName?: string;
  docRoot: string;
  phpVersion: string;
  enabled: boolean;
  aliases: string[];
  ssl: boolean;
  sslKind?: string;
  sslExpiry?: string;
  kind?: string;
  appPort?: number;
  wafEnabled?: boolean;
  wafDisabledIds?: number[];
};

export type SiteDashAction =
  | "files"
  | "databases"
  | "backup"
  | "info"
  | "php"
  | "logs"
  | "ssh"
  | "git"
  | "stats"
  | "laravel"
  | "wordpress"
  | "app"
  | "ssl"
  | "edit"
  | "rename"
  | "toggle";

type Tile = {
  key: SiteDashAction;
  title: string;
  desc: string;
  icon: ReactNode;
  color: string;
  warn?: string;
};

const APP_KINDS = new Set(["nodejs", "python", "go", "rust", "docker"]);

function kindLabel(kind?: string, php?: string) {
  if (APP_KINDS.has(kind || "")) return kind || "App";
  if (kind === "proxy") return "Reverse proxy";
  if (kind === "nginx") return "Direct nginx";
  return php ? `PHP ${php}` : "PHP";
}

function sslLabel(s: SiteDashSite) {
  if (!s.ssl) return "HTTP only";
  if (s.sslKind === "letsencrypt") return "Let's Encrypt";
  if (s.sslKind === "custom") return "Custom certificate";
  return "Local HTTPS";
}

function TileButton({ tile, onOpen }: { tile: Tile; onOpen: (key: SiteDashAction) => void }) {
  return (
    <button type="button" className="site-dash-tile" onClick={() => onOpen(tile.key)}>
      <span className="site-dash-icon" style={{ background: tile.color }}>
        {tile.icon}
      </span>
      <span className="site-dash-copy">
        <strong>{tile.title}</strong>
        <em>{tile.warn || tile.desc}</em>
      </span>
    </button>
  );
}

export function SiteDash({
  site,
  admin,
  accountWaf,
  siteWafBusy,
  diskLabel,
  onBack,
  onOpen,
  onWaf,
  onRules,
}: {
  site: SiteDashSite;
  admin?: boolean;
  accountWaf: boolean;
  siteWafBusy?: boolean;
  diskLabel?: string;
  onBack: () => void;
  onOpen: (key: SiteDashAction) => void;
  onWaf: (enabled: boolean) => void;
  onRules?: () => void;
}) {
  const phpSite = !APP_KINDS.has(site.kind || "") && site.kind !== "proxy" && site.kind !== "nginx";
  const files: Tile[] = [
    { key: "files", title: "File manager", desc: "Browse and edit the site root", icon: <FolderOutlined />, color: "#52c41a" },
    { key: "databases", title: "Databases", desc: "MySQL / MariaDB for this account", icon: <DatabaseOutlined />, color: "#1677ff" },
    { key: "backup", title: "Backup", desc: "Backup and restore", icon: <CloudDownloadOutlined />, color: "#13c2c2" },
    { key: "info", title: "Connection info", desc: "Linux user, path, and PHP", icon: <InfoCircleOutlined />, color: "#722ed1" },
  ];
  const dev: Tile[] = [
    { key: "php", title: "PHP & hosting", desc: phpSite ? `PHP ${site.phpVersion}` : kindLabel(site.kind, site.phpVersion), icon: <CodeOutlined />, color: "#eb2f96" },
    { key: "logs", title: "Logs", desc: "Access and error logs", icon: <FileTextOutlined />, color: "#fa8c16" },
    { key: "ssh", title: "SSH terminal", desc: `Shell as ${site.username}`, icon: <LaptopOutlined />, color: "#262626" },
    { key: "git", title: "Git", desc: "Force-pull deploy and webhook", icon: <ThunderboltOutlined />, color: "#f5222d" },
    { key: "stats", title: "Statistics", desc: "Geo, ASN, AI crawlers, time", icon: <GlobalOutlined />, color: "#2f54eb" },
  ];
  if (phpSite) {
    dev.push(
      { key: "laravel", title: "Laravel", desc: "Composer, queues, artisan", icon: <CodeOutlined />, color: "#ff4d4f" },
      { key: "wordpress", title: "WordPress", desc: "Install and manage WP", icon: <GlobalOutlined />, color: "#1677ff" },
    );
  }
  if (APP_KINDS.has(site.kind || "")) {
    dev.push({ key: "app", title: "App process", desc: site.appPort ? `Port ${site.appPort}` : "Start and logs", icon: <ThunderboltOutlined />, color: "#13c2c2" });
  }
  const security: Tile[] = [
    {
      key: "ssl",
      title: "SSL / TLS",
      desc: sslLabel(site),
      icon: <LockOutlined />,
      color: site.ssl ? "#52c41a" : "#faad14",
      warn: site.ssl ? undefined : "Domain not secured",
    },
    { key: "edit", title: "Site settings", desc: "Domain, directory, SSL, rewrite", icon: <SettingOutlined />, color: "#595959" },
  ];

  return (
    <div className="site-dash">
      <Breadcrumb
        items={[
          { title: <a onClick={onBack}>Websites & domains</a> },
          { title: site.displayName || site.domain },
        ]}
      />
      <div className="site-dash-hero">
        <div>
          <Typography.Title level={3} style={{ margin: "8px 0 4px" }}>
            {site.displayName || site.domain}
          </Typography.Title>
          {site.displayName && site.displayName !== site.domain ? (
            <Typography.Text type="secondary">{site.domain}</Typography.Text>
          ) : null}
          <Space wrap size={8}>
            <Tag color={site.enabled ? "success" : "default"}>{site.enabled ? "Active" : "Disabled"}</Tag>
            <Tag color={site.ssl ? "success" : "warning"}>{sslLabel(site)}</Tag>
            <Tag>{kindLabel(site.kind, site.phpVersion)}</Tag>
            {admin ? <Tag>{site.username}</Tag> : null}
          </Space>
          {site.aliases?.length ? (
            <div style={{ marginTop: 8 }}>
              <Typography.Text type="secondary">
                Aliases: {site.aliases.slice(0, 8).join(", ")}
                {site.aliases.length > 8 ? ` +${site.aliases.length - 8} more` : ""}
              </Typography.Text>
            </div>
          ) : null}
        </div>
        <div className="site-dash-side">
          {diskLabel ? (
            <div>
              <Typography.Text type="secondary">Account disk</Typography.Text>
              <div className="site-dash-metric">{diskLabel}</div>
            </div>
          ) : null}
          <Space wrap>
            <Button onClick={() => onOpen("rename")}>Rename</Button>
            <Button onClick={() => onOpen("toggle")}>{site.enabled ? "Disable" : "Enable"}</Button>
          </Space>
        </div>
      </div>

      <section>
        <Typography.Title level={5}>Files & databases</Typography.Title>
        <div className="site-dash-grid">
          {files.map((t) => (
            <TileButton key={t.key} tile={t} onOpen={onOpen} />
          ))}
        </div>
      </section>

      <section>
        <Typography.Title level={5}>Dev tools</Typography.Title>
        <div className="site-dash-grid">
          {dev.map((t) => (
            <TileButton key={t.key} tile={t} onOpen={onOpen} />
          ))}
        </div>
      </section>

      <section>
        <Typography.Title level={5}>Security</Typography.Title>
        <div className="site-dash-grid">
          {security.map((t) => (
            <TileButton key={t.key} tile={t} onOpen={onOpen} />
          ))}
          <div className="site-dash-tile site-dash-tile-static">
            <span className="site-dash-icon" style={{ background: accountWaf && site.wafEnabled !== false ? "#52c41a" : "#8c8c8c" }}>
              <SafetyOutlined />
            </span>
            <span className="site-dash-copy">
              <strong>Web application firewall</strong>
              <em>{accountWaf ? (site.wafEnabled !== false ? "ModSecurity on" : "Off for this site") : "Off for this account"}</em>
            </span>
            <Switch
              checked={accountWaf && site.wafEnabled !== false}
              disabled={!accountWaf || siteWafBusy}
              loading={siteWafBusy}
              onChange={onWaf}
            />
          </div>
        </div>
        {onRules ? (
          <Button style={{ marginTop: 12 }} onClick={onRules}>
            Rule exceptions{site.wafDisabledIds?.length ? ` (${site.wafDisabledIds.length} off)` : ""}
          </Button>
        ) : null}
      </section>
    </div>
  );
}
