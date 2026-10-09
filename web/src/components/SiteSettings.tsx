import { useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Input, Modal, Select, Space, Switch, Table, Typography } from "antd";
import { matchNginxRewrite, nginxRewriteBody, NGINX_REWRITE_OPTIONS } from "@/lib/nginxRewrites";
import { ProxySettings, type ProxySettingsValue } from "@/components/ProxySettings";
import { api } from "@/lib/api";

export type SiteOptions = {
  index?: string[];
  access?: "" | "allow" | "deny";
  ips?: string[];
  hotlink?: boolean;
  maintenance?: boolean;
  redirects?: { from: string; to: string; code: number }[];
  proxy?: ProxySettingsValue;
  static?: boolean;
  staticExt?: string;
};

export type SiteSettingsSite = {
  id: number;
  username: string;
  domain: string;
  displayName?: string;
  displayAliases?: string[];
  docRoot: string;
  phpVersion: string;
  aliases: string[];
  ssl: boolean;
  sslKind?: string;
  sslExpiry?: string;
  rewrite?: string;
  kind?: string;
  proxyPass?: string;
  appPort?: number;
  appCmd?: string;
  createdAt?: string;
  options?: SiteOptions;
};

export type CertInfo = {
  ok?: boolean;
  kind?: string;
  subject?: string;
  issuer?: string;
  notBefore?: string;
  notAfter?: string;
  dnsNames?: string[];
  serial?: string;
  message?: string;
};

function certDate(v?: string) {
  if (!v) return "—";
  return v.replace("T", " ").replace(/\.\d+/, "").replace("Z", " UTC");
}

function sslKindLabel(site: SiteSettingsSite) {
  if (!site.ssl) return "HTTP only";
  if (site.sslKind === "letsencrypt") return "Let's Encrypt";
  if (site.sslKind === "custom") return "Custom certificate";
  return "Local HTTPS";
}

export type SiteSection =
  | "domain"
  | "directory"
  | "access"
  | "rewrite"
  | "index"
  | "config"
  | "ssl"
  | "php"
  | "server"
  | "git"
  | "composer"
  | "redirect"
  | "proxy"
  | "hotlink"
  | "maintenance"
  | "logs";

const NAV: { key: SiteSection; label: string }[] = [
  { key: "domain", label: "Domain Manager" },
  { key: "directory", label: "Directory" },
  { key: "access", label: "Limit access" },
  { key: "rewrite", label: "URL rewrite" },
  { key: "index", label: "Default document" },
  { key: "config", label: "Config" },
  { key: "ssl", label: "SSL" },
  { key: "php", label: "PHP version" },
  { key: "server", label: "Web Server" },
  { key: "git", label: "Git Manager" },
  { key: "composer", label: "Composer" },
  { key: "redirect", label: "Redirect" },
  { key: "proxy", label: "Reverse proxy" },
  { key: "hotlink", label: "Hotlink Protection" },
  { key: "maintenance", label: "Maintenance Mode" },
  { key: "logs", label: "Response log" },
];

const KINDS = [
  { value: "php", label: "PHP / Apache" },
  { value: "nginx", label: "Direct nginx" },
  { value: "proxy", label: "Reverse proxy" },
  { value: "nodejs", label: "Node.js" },
  { value: "python", label: "Python" },
  { value: "go", label: "Go" },
  { value: "rust", label: "Rust" },
  { value: "docker", label: "Docker" },
];

const APP = new Set(["nodejs", "python", "go", "rust", "docker"]);

const DEFAULT_STATIC_EXT =
  "ac3 avi bmp bz2 css cue dat doc docx dts eot exe flv gif gz htm html ico img iso jpeg jpg js mkv mp3 mp4 mpeg mpg ogg pdf png ppt pptx qt rar rm svg swf tar tgz ttf txt wav webp woff woff2 xls xlsx zip";

function homePrefix(user: string) {
  return user ? `/home/${user}/` : "/home/";
}

