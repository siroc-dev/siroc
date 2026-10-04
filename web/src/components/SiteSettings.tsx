import { useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Input, Modal, Select, Space, Switch, Table, Typography } from "antd";
import { matchNginxRewrite, nginxRewriteBody, NGINX_REWRITE_OPTIONS } from "@/lib/nginxRewrites";

export type SiteOptions = {
  index?: string[];
  access?: "" | "allow" | "deny";
  ips?: string[];
  hotlink?: boolean;
  maintenance?: boolean;
  redirects?: { from: string; to: string; code: number }[];
};

export type SiteSettingsSite = {
  id: number;
  username: string;
  domain: string;
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
  { value: "proxy", label: "Reverse proxy" },
  { value: "nodejs", label: "Node.js" },
  { value: "python", label: "Python" },
  { value: "go", label: "Go" },
  { value: "rust", label: "Rust" },
  { value: "docker", label: "Docker" },
];

const APP = new Set(["nodejs", "python", "go", "rust", "docker"]);

function homePrefix(user: string) {
  return user ? `/home/${user}/` : "/home/";
}

function toRel(user: string, abs: string) {
  const p = homePrefix(user);
  if (abs.startsWith(p)) return abs.slice(p.length);
  return abs.replace(/^\/+/, "");
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
  onLocalSSL,
  onDisableSSL,
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
  onLocalSSL: () => void;
  onDisableSSL: () => void;
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
  const [kind, setKind] = useState("php");
  const [proxyPass, setProxyPass] = useState("");
  const [appCmd, setAppCmd] = useState("");
  const [appPort, setAppPort] = useState("");
  const [access, setAccess] = useState<"" | "allow" | "deny">("");
  const [ips, setIps] = useState("");
  const [index, setIndex] = useState("");
  const [hotlink, setHotlink] = useState(false);
  const [maintenance, setMaintenance] = useState(false);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [code, setCode] = useState(301);
  const [redirects, setRedirects] = useState<{ from: string; to: string; code: number }[]>([]);
  const { message } = App.useApp();

  useEffect(() => {
    setTab(section);
  }, [section, site.id]);

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
    setRedirects(o.redirects || []);
    setDraft("");
    setPicked([]);
  }, [site]);

  const when = addedAt(site.createdAt);
  const title = `Site modification [${site.domain}]${when ? ` -- Time added [${when}]` : ""}`;
  const port = site.ssl ? "80, 443" : "80";
  const rows = useMemo(
    () => [{ name: site.domain, primary: true }, ...(site.aliases || []).map((name) => ({ name, primary: false }))],
    [site.domain, site.aliases],
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

  const phpSite = !APP.has(kind) && kind !== "proxy";

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
                  One domain per line. The default ports are 80 and 443.
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
                  { title: "Domain name", dataIndex: "name", render: (name: string, row) => (row.primary ? <span className="site-mod-primary">{name}</span> : name) },
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
              <Typography.Paragraph type="secondary">Document root for this website. It must stay inside the account home.</Typography.Paragraph>
              <Input addonBefore={homePrefix(site.username)} value={docRoot} onChange={(e) => setDocRoot(e.target.value)} />
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
                placeholder={"rewrite ^/old$ /new permanent;\ntry_files $uri $uri/ /index.php?$args;"}
                onChange={(e) => setRewrite(e.target.value)}
              />
              <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
                Inserted in location / before the site is proxied. include, proxy_pass, and listen are rejected.
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
                <Typography.Text>
                  {site.ssl ? (site.sslKind === "letsencrypt" ? "Let's Encrypt" : "Local HTTPS") : "HTTP only"}
                </Typography.Text>
                {site.ssl && site.sslExpiry ? <Typography.Text type="secondary">{site.sslExpiry}</Typography.Text> : null}
              </Space>
              <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
                Local certificates are self-signed. Let's Encrypt needs a public name. HTTP stays available while the certificate is local.
              </Typography.Paragraph>
              {!publicName ? (
                <Alert
                  type="warning"
                  showIcon
                  style={{ marginBottom: 12 }}
                  message={`${site.domain} is not a public TLD`}
                  description="Let's Encrypt will refuse this name. Use local HTTPS, or set a custom ACME server."
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
              <Typography.Paragraph>This site is {kind}. PHP-FPM is not in front of it. Change the runtime under Reverse proxy.</Typography.Paragraph>
            )
          ) : null}

          {tab === "server" ? (
            <Typography.Paragraph>
              {phpSite
                ? "Nginx listens on ports 80 and 443 and proxies this site to Apache and PHP-FPM on 127.0.0.1:8080."
                : `Nginx listens on ports 80 and 443 and proxies this site to ${site.proxyPass || "the app port"}.`}
            </Typography.Paragraph>
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
                <Input placeholder="http://127.0.0.1:3000/" value={proxyPass} onChange={(e) => setProxyPass(e.target.value)} />
              ) : null}
              {APP.has(kind) ? (
                <Space direction="vertical" style={{ width: "100%" }}>
                  <Input placeholder={defaultCmd(kind)} value={appCmd} onChange={(e) => setAppCmd(e.target.value)} />
                  <Input placeholder="port, empty = auto" value={appPort} onChange={(e) => setAppPort(e.target.value)} />
                </Space>
              ) : null}
              <div>
                <Button
                  style={{ marginTop: 12 }}
                  type="primary"
                  loading={busy}
                  onClick={() =>
                    onPatch(
                      {
                        kind,
                        proxyPass: kind === "proxy" ? proxyPass : "",
                        appCmd: APP.has(kind) ? appCmd : "",
                        appPort: APP.has(kind) && appPort ? Number(appPort) : 0,
                      },
                      "Proxy saved",
                    )
                  }
                >
                  Save
                </Button>
              </div>
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