function toRel(user: string, abs: string) {
  const p = homePrefix(user);
  if (abs.startsWith(p)) return abs.slice(p.length);
  if (abs.startsWith("/")) return abs;
  return abs;
}

function addedAt(raw?: string) {
  if (!raw || raw.startsWith("0001")) return "";
  return raw.replace("T", " ").replace(/\.\d+Z?$/, "").replace(/Z$/, "");
}

function baseOpts(site: SiteSettingsSite): SiteOptions {
  const o = site.options || {};
  return {
    index: o.index || [],
    access: o.access || "",
    ips: o.ips || [],
    hotlink: !!o.hotlink,
    maintenance: !!o.maintenance,
    redirects: o.redirects || [],
    proxy: o.proxy,
    static: o.static,
    staticExt: o.staticExt,
  };
}

function defaultCmd(kind: string) {
  switch (kind) {
    case "nodejs":
      return "node server.js";
    case "python":
      return "python3 app.py";
    case "go":
      return "./app";
    case "rust":
      return "./app";
    case "docker":
      return "docker compose up";
    default:
      return "";
  }
}

export function SiteSettings({
  site,
  section,
  busy,
  phpVersions,
  publicName,
  onClose,
  onPatch,
  onIssueSSL,
  sslLog,
  onLocalSSL,
  onDisableSSL,
  onCustomSSL,
  onGit,
  onLogs,
  onSSH,
  onLaravel,
}: {
  site: SiteSettingsSite;
  section: SiteSection;
  busy: boolean;
  phpVersions: { value: string; label: string }[];
  publicName: boolean;
  onClose: () => void;
  onPatch: (body: Record<string, unknown>, ok: string) => Promise<void>;
  onIssueSSL: () => void;
  sslLog?: string;
  onLocalSSL: () => void;
  onDisableSSL: () => void;
  onCustomSSL?: (cert: string, key: string) => Promise<void>;
  onGit: () => void;
  onLogs: () => void;
  onSSH: () => void;
  onLaravel: () => void;
}) {
  const [tab, setTab] = useState<SiteSection>(section);
  const [draft, setDraft] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const [docRoot, setDocRoot] = useState("");
  const [rewrite, setRewrite] = useState("");
  const [php, setPhp] = useState("");
  const [sslInfo, setSslInfo] = useState<CertInfo | null>(null);
  const [certPEM, setCertPEM] = useState("");
  const [keyPEM, setKeyPEM] = useState("");
  const [kind, setKind] = useState("php");
  const [proxyPass, setProxyPass] = useState("");
  const [appCmd, setAppCmd] = useState("");
  const [appPort, setAppPort] = useState("");
  const [access, setAccess] = useState<"" | "allow" | "deny">("");
  const [ips, setIps] = useState("");
  const [index, setIndex] = useState("");
  const [hotlink, setHotlink] = useState(false);
  const [maintenance, setMaintenance] = useState(false);
  const [staticOn, setStaticOn] = useState(true);
  const [staticExt, setStaticExt] = useState(DEFAULT_STATIC_EXT);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [code, setCode] = useState(301);
  const [redirects, setRedirects] = useState<{ from: string; to: string; code: number }[]>([]);
  const { message } = App.useApp();

  useEffect(() => {
    setTab(section);
  }, [section, site.id]);

  useEffect(() => {
    if (tab !== "ssl") return;
    let stop = false;
    api
      .get<CertInfo>(`/api/sites/${site.id}/ssl`)
      .then((info) => {
        if (!stop) setSslInfo(info);
      })
      .catch(() => {
        if (!stop) setSslInfo(null);
      });
    return () => {
      stop = true;
    };
  }, [tab, site.id, site.ssl, site.sslKind]);

  useEffect(() => {
    const o = baseOpts(site);
    setDocRoot(toRel(site.username, site.docRoot));
    setRewrite(site.rewrite || "");
    setPhp(site.phpVersion);
    setKind(site.kind || "php");
    setProxyPass(site.proxyPass || "");
    setAppCmd(site.appCmd || "");
    setAppPort(site.appPort ? String(site.appPort) : "");
    setAccess(o.access || "");
    setIps((o.ips || []).join("\n"));
    setIndex((o.index || []).join(" "));
    setHotlink(!!o.hotlink);
    setMaintenance(!!o.maintenance);
    setStaticOn(o.static !== false);
    setStaticExt(o.staticExt || DEFAULT_STATIC_EXT);
    setRedirects(o.redirects || []);
    setDraft("");
    setPicked([]);
  }, [site]);

  const when = addedAt(site.createdAt);
  const label = site.displayName || site.domain;
  const title = `Site modification [${label}]${when ? ` -- Time added [${when}]` : ""}`;
  const port = site.ssl ? "80, 443" : "80";
  const rows = useMemo(
    () => [
      { name: site.domain, label: site.displayName || site.domain, primary: true },
      ...(site.aliases || []).map((name, i) => ({ name, label: site.displayAliases?.[i] || name, primary: false })),
    ],
    [site.domain, site.displayName, site.aliases, site.displayAliases],
  );

  async function addDomains() {
    const lines = draft
      .split(/\r?\n/)
      .map((l) => l.trim().toLowerCase())
      .filter(Boolean);
    if (!lines.length) return;
    const bad = lines.find((l) => l.includes(":"));
    if (bad) {
      message.error(`${bad}: this panel serves ports 80 and 443. Add the hostname only.`);
      return;
    }
    const next = Array.from(new Set([...(site.aliases || []), ...lines.filter((l) => l !== site.domain)]));
    await onPatch({ aliases: next }, "Domains added");
    setDraft("");
  }

  async function deletePicked() {
    const drop = new Set(picked);
    const next = (site.aliases || []).filter((a) => !drop.has(a));
    await onPatch({ aliases: next }, "Domains removed");
    setPicked([]);
  }

  function saveOpts(partial: SiteOptions, ok: string) {
    return onPatch({ options: { ...baseOpts(site), ...partial } }, ok);
  }

  const phpSite = !APP.has(kind) && kind !== "proxy" && kind !== "nginx";

  return (
    <Modal className="site-mod" width={980} title={title} open onCancel={onClose} footer={null} destroyOnHidden>
      <div className="site-mod-body">
        <nav className="site-mod-nav">
          {NAV.map((item) => (
            <button key={item.key} type="button" className={tab === item.key ? "active" : ""} onClick={() => setTab(item.key)}>
              {item.label}
            </button>
          ))}
        </nav>
        <div className="site-mod-pane">
          {tab === "domain" ? (
            <>
              <div className="site-mod-domain-top">
                <div className="site-mod-hint">
                  One domain per line, up to 99 aliases. The default ports are 80 and 443.
                  <br />
                  Wildcard domain format: *.domain.com
                  <br />
                  The primary name stays here. Use Rename to change it.
                  <Input.TextArea
                    rows={3}
                    style={{ marginTop: 12 }}
                    value={draft}
                    placeholder={"www.example.com\n*.example.com"}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                </div>
                <Button type="primary" className="site-mod-add" loading={busy} onClick={() => void addDomains()}>
                  Add
                </Button>
              </div>
              <Table
                size="small"
                rowKey="name"
                pagination={false}
                dataSource={rows}
                rowSelection={{
                  selectedRowKeys: picked,
                  onChange: (keys) => setPicked(keys.map(String)),
                  getCheckboxProps: (row) => ({ disabled: row.primary }),
                }}
                columns={[
                  {
                    title: "Domain name",
                    dataIndex: "label",
                    render: (label: string, row: { name: string; primary: boolean }) => (
                      <span>
                        {row.primary ? <span className="site-mod-primary">{label}</span> : label}
                        {label !== row.name ? <div className="site-list-puny">{row.name}</div> : null}
                      </span>
                    ),
                  },
                  { title: "Port", width: 110, render: () => port },
                  {
                    title: "Operate",
                    width: 120,
                    render: (_, row) =>
                      row.primary ? (
                        <Typography.Text type="secondary">Inoperable</Typography.Text>
                      ) : (
                        <Button
                          type="link"
                          size="small"
                          danger
                          onClick={() => onPatch({ aliases: (site.aliases || []).filter((a) => a !== row.name) }, "Domain removed")}
                        >
                          Delete
                        </Button>
                      ),
                  },
                ]}
              />
              <Button style={{ marginTop: 12 }} disabled={!picked.length} loading={busy} onClick={() => void deletePicked()}>
                Delete Selected
              </Button>
            </>
          ) : null}

          {tab === "directory" ? (
            <>
              <Typography.Paragraph type="secondary">
                Defaults to {homePrefix(site.username)}. A path that starts with / uses another disk, such as /mnt/data/{site.domain}/public_html.
              </Typography.Paragraph>
              <Input
                addonBefore={docRoot.startsWith("/") ? undefined : homePrefix(site.username)}
                value={docRoot}
                onChange={(e) => setDocRoot(e.target.value)}
                placeholder={docRoot.startsWith("/") ? `/mnt/data/${site.domain}/public_html` : `domains/${site.domain}/public_html`}
              />
              <Button style={{ marginTop: 12 }} type="primary" loading={busy} onClick={() => onPatch({ docRoot }, "Directory saved")}>
                Save
              </Button>
            </>
          ) : null}

          {tab === "access" ? (
            <>
              <Typography.Paragraph type="secondary">Nginx allow or deny. One IP or CIDR per line, for example 203.0.113.5 or 10.0.0.0/8.</Typography.Paragraph>
              <Select
                style={{ width: 280, marginBottom: 12 }}
                value={access || "off"}
                onChange={(v: "" | "allow" | "deny" | "off") => setAccess(v === "off" ? "" : v)}
                options={[
                  { value: "off", label: "Off" },
                  { value: "allow", label: "Allow only these addresses" },
                  { value: "deny", label: "Deny these addresses" },
                ]}
              />
              <Input.TextArea rows={6} value={ips} placeholder={"203.0.113.5\n10.0.0.0/8"} onChange={(e) => setIps(e.target.value)} />
              <Button
                style={{ marginTop: 12 }}
                type="primary"
                loading={busy}
                onClick={() =>
                  saveOpts(
                    { access, ips: ips.split(/\r?\n/).map((l) => l.trim()).filter(Boolean) },
                    "Access rules saved",
                  )
                }
              >
                Save
              </Button>
            </>
          ) : null}

          {tab === "rewrite" ? (
            <>
              <Select
                showSearch
                allowClear
                placeholder="Common templates"
                optionFilterProp="label"
                style={{ width: "100%", marginBottom: 8 }}
                value={matchNginxRewrite(rewrite)}
                options={NGINX_REWRITE_OPTIONS}
                onChange={(id) => setRewrite(nginxRewriteBody(id) || "")}
              />
              <Input.TextArea
                rows={10}
                spellCheck={false}
                value={rewrite}
                style={{ fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" }}
                placeholder={"location /vod/ {\n    vod hls;\n    alias /home/user/videos/;\n}\ntry_files $uri $uri/ /index.php?$args;"}
                onChange={(e) => setRewrite(e.target.value)}
              />
              <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
                Location blocks are added in the server. Write location / yourself when you want to replace the default proxy location. Directives without a location stay inside that default location. include, proxy_pass, and listen are rejected.
              </Typography.Paragraph>
              <Button type="primary" loading={busy} onClick={() => onPatch({ rewrite }, "Rewrite saved")}>
                Save
              </Button>
            </>
          ) : null}

          {tab === "index" ? (
            <>
              <Typography.Paragraph type="secondary">Apache DirectoryIndex, in order. Example: index.php index.html</Typography.Paragraph>
              <Input value={index} placeholder="index.php index.html" onChange={(e) => setIndex(e.target.value)} />
              <Button
                style={{ marginTop: 12 }}
                type="primary"
                loading={busy}
                onClick={() => saveOpts({ index: index.split(/\s+/).filter(Boolean) }, "Default document saved")}
              >
                Save
              </Button>
            </>
          ) : null}

          {tab === "config" ? (
            <>
              <Alert
                type="info"
                showIcon
                message="Generated configuration"
                description="Nginx and Apache files are written from these settings. A hand edit is replaced the next time the site is saved."
              />
              <pre className="site-mod-config">{`server_name ${[site.domain, ...(site.aliases || [])].join(" ")};
root ${site.docRoot};
listen 80;
${site.ssl ? "listen 443 ssl;\n" : ""}${phpSite ? `php ${site.phpVersion};\n` : ""}${site.proxyPass ? `proxy_pass ${site.proxyPass};\n` : ""}`}</pre>
            </>
          ) : null}

          {tab === "ssl" ? (
            <>
              <Space wrap>
                <Typography.Text>{sslKindLabel(site)}</Typography.Text>
                {site.ssl && site.sslExpiry ? <Typography.Text type="secondary">{site.sslExpiry}</Typography.Text> : null}
              </Space>
              <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
                A new site starts on HTTP. Issue Let's Encrypt, use a local certificate, or paste your own certificate and private key.
              </Typography.Paragraph>
              {sslInfo?.ok ? (
                <Typography.Paragraph>
                  Subject: {sslInfo.subject || "—"}
                  <br />
                  Issuer: {sslInfo.issuer || "—"}
                  <br />
                  Valid from: {certDate(sslInfo.notBefore)}
                  <br />
                  Valid until: {certDate(sslInfo.notAfter)}
                  <br />
                  Names: {(sslInfo.dnsNames || []).join(", ") || "—"}
                  <br />
                  Serial: {sslInfo.serial || "—"}
                </Typography.Paragraph>
              ) : sslInfo?.message ? (
                <Typography.Paragraph type="secondary">{sslInfo.message}</Typography.Paragraph>
              ) : null}
              {!publicName ? (
                <Alert
                  type="warning"
                  showIcon
                  style={{ marginBottom: 12 }}
                  message={`${label} is not a public TLD`}
                  description="Let's Encrypt will refuse this name. Use local HTTPS, paste a certificate, or set a custom ACME server."
                />
              ) : null}
              <Space wrap>
                <Button loading={busy} onClick={onIssueSSL}>
                  {site.sslKind === "letsencrypt" ? "Renew Let's Encrypt" : "Issue Let's Encrypt"}
                </Button>
                {site.sslKind !== "local" || !site.ssl ? (
                  <Button loading={busy} onClick={onLocalSSL}>
                    Use local HTTPS
                  </Button>
                ) : null}
                {site.ssl ? (
                  <Button loading={busy} onClick={onDisableSSL}>
                    Disable HTTPS
                  </Button>
                ) : null}
              </Space>
              <Typography.Paragraph strong style={{ marginTop: 16 }}>
                Your certificate
              </Typography.Paragraph>
              <Input.TextArea
                value={certPEM}
                onChange={(e) => setCertPEM(e.target.value)}
                placeholder={"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"}
                autoSize={{ minRows: 5, maxRows: 10 }}
                spellCheck={false}
              />
              <Input.TextArea
                style={{ marginTop: 8 }}
                value={keyPEM}
                onChange={(e) => setKeyPEM(e.target.value)}
                placeholder={"-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"}
                autoSize={{ minRows: 4, maxRows: 8 }}
                spellCheck={false}
              />
              <Button
                style={{ marginTop: 8 }}
                loading={busy}
                disabled={!certPEM.trim() || !keyPEM.trim()}
                onClick={() => {
                  if (!onCustomSSL) return;
                  void onCustomSSL(certPEM, keyPEM)
                    .then(() => {
                      setCertPEM("");
                      setKeyPEM("");
                    })
                    .catch(() => undefined);
                }}
              >
                Install certificate
              </Button>
              {sslLog ? <pre className="cmd-out">{sslLog}</pre> : null}
            </>
          ) : null}

          {tab === "php" ? (
            phpSite ? (
              <>
                <Select style={{ width: 220 }} value={php} options={phpVersions} onChange={setPhp} />
                <div>
                  <Button style={{ marginTop: 12 }} type="primary" loading={busy} onClick={() => onPatch({ phpVersion: php }, "PHP version saved")}>
                    Save
                  </Button>
                </div>
              </>
            ) : (
              <Typography.Paragraph>
                {kind === "nginx"
                  ? "Nginx serves this document root directly. PHP is not used."
                  : `This site is ${kind}. PHP-FPM is not in front of it. Change the runtime under Reverse proxy.`}
              </Typography.Paragraph>
            )
          ) : null}

          {tab === "server" ? (
            phpSite ? (
              <>
                <Typography.Paragraph type="secondary">
                  Nginx listens on ports 80 and 443. PHP still goes to Apache. Static files are read from the document root, and a missing file is sent to Apache.
                </Typography.Paragraph>
                <Space>
                  <Switch checked={staticOn} onChange={setStaticOn} />
                  <span>Serve static files directly by nginx</span>
                </Space>
                <Input.TextArea
                  rows={4}
                  style={{ marginTop: 12, fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" }}
                  value={staticExt}
                  disabled={!staticOn}
                  onChange={(e) => setStaticExt(e.target.value)}
                />
                <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
                  One extension per word. PHP stays on Apache.
                </Typography.Paragraph>
                <Button
                  type="primary"
                  loading={busy}
                  onClick={() => {
                    const ext = staticExt.trim().replace(/\s+/g, " ");
                    return saveOpts({ static: staticOn, staticExt: ext === DEFAULT_STATIC_EXT ? "" : ext }, "Web server saved");
                  }}
                >
                  Save
                </Button>
              </>
            ) : kind === "nginx" ? (
              <Typography.Paragraph>
                Nginx serves files from the document root. A missing file returns 404, and PHP files are not executed.
              </Typography.Paragraph>
            ) : (
              <Typography.Paragraph>
                Nginx listens on ports 80 and 443 and proxies this site to {site.proxyPass || "the app port"}.
              </Typography.Paragraph>
            )
          ) : null}

          {tab === "git" ? (
            <>
              <Typography.Paragraph type="secondary">Force-pull a repository into this site and install a webhook.</Typography.Paragraph>
              <Button type="primary" onClick={onGit}>
                Open Git Manager
              </Button>
            </>
          ) : null}

          {tab === "composer" ? (
            <>
              <Typography.Paragraph type="secondary">
                Composer runs as {site.username} inside the site directory. SSH can run composer install. Laravel sites also have queues and artisan.
              </Typography.Paragraph>
              <Space wrap>
                <Button onClick={onSSH}>Open SSH</Button>
                {phpSite ? (
                  <Button type="primary" onClick={onLaravel}>
                    Laravel tools
                  </Button>
                ) : null}
              </Space>
            </>
          ) : null}

          {tab === "redirect" ? (
            <>
              <Space wrap style={{ marginBottom: 12 }}>
                <Input style={{ width: 180 }} placeholder="/old" value={from} onChange={(e) => setFrom(e.target.value)} />
                <Input style={{ width: 220 }} placeholder="/new or https://..." value={to} onChange={(e) => setTo(e.target.value)} />
                <Select
                  style={{ width: 90 }}
                  value={code}
                  onChange={setCode}
                  options={[
                    { value: 301, label: "301" },
                    { value: 302, label: "302" },
                  ]}
                />
                <Button
                  onClick={() => {
                    const src = from.trim();
                    const dst = to.trim();
                    if (!src || !dst) return;
                    setRedirects((list) => [...list.filter((r) => r.from !== src), { from: src, to: dst, code }]);
                    setFrom("");
                    setTo("");
                  }}
                >
                  Add
                </Button>
              </Space>
              <Table
                size="small"
                rowKey="from"
                pagination={false}
                dataSource={redirects}
                columns={[
                  { title: "From", dataIndex: "from" },
                  { title: "To", dataIndex: "to" },
                  { title: "Code", dataIndex: "code", width: 80 },
                  {
                    title: "",
                    width: 80,
                    render: (_, row) => (
                      <Button type="link" size="small" danger onClick={() => setRedirects((list) => list.filter((r) => r.from !== row.from))}>
                        Delete
                      </Button>
                    ),
                  },
                ]}
              />
              <Button style={{ marginTop: 12 }} type="primary" loading={busy} onClick={() => saveOpts({ redirects }, "Redirects saved")}>
                Save
              </Button>
            </>
          ) : null}

          {tab === "proxy" ? (
            <>
              <Select
                style={{ width: 240, marginBottom: 12 }}
                value={kind}
                options={KINDS}
                onChange={(v) => {
                  setKind(v);
                  if (APP.has(v) && !appCmd) setAppCmd(defaultCmd(v));
                }}
              />
              {kind === "proxy" ? (
                <ProxySettings
                  busy={busy}
                  target={proxyPass}
                  value={site.options?.proxy}
                  onSave={(proxy, target) =>
                    onPatch(
                      {
                        kind: "proxy",
                        proxyPass: target,
                        appCmd: "",
                        appPort: 0,
                        options: { ...baseOpts(site), proxy: { ...proxy, target } },
                      },
                      "Proxy saved",
                    )
                  }
                />
              ) : null}
              {APP.has(kind) ? (
                <Space direction="vertical" style={{ width: "100%" }}>
                  <Input placeholder={defaultCmd(kind)} value={appCmd} onChange={(e) => setAppCmd(e.target.value)} />
                  <Input placeholder="port, empty = auto" value={appPort} onChange={(e) => setAppPort(e.target.value)} />
                </Space>
              ) : null}
              {kind !== "proxy" ? (
              <div>
                <Button
                  style={{ marginTop: 12 }}
                  type="primary"
                  loading={busy}
                  onClick={() =>
                    onPatch(
                      {
                        kind,
                        proxyPass: "",
                        appCmd: APP.has(kind) ? appCmd : "",
                        appPort: APP.has(kind) && appPort ? Number(appPort) : 0,
                      },
                      kind === "nginx" ? "Direct nginx saved" : "Proxy saved",
                    )
                  }
                >
                  Save
                </Button>
              </div>
              ) : null}
            </>
          ) : null}

          {tab === "hotlink" ? (
            <>
              <Typography.Paragraph type="secondary">
                Block image, CSS, and JS requests whose Referer is another site. Empty and blocked referers are still allowed.
              </Typography.Paragraph>
              <Space>
                <Switch checked={hotlink} onChange={setHotlink} />
                <span>{hotlink ? "On" : "Off"}</span>
              </Space>
              <div>
                <Button style={{ marginTop: 12 }} type="primary" loading={busy} onClick={() => saveOpts({ hotlink }, "Hotlink protection saved")}>
                  Save
                </Button>
              </div>
            </>
          ) : null}

          {tab === "maintenance" ? (
            <>
              <Typography.Paragraph type="secondary">Visitors receive HTTP 503. The Let's Encrypt challenge path stays open.</Typography.Paragraph>
              <Space>
                <Switch checked={maintenance} onChange={setMaintenance} />
                <span>{maintenance ? "On" : "Off"}</span>
              </Space>
              <div>
                <Button style={{ marginTop: 12 }} type="primary" loading={busy} onClick={() => saveOpts({ maintenance }, "Maintenance mode saved")}>
                  Save
                </Button>
              </div>
            </>
          ) : null}

          {tab === "logs" ? (
            <>
              <Typography.Paragraph type="secondary">Access, error, and panel response logs for this domain.</Typography.Paragraph>
              <Button type="primary" onClick={onLogs}>
                Open response log
              </Button>
            </>
          ) : null}
        </div>
      </div>
    </Modal>
  );
}
