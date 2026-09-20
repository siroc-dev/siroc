import { useEffect, useState } from "react";
import { useOutletContext } from "react-router-dom";
import { MoreOutlined } from "@ant-design/icons";
import { Alert, App, Button, Card, Dropdown, Flex, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tabs, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { asList } from "@/lib/lists";
import { matchNginxRewrite, nginxRewriteBody, NGINX_REWRITE_OPTIONS } from "@/lib/nginxRewrites";
import { AppPackages, type PkgHit, type PkgRow } from "@/components/AppPackages";
import { WebOptimize } from "@/components/WebOptimize";
import { ArtisanRun, type ArtisanCmd } from "@/components/ArtisanRun";
import { LaravelQueues, type LaravelQueueRow, type LaravelSchedule } from "@/components/LaravelQueues";
import { SiteLogs } from "@/components/SiteLogs";
import type { FormInstance } from "antd/es/form";

type Account = { username: string; wafEnabled?: boolean };
function homePrefix(user: string) {
  return user ? `/home/${user}/` : "/home/";
}

function toRel(user: string, abs: string) {
  if (!abs) return "";
  const p = homePrefix(user);
  if (abs.startsWith(p)) return abs.slice(p.length);
  if (user && abs === `/home/${user}`) return "";
  return abs.replace(/^\/+/, "");
}

function defaultDoc(domain: string) {
  const d = (domain || "").trim().toLowerCase();
  return d ? `domains/${d}/public_html` : "";
}

type Site = {
  id: number;
  username: string;
  domain: string;
  docRoot: string;
  phpVersion: string;
  enabled: boolean;
  aliases: string[];
  ssl: boolean;
  sslKind?: string;
  sslExpiry?: string;
  rewrite?: string;
  kind?: string;
  proxyPass?: string;
  appPort?: number;
  appCmd?: string;
  wafEnabled?: boolean;
};

const SITE_KINDS = [
  { value: "php", label: "PHP / Apache" },
  { value: "proxy", label: "Reverse proxy" },
  { value: "nodejs", label: "Node.js" },
  { value: "python", label: "Python" },
  { value: "go", label: "Go" },
  { value: "rust", label: "Rust" },
  { value: "docker", label: "Docker" },
];

const APP_KINDS = new Set(["nodejs", "python", "go", "rust", "docker"]);

function kindLabel(kind?: string) {
  return SITE_KINDS.find((k) => k.value === kind)?.label || kind || "PHP / Apache";
}

function defaultAppCmd(kind: string) {
  switch (kind) {
    case "nodejs":
      return "node server.js";
    case "python":
      return "python3 app.py";
    case "go":
      return "go run .";
    case "rust":
      return "cargo run --release";
    case "docker":
      return "docker compose up --build --abort-on-container-exit";
    default:
      return "";
  }
}

type AppRuntime = {
  ok: boolean;
  kind: string;
  active: boolean;
  status?: string;
  port?: number;
  cmd?: string;
  unit?: string;
  log?: string;
  message?: string;
};

type LEConfig = {
  email: string;
  server: string;
  directory?: string;
  keyType: string;
  rsaKeySize?: number;
  eabKid?: string;
  hasEabHmac?: boolean;
  noVerify?: boolean;
  registered?: boolean;
  accountUri?: string;
  certbotInstalled?: boolean;
};

const reservedTLD = new Set(["test", "localhost", "invalid", "example", "local", "onion", "internal", "lan", "home", "corp", "private", "localdomain"]);

function publicHostname(d: string) {
  const parts = (d || "").toLowerCase().replace(/\.$/, "").split(".").filter(Boolean);
  if (parts.length < 2) return false;
  const tld = parts[parts.length - 1];
  return tld.length >= 2 && !reservedTLD.has(tld);
}

function leKeyValue(cfg?: LEConfig | null) {
  if (!cfg) return "ecdsa";
  if (cfg.keyType === "rsa" && cfg.rsaKeySize === 4096) return "rsa4096";
  if (cfg.keyType === "rsa") return "rsa2048";
  if (cfg.keyType === "ecdsa-p384") return "ecdsa-p384";
  return "ecdsa";
}

type LaravelStatus = {
  queue: boolean;
  queueName?: string;
  workers: number;
  queues?: LaravelQueueRow[];
  processes?: LaravelQueueRow["processes"];
  scheduler: boolean;
  schedule?: LaravelSchedule[];
  scheduleLast?: string;
  envExists: boolean;
  env?: string;
  hasComposer: boolean;
  hasPackage: boolean;
  hasArtisan: boolean;
  composerPkgs?: PkgRow[];
  npmPkgs?: PkgRow[];
  npmScripts?: string[];
  artisanCmds?: ArtisanCmd[];
};

type WPUser = { id: number; login: string; email: string; name: string; roles: string };
type WPPlugin = { name: string; status: string; version: string; update?: string; title: string };
type WPIssue = { id: string; level: string; title: string; detail: string };
type WPUpdates = { core?: string; plugins?: WPPlugin[]; themes?: string[] };
type WPStatus = {
  name: string;
  url: string;
  home: string;
  version: string;
  prefix?: string;
  xmlrpcBlocked?: boolean;
  pingbacksOff?: boolean;
  uploadsPhpBlocked?: boolean;
  users?: WPUser[];
  plugins?: WPPlugin[];
  updates?: WPUpdates;
  security?: WPIssue[];
  wpCmds?: ArtisanCmd[];
};

type SiteApp = {
  ok: boolean;
  kind: string;
  appRoot?: string;
  message?: string;
  output?: string;
  loginUrls?: string[];
  laravel?: LaravelStatus;
  wordpress?: WPStatus;
  pkgHits?: PkgHit[];
};

function sslTag(s: Site) {
  if (!s.ssl) return <Tag>HTTP</Tag>;
  if (s.sslKind === "letsencrypt") return <Tag color="success">Let's Encrypt</Tag>;
  return <Tag color="processing">Local HTTPS</Tag>;
}

function mergeApp(cur: SiteApp | null, out: SiteApp): SiteApp {
  let next = out;
  if (cur?.laravel && out.laravel) {
    next = {
      ...next,
      laravel: {
        ...out.laravel,
        composerPkgs: out.laravel.composerPkgs ?? cur.laravel.composerPkgs,
        npmPkgs: out.laravel.npmPkgs ?? cur.laravel.npmPkgs,
        npmScripts: out.laravel.npmScripts ?? cur.laravel.npmScripts,
        artisanCmds: out.laravel.artisanCmds ?? cur.laravel.artisanCmds,
        env: out.laravel.env ?? cur.laravel.env,
      },
    };
  }
  if (cur?.wordpress && out.wordpress) {
    next = {
      ...next,
      wordpress: {
        ...out.wordpress,
        users: out.wordpress.users ?? cur.wordpress.users,
        plugins: out.wordpress.plugins ?? cur.wordpress.plugins,
        updates: out.wordpress.updates ?? cur.wordpress.updates,
        security: out.wordpress.security ?? cur.wordpress.security,
        wpCmds: out.wordpress.wpCmds ?? cur.wordpress.wpCmds,
        prefix: out.wordpress.prefix || cur.wordpress.prefix,
      },
    };
  } else if (cur?.wordpress && !out.wordpress) {
    next = { ...next, wordpress: cur.wordpress };
  }
  return next;
}

function randomWPPrefix() {
  const letters = "abcdefghijklmnopqrstuvwxyz";
  const alnum = letters + "0123456789";
  const bytes = new Uint8Array(5);
  crypto.getRandomValues(bytes);
  let s = letters[bytes[0] % letters.length];
  for (let i = 1; i < 5; i++) s += alnum[bytes[i] % alnum.length];
  return `${s}_`;
}

function randomWPLogin() {
  const letters = "abcdefghijklmnopqrstuvwxyz";
  const alnum = letters + "0123456789";
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  let s = letters[bytes[0] % letters.length];
  for (let i = 1; i < 8; i++) s += alnum[bytes[i] % alnum.length];
  return s;
}

function RewriteBlock({ form }: { form: FormInstance }) {
  return (
    <Form.Item noStyle shouldUpdate>
      {() => {
        const current = (form.getFieldValue("rewrite") as string) || "";
        return (
          <Form.Item
            label="Nginx rewrite"
            extra="Inserted in location / before proxy_pass. Pick a common template or edit the rules."
          >
            <Select
              showSearch
              allowClear
              placeholder="Common templates"
              optionFilterProp="label"
              value={matchNginxRewrite(current)}
              options={NGINX_REWRITE_OPTIONS}
              onChange={(id) => form.setFieldsValue({ rewrite: nginxRewriteBody(id) })}
              style={{ width: "100%", marginBottom: 8 }}
            />
            <Form.Item name="rewrite" noStyle>
              <Input.TextArea
                rows={6}
                spellCheck={false}
                placeholder={"rewrite ^/old$ /new permanent;\ntry_files $uri $uri/ /index.php?$args;"}
                style={{ fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" }}
              />
            </Form.Item>
          </Form.Item>
        );
      }}
    </Form.Item>
  );
}

export function Sites() {
  const { message, modal } = App.useApp();
  const { admin } = useOutletContext<{ user: string; admin?: boolean }>();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [sites, setSites] = useState<Site[]>([]);
  const [phps, setPhps] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [edit, setEdit] = useState<Site | null>(null);
  const [logSite, setLogSite] = useState<Site | null>(null);
  const [stats, setStats] = useState<Site | null>(null);
  const [statsSrc, setStatsSrc] = useState("");
  const [statsErr, setStatsErr] = useState("");
  const [statsBusy, setStatsBusy] = useState(false);
  const [form] = Form.useForm();
  const [editForm] = Form.useForm();
  const [renameForm] = Form.useForm();
  const [renameSite, setRenameSite] = useState<Site | null>(null);
  const [appSite, setAppSite] = useState<Site | null>(null);
  const [appKind, setAppKind] = useState<"laravel" | "wordpress" | null>(null);
  const [app, setApp] = useState<SiteApp | null>(null);
  const [appBusy, setAppBusy] = useState(false);
  const [envText, setEnvText] = useState("");
  const [wpName, setWpName] = useState("");
  const [wpUrl, setWpUrl] = useState("");
  const [wpPrefix, setWpPrefix] = useState("");
  const [wpForm] = Form.useForm();
  const [loginUrls, setLoginUrls] = useState<string[]>([]);
  const [leForm] = Form.useForm();
  const [leBusy, setLeBusy] = useState(false);
  const [leAccount, setLeAccount] = useState<LEConfig | null>(null);
  const [runtimeSite, setRuntimeSite] = useState<Site | null>(null);
  const [runtime, setRuntime] = useState<AppRuntime | null>(null);
  const [runtimeBusy, setRuntimeBusy] = useState(false);
  const [wafBusy, setWafBusy] = useState("");

  async function load() {
    const [a, s, p] = await Promise.all([
      api.get<Account[]>("/api/accounts"),
      api.get<Site[]>("/api/sites"),
      api.get<string[]>("/api/software/php-versions").catch(() => [] as string[]),
    ]);
    const accounts = asList(a);
    const sites = asList(s);
    const phps = asList(p);
    setAccounts(accounts);
    setSites(sites);
    setPhps(phps);
    if (accounts[0] && !form.getFieldValue("username")) {
      form.setFieldsValue({ username: accounts[0].username, phpVersion: phps.length ? phps[phps.length - 1] : "8.3" });
    }
  }

  async function loadLE() {
    if (!admin) return;
    const data = await api.get<LEConfig>("/api/ssl/letsencrypt");
    leForm.setFieldsValue({
      email: data.email,
      server: data.server || "production",
      directory: data.directory,
      key: leKeyValue(data),
      eabKid: data.eabKid,
      eabHmac: data.hasEabHmac ? "********" : "",
      noVerify: data.noVerify,
    });
    setLeAccount(data);
  }

  function accountWAFOn(username: string) {
    return accounts.find((a) => a.username === username)?.wafEnabled !== false;
  }

  async function setAccountWAF(username: string, enabled: boolean) {
    const key = `acc:${username}`;
    setWafBusy(key);
    try {
      await api.put(`/api/accounts/${encodeURIComponent(username)}/waf`, { enabled });
      message.success(enabled ? "ModSecurity enabled for this account" : "ModSecurity disabled for this account");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setWafBusy("");
    }
  }

  async function setSiteWAF(s: Site, enabled: boolean) {
    const key = `site:${s.id}`;
    setWafBusy(key);
    try {
      await api.put(`/api/sites/${s.id}/waf`, { enabled });
      message.success(enabled ? `ModSecurity on for ${s.domain}` : `ModSecurity off for ${s.domain}`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setWafBusy("");
    }
  }

  useEffect(() => {
    load().catch((e) => message.error(e.message));
    loadLE().catch((e) => message.error(e.message));
  }, []);

  function leBody(v: { email?: string; server: string; directory?: string; key: string; eabKid?: string; eabHmac?: string; noVerify?: boolean }) {
    const key = v.key;
    const body: Record<string, unknown> = {
      email: v.email || "",
      server: v.server,
      directory: v.directory || "",
      keyType: key === "rsa2048" || key === "rsa4096" ? "rsa" : key,
      rsaKeySize: key === "rsa4096" ? 4096 : key === "rsa2048" ? 2048 : 0,
      eabKid: v.eabKid || "",
      noVerify: !!v.noVerify,
    };
    if (v.eabHmac && v.eabHmac !== "********") body.eabHmac = v.eabHmac;
    return body;
  }

  async function saveLE(v: { email?: string; server: string; directory?: string; key: string; eabKid?: string; eabHmac?: string; noVerify?: boolean }) {
    setLeBusy(true);
    try {
      await api.put("/api/ssl/letsencrypt", leBody(v));
      message.success("Let's Encrypt settings saved");
      await loadLE();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setLeBusy(false);
    }
  }

  async function createLEAccount() {
    setLeBusy(true);
    try {
      const v = await leForm.validateFields();
      const out = await api.post<{ registered?: boolean; message?: string }>("/api/ssl/letsencrypt/account", leBody(v));
      message.success(out.message || (out.registered ? "Let's Encrypt account is ready" : "Account request finished"));
      await loadLE();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setLeBusy(false);
    }
  }

  function phpOptions(current?: string) {
    const list = [...phps];
    if (current && !list.includes(current)) list.unshift(current);
    return list.map((v) => ({ value: v, label: `PHP ${v}` }));
  }

  async function create(values: { username: string; domain: string; phpVersion: string; aliases?: string[]; docRoot?: string; kind?: string; proxyPass?: string; appPort?: number; appCmd?: string; rewrite?: string }) {
    setBusy(true);
    try {
      await api.post("/api/sites", values);
      form.resetFields();
      setCreateOpen(false);
      message.success("Website created with a local HTTPS certificate");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  function openEdit(s: Site) {
    setEdit(s);
    editForm.setFieldsValue({
      phpVersion: s.phpVersion,
      aliases: s.aliases || [],
      rewrite: s.rewrite || "",
      docRoot: toRel(s.username, s.docRoot),
      kind: s.kind || "php",
      proxyPass: s.proxyPass || "",
      appPort: s.appPort || undefined,
      appCmd: s.appCmd || defaultAppCmd(s.kind || "php"),
    });
  }

  async function saveEdit(values: { phpVersion: string; aliases: string[]; rewrite?: string; docRoot: string; kind?: string; proxyPass?: string; appPort?: number; appCmd?: string }) {
    if (!edit) return;
    setBusy(true);
    try {
      await api.patch(`/api/sites/${edit.id}`, values);
      message.success(`${edit.domain} updated`);
      setEdit(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
      await load();
    } finally {
      setBusy(false);
    }
  }

  function openRename(s: Site) {
    setRenameSite(s);
    renameForm.setFieldsValue({ domain: s.domain });
  }

  async function saveRename(values: { domain: string }) {
    if (!renameSite) return;
    setBusy(true);
    try {
      await api.post(`/api/sites/${renameSite.id}/rename`, { domain: values.domain });
      message.success(`${renameSite.domain} renamed to ${values.domain}`);
      setRenameSite(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function openRuntime(s: Site) {
    setRuntimeSite(s);
    setRuntime(null);
    setRuntimeBusy(true);
    try {
      const out = await api.post<AppRuntime>(`/api/sites/${s.id}/runtime`, { action: "status" });
      setRuntime(out);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setRuntimeBusy(false);
    }
  }

  async function runRuntime(action: string) {
    if (!runtimeSite) return;
    setRuntimeBusy(true);
    try {
      const out = await api.post<AppRuntime>(`/api/sites/${runtimeSite.id}/runtime`, { action });
      setRuntime(out);
      message.success(action === "logs" ? "Log refreshed" : `App ${action}`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setRuntimeBusy(false);
    }
  }

  async function toggle(s: Site) {
    try {
      await api.patch(`/api/sites/${s.id}`, { enabled: !s.enabled });
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  async function issueSSL() {
    if (!edit) return;
    setBusy(true);
    try {
      const st = await api.post<Site>(`/api/sites/${edit.id}/ssl`, {});
      message.success(`Let's Encrypt issued for ${edit.domain}`);
      setEdit(st);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function useLocalSSL() {
    if (!edit) return;
    setBusy(true);
    try {
      const st = await api.patch<Site>(`/api/sites/${edit.id}`, { ssl: true, sslKind: "local" });
      message.success(`Local HTTPS enabled for ${edit.domain}`);
      setEdit(st);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function disableSSL() {
    if (!edit) return;
    setBusy(true);
    try {
      const st = await api.delete<Site>(`/api/sites/${edit.id}/ssl`);
      message.success(`HTTPS disabled for ${edit.domain}`);
      setEdit(st);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function loadStats(s: Site, refresh: boolean) {
    setStatsBusy(true);
    setStatsErr("");
    try {
      await api.post(`/api/sites/${s.id}/stats`, { refresh });
      setStatsSrc(`/api/sites/${s.id}/stats.html?t=${Date.now()}`);
    } catch (err) {
      setStatsSrc("");
      setStatsErr(err instanceof Error ? err.message : "Failed");
    } finally {
      setStatsBusy(false);
    }
  }

  function openStats(s: Site) {
    setStats(s);
    setStatsSrc("");
    setStatsErr("");
    loadStats(s, false);
  }

  async function remove(id: number) {
    try {
      await api.delete(`/api/sites/${id}`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  async function runApp(site: Site, body: Record<string, unknown>) {
    setAppBusy(true);
    try {
      const out = await api.post<SiteApp>(`/api/sites/${site.id}/app`, body);
      setApp((cur) => mergeApp(cur, out));
      if (out.laravel?.env != null) setEnvText(out.laravel.env);
      if (out.wordpress) {
        setWpName(out.wordpress.name || "");
        setWpUrl(out.wordpress.url || out.wordpress.home || "");
        if (out.wordpress.prefix) setWpPrefix(out.wordpress.prefix);
      }
      if (out.loginUrls?.length) setLoginUrls(out.loginUrls);
      if (out.message && out.output) message.warning(out.message);
      else if (out.output) message.success("Command finished");
      else if (out.message) message.success(out.message);
      return out;
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
      return null;
    } finally {
      setAppBusy(false);
    }
  }

  async function openApp(s: Site, kind: "laravel" | "wordpress") {
    setAppSite(s);
    setAppKind(kind);
    setApp(null);
    setEnvText("");
    setLoginUrls([]);
    const out = await runApp(s, { action: "status" });
    if (!out) return;
    if (kind === "laravel" && out.kind === "laravel") {
      await runApp(s, { action: "env-get" });
      void loadPackages(s);
    }
    if (kind === "wordpress" && out.kind !== "wordpress") {
      wpForm.setFieldsValue({
        title: s.domain,
        url: `http://${s.domain}`,
        prefix: randomWPPrefix(),
        adminUser: randomWPLogin(),
        dbSuffix: "wp",
      });
    }
  }

  async function loadPackages(s: Site) {
    try {
      const out = await api.post<SiteApp>(`/api/sites/${s.id}/app`, { action: "packages" });
      setApp((cur) => (cur ? { ...cur, laravel: out.laravel || cur.laravel } : out));
    } catch {
      /* outdated lookup can fail offline */
    }
  }

  async function searchPkgs(kind: "composer" | "npm", query: string): Promise<PkgHit[]> {
    if (!appSite) return [];
    const body =
      kind === "npm"
        ? { action: "pkg-search", name: "npm", npm: query }
        : { action: "pkg-search", name: "composer", composer: query };
    const out = await api.post<SiteApp>(`/api/sites/${appSite.id}/app`, body);
    return out.pkgHits || [];
  }

  const sitesTable = (
    <Card>
      <Table
        rowKey="id"
        dataSource={sites}
        pagination={false}
        locale={{ emptyText: "No websites yet. Install nginx, Apache, and PHP first." }}
        columns={[
          { title: "Domain", dataIndex: "domain", ellipsis: true },
          { title: "User", dataIndex: "username", width: 90 },
          {
            title: "Aliases",
            ellipsis: true,
            render: (_, s) =>
              s.aliases?.length ? (
                <Space size={4} wrap>
                  {s.aliases.map((a) => (
                    <Tag key={a}>{a}</Tag>
                  ))}
                </Space>
              ) : (
                <Typography.Text type="secondary">None</Typography.Text>
              ),
          },
          {
            title: "Type",
            width: 120,
            render: (_, s) =>
              APP_KINDS.has(s.kind || "") ? (
                <Tag color="blue">{kindLabel(s.kind)}{s.appPort ? ` :${s.appPort}` : ""}</Tag>
              ) : s.kind === "proxy" ? (
                <Tag>Proxy</Tag>
              ) : (
                <Tag>PHP {s.phpVersion}</Tag>
              ),
          },
          {
            title: "Path",
            ellipsis: true,
            render: (_, s) => <Typography.Text type="secondary">{toRel(s.username, s.docRoot) || s.docRoot}</Typography.Text>,
          },
          {
            title: "Rewrite",
            width: 80,
            render: (_, s) => (s.rewrite?.trim() ? "Yes" : "—"),
          },
          {
            title: "SSL",
            width: 130,
            render: (_, s) => sslTag(s),
          },
          {
            title: "WAF",
            width: 88,
            render: (_, s) => {
              const accountOn = accountWAFOn(s.username);
              return (
                <Switch
                  size="small"
                  checked={accountOn && s.wafEnabled !== false}
                  disabled={!accountOn || wafBusy === `site:${s.id}`}
                  loading={wafBusy === `site:${s.id}`}
                  onChange={(v) => void setSiteWAF(s, v)}
                />
              );
            },
          },
          {
            title: "Status",
            width: 100,
            render: (_, s) => <Tag color={s.enabled ? "success" : "default"}>{s.enabled ? "Enabled" : "Disabled"}</Tag>,
          },
          {
            title: "",
            width: 56,
            align: "right",
            render: (_, s) => (
              <Dropdown
                trigger={["click"]}
                menu={{
                  items: [
                    { key: "logs", label: "Logs" },
                    { key: "stats", label: "Stats" },
                    { key: "edit", label: "Edit" },
                    { key: "rename", label: "Rename" },
                    ...(APP_KINDS.has(s.kind || "") ? [{ key: "app", label: "App" }] : []),
                    ...(!APP_KINDS.has(s.kind || "") && s.kind !== "proxy"
                      ? [
                          { key: "laravel", label: "Laravel" },
                          { key: "wordpress", label: "WordPress" },
                        ]
                      : []),
                    { key: "toggle", label: s.enabled ? "Disable" : "Enable" },
                    { type: "divider" },
                    { key: "delete", label: "Delete", danger: true },
                  ],
                  onClick: ({ key }) => {
                    if (key === "logs") setLogSite(s);
                    if (key === "stats") openStats(s);
                    if (key === "edit") openEdit(s);
                    if (key === "rename") openRename(s);
                    if (key === "app") void openRuntime(s);
                    if (key === "laravel") void openApp(s, "laravel");
                    if (key === "wordpress") void openApp(s, "wordpress");
                    if (key === "toggle") void toggle(s);
                    if (key === "delete") {
                      modal.confirm({
                        title: `Delete ${s.domain}?`,
                        okText: "Delete",
                        okButtonProps: { danger: true },
                        onOk: () => remove(s.id),
                      });
                    }
                  },
                }}
              >
                <Button size="small" icon={<MoreOutlined />} />
              </Dropdown>
            ),
          },
        ]}
      />
    </Card>
  );

  const leStatus = leAccount?.registered
    ? { type: "success" as const, message: "Let's Encrypt account is registered", description: leAccount.accountUri ? `Account: ${leAccount.accountUri}` : "Certbot can issue certificates with this account." }
    : leAccount && !leAccount.certbotInstalled
      ? { type: "warning" as const, message: "Certbot is not installed yet", description: "Install Let's Encrypt (Certbot) from Software. The account will be created automatically, or click Create account after that." }
      : { type: "info" as const, message: "Let's Encrypt account is not registered yet", description: "Save the email, then create the account. Production HTTP-01 needs port 80 reachable from the internet." };

  const leTab = (
    <Card>
      <Alert type={leStatus.type} showIcon style={{ marginBottom: 16 }} message={leStatus.message} description={leStatus.description} />
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="Let's Encrypt only issues certificates for public domains"
        description="Names like a02.test are not a public TLD. Use local HTTPS for lab sites, or set a custom ACME directory (Pebble, step-ca, ZeroSSL)."
      />
      <Form form={leForm} layout="vertical" onFinish={saveLE} requiredMark={false}>
        <Form.Item
          name="email"
          label="Account email"
          extra="Registers the Let's Encrypt ACME account and receives expiry notices."
          rules={[{ required: true, type: "email", message: "Enter a valid email" }]}
        >
          <Input placeholder="admin@example.com" />
        </Form.Item>
        <Form.Item name="server" label="ACME server">
          <Select
            options={[
              { value: "production", label: "Let's Encrypt (production)" },
              { value: "staging", label: "Let's Encrypt (staging)" },
              { value: "custom", label: "Custom ACME directory" },
            ]}
          />
        </Form.Item>
        <Form.Item noStyle shouldUpdate>
          {() =>
            leForm.getFieldValue("server") === "custom" ? (
              <>
                <Form.Item
                  name="directory"
                  label="ACME directory URL"
                  rules={[{ required: true, message: "Directory URL required" }]}
                >
                  <Input placeholder="https://acme.example.com/directory" />
                </Form.Item>
                <Form.Item name="noVerify" label="Skip TLS verify" valuePropName="checked">
                  <Switch />
                </Form.Item>
              </>
            ) : null
          }
        </Form.Item>
        <Form.Item name="key" label="Certificate key">
          <Select
            options={[
              { value: "ecdsa", label: "ECDSA P-256" },
              { value: "ecdsa-p384", label: "ECDSA P-384" },
              { value: "rsa2048", label: "RSA 2048" },
              { value: "rsa4096", label: "RSA 4096" },
            ]}
          />
        </Form.Item>
        <Form.Item name="eabKid" label="EAB Key ID" extra="Required by some CAs such as ZeroSSL or Google Trust Services.">
          <Input placeholder="optional" />
        </Form.Item>
        <Form.Item name="eabHmac" label="EAB HMAC key">
          <Input.Password placeholder="optional" />
        </Form.Item>
        <Space>
          <Button type="primary" htmlType="submit" loading={leBusy}>
            Save settings
          </Button>
          <Button onClick={() => void createLEAccount()} loading={leBusy}>
            {leAccount?.registered ? "Update Let's Encrypt account" : "Create Let's Encrypt account"}
          </Button>
        </Space>
      </Form>
    </Card>
  );

  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 16 }}>
        <div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            Websites
          </Typography.Title>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            New sites get a local HTTPS certificate. Let's Encrypt can replace it later. HTTP on port 8080 stays available until Let's Encrypt is issued.
          </Typography.Paragraph>
        </div>
        <Button type="primary" onClick={() => setCreateOpen(true)} disabled={phps.length === 0}>
          New site
        </Button>
      </div>
      {accounts.length > 0 ? (
        <Card style={{ marginBottom: 16 }} title="ModSecurity WAF">
          <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
            Blocks common attacks on Apache (SQL injection, XSS). The global engine stays under Security. Turn WAF off for your whole account, or disable it on a single website in the table.
          </Typography.Paragraph>
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            {accounts.map((a) => (
              <Flex key={a.username} align="center" justify="space-between" gap={16}>
                <div>
                  <Typography.Text strong>{admin ? a.username : "Protect my websites"}</Typography.Text>
                  <div>
                    <Typography.Text type="secondary">
                      {a.wafEnabled !== false ? "On for this account" : "Off for this account — site switches are disabled"}
                    </Typography.Text>
                  </div>
                </div>
                <Switch
                  checked={a.wafEnabled !== false}
                  loading={wafBusy === `acc:${a.username}`}
                  onChange={(v) => void setAccountWAF(a.username, v)}
                />
              </Flex>
            ))}
          </Space>
        </Card>
      ) : null}
      {admin ? (
        <Tabs
          items={[
            { key: "sites", label: "Websites", children: sitesTable },
            { key: "le", label: "Let's Encrypt", children: leTab },
            { key: "optimize", label: "Optimize", children: <WebOptimize /> },
          ]}
        />
      ) : (
        sitesTable
      )}

      <Modal
        title="New website"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={busy}
        destroyOnHidden
        okText="Create vhost"
        okButtonProps={{ disabled: phps.length === 0 && !APP_KINDS.has(form.getFieldValue("kind")) && form.getFieldValue("kind") !== "proxy" }}
      >
        <Form form={form} layout="vertical" onFinish={create} requiredMark={false} style={{ marginTop: 8 }}>
          <Form.Item name="username" label="Account" rules={[{ required: true }]}>
            <Select
              options={accounts.map((a) => ({ value: a.username, label: a.username }))}
              onChange={() => {
                const d = form.getFieldValue("domain");
                if (d) form.setFieldsValue({ docRoot: defaultDoc(d) });
              }}
            />
          </Form.Item>
          <Form.Item name="domain" label="Domain" rules={[{ required: true }]}>
            <Input
              placeholder="site.test"
              onChange={(e) => {
                const d = e.target.value;
                const cur = form.getFieldValue("docRoot") as string | undefined;
                if (!cur || cur.startsWith("domains/")) form.setFieldsValue({ docRoot: defaultDoc(d) });
              }}
            />
          </Form.Item>
          <Form.Item noStyle shouldUpdate>
            {() => {
              const user = (form.getFieldValue("username") as string) || "";
              return (
                <Form.Item name="docRoot" label="Document root" extra={`Must stay inside ${homePrefix(user)}`}>
                  <Input addonBefore={homePrefix(user)} placeholder="domains/site.test/public_html" />
                </Form.Item>
              );
            }}
          </Form.Item>
          <Form.Item name="aliases" label="Aliases">
            <Select mode="tags" tokenSeparators={[",", " "]} placeholder="www.site.test" />
          </Form.Item>
          <Form.Item name="kind" label="Type" initialValue="php">
            <Select
              options={SITE_KINDS}
              onChange={(v) => {
                if (APP_KINDS.has(v) && !form.getFieldValue("appCmd")) {
                  form.setFieldsValue({ appCmd: defaultAppCmd(v) });
                }
              }}
            />
          </Form.Item>
          <Form.Item noStyle shouldUpdate>
            {() => {
              const kind = form.getFieldValue("kind") as string;
              if (kind === "proxy") {
                return (
                  <Form.Item name="proxyPass" label="Proxy URL" rules={[{ required: true }]} extra="Example: http://127.0.0.1:3000/">
                    <Input placeholder="http://127.0.0.1:3000/" />
                  </Form.Item>
                );
              }
              if (APP_KINDS.has(kind)) {
                return (
                  <>
                    <Form.Item name="appCmd" label="Start command" extra="Runs as the Linux user. PORT is set automatically.">
                      <Input placeholder={defaultAppCmd(kind)} />
                    </Form.Item>
                    <Form.Item name="appPort" label="Port" extra="Leave empty to assign 30000+ automatically. nginx proxies to 127.0.0.1:port.">
                      <InputNumber min={1024} max={65535} style={{ width: "100%" }} placeholder="auto" />
                    </Form.Item>
                  </>
                );
              }
              return (
                <Form.Item name="phpVersion" label="PHP" rules={[{ required: true }]}>
                  <Select options={phpOptions()} />
                </Form.Item>
              );
            }}
          </Form.Item>
          <RewriteBlock form={form} />
        </Form>
      </Modal>

      <Modal
        width={720}
        title={edit ? `Edit ${edit.domain}` : "Edit website"}
        open={!!edit}
        onCancel={() => setEdit(null)}
        onOk={() => editForm.submit()}
        confirmLoading={busy}
        destroyOnHidden
        okText="Save"
      >
        {edit ? (
          <Form form={editForm} layout="vertical" onFinish={saveEdit} requiredMark={false} style={{ marginTop: 8 }}>
            <Form.Item name="phpVersion" label="PHP version">
              <Select options={phpOptions(edit.phpVersion)} />
            </Form.Item>
            <Form.Item name="kind" label="Type">
              <Select
                options={SITE_KINDS}
                onChange={(v) => {
                  if (APP_KINDS.has(v) && !editForm.getFieldValue("appCmd")) {
                    editForm.setFieldsValue({ appCmd: defaultAppCmd(v) });
                  }
                }}
              />
            </Form.Item>
            <Form.Item noStyle shouldUpdate>
              {() => {
                const kind = editForm.getFieldValue("kind") as string;
                if (kind === "proxy") {
                  return (
                    <Form.Item name="proxyPass" label="Proxy URL" extra="nginx proxy_pass target">
                      <Input placeholder="http://127.0.0.1:3000/" />
                    </Form.Item>
                  );
                }
                if (APP_KINDS.has(kind)) {
                  return (
                    <>
                      <Form.Item name="appCmd" label="Start command">
                        <Input placeholder={defaultAppCmd(kind)} />
                      </Form.Item>
                      <Form.Item name="appPort" label="Port">
                        <InputNumber min={1024} max={65535} style={{ width: "100%" }} />
                      </Form.Item>
                    </>
                  );
                }
                return null;
              }}
            </Form.Item>
            <Form.Item
              name="docRoot"
              label="Document root"
              extra={`Must stay inside ${homePrefix(edit.username)}`}
              rules={[{ required: true, message: "Path required" }]}
            >
              <Input addonBefore={homePrefix(edit.username)} />
            </Form.Item>
            <Form.Item name="aliases" label="Domain aliases">
              <Select mode="tags" tokenSeparators={[",", " "]} placeholder="www.example.com" />
            </Form.Item>
            <RewriteBlock form={editForm} />
            <Typography.Text type="secondary">SSL</Typography.Text>
            <div style={{ marginTop: 8 }}>
              <Space wrap>
                {sslTag(edit)}
                {edit.ssl && edit.sslExpiry ? <Typography.Text type="secondary">{edit.sslExpiry}</Typography.Text> : null}
              </Space>
            </div>
            <Typography.Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
              Local certificates are self-signed (browser warning). Let's Encrypt needs a public domain. HTTP is not redirected while using a local certificate.
            </Typography.Paragraph>
            {!publicHostname(edit.domain) ? (
              <Alert
                type="warning"
                showIcon
                style={{ marginTop: 12 }}
                message={`${edit.domain} is not a public TLD`}
                description="Let's Encrypt production and staging will refuse this name. Use local HTTPS, or set a custom ACME server in the Let's Encrypt tab."
              />
            ) : null}
            <Space wrap style={{ marginTop: 12 }}>
              <Button loading={busy} onClick={issueSSL}>
                {edit.sslKind === "letsencrypt" ? "Renew Let's Encrypt" : "Issue Let's Encrypt"}
              </Button>
              {edit.sslKind !== "local" || !edit.ssl ? (
                <Button loading={busy} onClick={useLocalSSL}>
                  Use local HTTPS
                </Button>
              ) : null}
              {edit.ssl ? (
                <Button loading={busy} onClick={disableSSL}>
                  Disable HTTPS
                </Button>
              ) : null}
            </Space>
          </Form>
        ) : null}
      </Modal>

      <Modal
        title={renameSite ? `Rename ${renameSite.domain}` : "Rename website"}
        open={!!renameSite}
        onCancel={() => setRenameSite(null)}
        onOk={() => renameForm.submit()}
        confirmLoading={busy}
        destroyOnHidden
        okText="Rename"
      >
        <Form form={renameForm} layout="vertical" onFinish={saveRename} requiredMark={false} style={{ marginTop: 8 }}>
          <Form.Item
            name="domain"
            label="New domain"
            extra={
              renameSite
                ? `Moves domains/${renameSite.domain} if present. Let's Encrypt must be issued again after rename.`
                : undefined
            }
            rules={[{ required: true, message: "Domain required" }]}
          >
            <Input placeholder="site.test" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={runtimeSite ? `${kindLabel(runtimeSite.kind)} · ${runtimeSite.domain}` : "App"}
        open={!!runtimeSite}
        onCancel={() => {
          setRuntimeSite(null);
          setRuntime(null);
        }}
        footer={
          <Space wrap>
            <Button loading={runtimeBusy} onClick={() => void runRuntime("start")}>
              Start
            </Button>
            <Button loading={runtimeBusy} onClick={() => void runRuntime("restart")}>
              Restart
            </Button>
            <Button loading={runtimeBusy} onClick={() => void runRuntime("stop")}>
              Stop
            </Button>
            <Button loading={runtimeBusy} onClick={() => void runRuntime("logs")}>
              Logs
            </Button>
            <Button onClick={() => setRuntimeSite(null)}>Close</Button>
          </Space>
        }
        width={720}
        destroyOnHidden
      >
        {runtimeSite ? (
          <Space direction="vertical" style={{ width: "100%" }} size={12}>
            <Space wrap>
              <Tag color={runtime?.active ? "success" : "default"}>{runtime?.status || (runtimeBusy ? "…" : "unknown")}</Tag>
              {runtimeSite.appPort ? <Tag>PORT {runtimeSite.appPort}</Tag> : null}
              <Typography.Text type="secondary">{runtime?.unit || runtimeSite.appCmd}</Typography.Text>
            </Space>
            <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
              Command: <code>{runtime?.cmd || runtimeSite.appCmd || defaultAppCmd(runtimeSite.kind || "")}</code>
              {runtimeSite.proxyPass ? (
                <>
                  {" "}
                  · nginx → <code>{runtimeSite.proxyPass}</code>
                </>
              ) : null}
            </Typography.Paragraph>
            {runtime?.message ? <Alert type="warning" showIcon message={runtime.message} /> : null}
            <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
              Starter files are created in the document root if it is empty. Install the runtime from Software (Node.js, Python, Go, Rust, or Docker).
            </Typography.Paragraph>
            <pre
              style={{
                margin: 0,
                maxHeight: 320,
                overflow: "auto",
                padding: 12,
                background: "#0f172a",
                color: "#e2e8f0",
                borderRadius: 8,
                fontSize: 12,
              }}
            >
              {runtime?.log || (runtimeBusy ? "Loading…" : "No log yet")}
            </pre>
          </Space>
        ) : null}
      </Modal>

      <Modal
        title={logSite ? `Logs · ${logSite.domain}` : "Logs"}
        open={!!logSite}
        onCancel={() => setLogSite(null)}
        footer={<Button onClick={() => setLogSite(null)}>Close</Button>}
        width={1240}
        destroyOnHidden
        styles={{ body: { paddingTop: 12 } }}
      >
        {logSite ? <SiteLogs siteId={logSite.id} domain={logSite.domain} /> : null}
      </Modal>

      <Modal
        title={stats ? `GoAccess · ${stats.domain}` : "GoAccess"}
        open={!!stats}
        onCancel={() => setStats(null)}
        footer={
          <Space>
            <Button loading={statsBusy} disabled={!stats} onClick={() => stats && loadStats(stats, true)}>
              Rebuild
            </Button>
            <Button onClick={() => setStats(null)}>Close</Button>
          </Space>
        }
        width={1100}
        destroyOnHidden
      >
        {statsErr ? <Alert type="error" showIcon message={statsErr} style={{ marginBottom: 12 }} /> : null}
        {statsBusy && !statsSrc ? <Typography.Text type="secondary">Building report…</Typography.Text> : null}
        {statsSrc ? (
          <iframe
            title="GoAccess"
            src={statsSrc}
            style={{ width: "100%", height: "70vh", border: "1px solid #f0f0f0", borderRadius: 8, background: "#fff" }}
          />
        ) : null}
      </Modal>

      <Modal
        title={appSite ? `${appKind === "laravel" ? "Laravel" : "WordPress"} · ${appSite.domain}` : "App"}
        open={!!appSite}
        onCancel={() => {
          setAppSite(null);
          setAppKind(null);
        }}
        footer={<Button onClick={() => setAppSite(null)}>Close</Button>}
        width={1040}
        destroyOnHidden
      >
        {appBusy && !app ? <Typography.Text type="secondary">Loading…</Typography.Text> : null}
        {app && appKind === "laravel" && app.kind !== "laravel" ? (
          <Alert type="info" showIcon message="This document root is not a Laravel app (no artisan file). Point the site at public/ or the app root." />
        ) : null}
        {app && appKind === "wordpress" && app.kind !== "wordpress" ? (
          <Space direction="vertical" size={16} style={{ width: "100%" }}>
            <Alert type="info" showIcon message="No WordPress found. Fill the form to install core, create a database, and finish wp-admin in one step." />
            <Form
              form={wpForm}
              layout="vertical"
              initialValues={{ title: appSite?.domain, adminUser: randomWPLogin(), dbSuffix: "wp", url: appSite ? `http://${appSite.domain}` : "", prefix: randomWPPrefix() }}
              onFinish={async (v) => {
                if (!appSite) return;
                setAppBusy(true);
                try {
                  const out = await api.post<SiteApp>(`/api/sites/${appSite.id}/wordpress`, v);
                  setApp(out);
                  if (out.wordpress) {
                    setWpName(out.wordpress.name || "");
                    setWpUrl(out.wordpress.url || out.wordpress.home || "");
                    setWpPrefix(out.wordpress.prefix || "");
                  }
                  message.success("WordPress installed");
                } catch (err) {
                  message.error(err instanceof Error ? err.message : "Failed");
                } finally {
                  setAppBusy(false);
                }
              }}
            >
              <Form.Item name="title" label="Site title" rules={[{ required: true }]}>
                <Input />
              </Form.Item>
              <Form.Item name="url" label="Site URL">
                <Input />
              </Form.Item>
              <Form.Item label="Admin user" extra="Random username is used unless you change it. Avoid admin.">
                <Space.Compact style={{ width: "100%" }}>
                  <Form.Item name="adminUser" noStyle rules={[{ required: true, message: "Admin user required" }]}>
                    <Input />
                  </Form.Item>
                  <Button onClick={() => wpForm.setFieldValue("adminUser", randomWPLogin())}>Randomize</Button>
                </Space.Compact>
              </Form.Item>
              <Form.Item name="adminPass" label="Admin password" rules={[{ required: true, min: 8 }]}>
                <Input.Password />
              </Form.Item>
              <Form.Item name="adminEmail" label="Admin email">
                <Input />
              </Form.Item>
              <Form.Item name="dbSuffix" label="Database suffix" extra="Creates account_suffix and a matching DB user.">
                <Input />
              </Form.Item>
              <Form.Item label="Table prefix" extra="Random prefix is used unless you change it. Must start with a letter and end with _.">
                <Space.Compact style={{ width: "100%" }}>
                  <Form.Item name="prefix" noStyle rules={[{ required: true, message: "Prefix required" }]}>
                    <Input />
                  </Form.Item>
                  <Button onClick={() => wpForm.setFieldValue("prefix", randomWPPrefix())}>Randomize</Button>
                </Space.Compact>
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={appBusy}>
                Install WordPress
              </Button>
            </Form>
          </Space>
        ) : null}
        {appSite && app && appKind === "laravel" && app.kind === "laravel" ? (
          <Tabs
            items={[
              {
                key: "app",
                label: "App",
                children: (
                  <Space direction="vertical" size={16} style={{ width: "100%" }}>
                    <Typography.Text type="secondary">App root {app.appRoot}</Typography.Text>
                    {app.laravel ? (
                      <LaravelQueues
                        laravel={app.laravel}
                        busy={appBusy}
                        onToggle={(on, rows) => void runApp(appSite, { action: "queue", queue: on, queues: rows })}
                        onApply={(rows) => void runApp(appSite, { action: "queue", queue: true, queues: rows })}
                        onScheduler={(on) => void runApp(appSite, { action: "scheduler", scheduler: on })}
                      />
                    ) : null}
                    {app.output ? (
                      <pre style={{ maxHeight: 240, overflow: "auto", background: "#fafafa", padding: 12, borderRadius: 8 }}>{app.output}</pre>
                    ) : null}
                  </Space>
                ),
              },
              {
                key: "artisan",
                label: "Artisan",
                children: (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <ArtisanRun
                      disabled={!app.laravel?.hasArtisan}
                      busy={appBusy}
                      commands={app.laravel?.artisanCmds || []}
                      onRun={(command) => void runApp(appSite, { action: "artisan", artisan: command })}
                    />
                    {app.output ? (
                      <pre style={{ maxHeight: 320, overflow: "auto", background: "#fafafa", padding: 12, borderRadius: 8, whiteSpace: "pre-wrap" }}>{app.output}</pre>
                    ) : null}
                  </Space>
                ),
              },
              {
                key: "composer",
                label: (() => {
                  const n = (app.laravel?.composerPkgs || []).filter((p) => p.update).length;
                  return n ? `Composer (${n})` : "Composer";
                })(),
                children: (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <AppPackages
                      kind="composer"
                      disabled={!app.laravel?.hasComposer}
                      busy={appBusy}
                      packages={app.laravel?.composerPkgs || []}
                      onRun={(command) => void runApp(appSite, { action: "composer", composer: command })}
                      onSearch={(q) => searchPkgs("composer", q)}
                      onUpdate={(name) => void runApp(appSite, { action: "composer", composer: `update ${name}` })}
                    />
                    {app.output ? (
                      <pre style={{ maxHeight: 240, overflow: "auto", background: "#fafafa", padding: 12, borderRadius: 8 }}>{app.output}</pre>
                    ) : null}
                  </Space>
                ),
              },
              {
                key: "node",
                label: (() => {
                  const n = (app.laravel?.npmPkgs || []).filter((p) => p.update).length;
                  return n ? `Node (${n})` : "Node";
                })(),
                children: (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <AppPackages
                      kind="npm"
                      disabled={!app.laravel?.hasPackage}
                      busy={appBusy}
                      packages={app.laravel?.npmPkgs || []}
                      scripts={app.laravel?.npmScripts}
                      onRun={(command) => void runApp(appSite, { action: "npm", npm: command })}
                      onSearch={(q) => searchPkgs("npm", q)}
                      onUpdate={(name) => void runApp(appSite, { action: "npm", npm: `update ${name}` })}
                    />
                    {app.output ? (
                      <pre style={{ maxHeight: 240, overflow: "auto", background: "#fafafa", padding: 12, borderRadius: 8 }}>{app.output}</pre>
                    ) : null}
                  </Space>
                ),
              },
              {
                key: "env",
                label: ".env",
                children: (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <Input.TextArea
                      rows={16}
                      value={envText}
                      onChange={(e) => setEnvText(e.target.value)}
                      spellCheck={false}
                      style={{ fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" }}
                    />
                    <Button type="primary" loading={appBusy} onClick={() => void runApp(appSite, { action: "env-set", env: envText })}>
                      Save .env
                    </Button>
                  </Space>
                ),
              },
            ]}
          />
        ) : null}
        {appSite && app && appKind === "wordpress" && app.kind === "wordpress" ? (
          <Tabs
            items={[
              {
                key: "site",
                label: "Site",
                children: (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <Typography.Text type="secondary">
                      WordPress {app.wordpress?.version} · {app.appRoot}
                    </Typography.Text>
                    <Typography.Text>Site title</Typography.Text>
                    <Input value={wpName} onChange={(e) => setWpName(e.target.value)} />
                    <Typography.Text>Site URL</Typography.Text>
                    <Input value={wpUrl} onChange={(e) => setWpUrl(e.target.value)} placeholder="http://site.test:8080" />
                    <Button
                      type="primary"
                      loading={appBusy}
                      onClick={() => void runApp(appSite, { action: "settings", name: wpName, url: wpUrl })}
                    >
                      Save site settings
                    </Button>
                    <Typography.Text>Table prefix</Typography.Text>
                    <Typography.Text type="secondary">Current: {app.wordpress?.prefix || wpPrefix || "wp_"}</Typography.Text>
                    <Space.Compact style={{ width: "100%" }}>
                      <Input value={wpPrefix} onChange={(e) => setWpPrefix(e.target.value)} placeholder="wp_" />
                      <Button onClick={() => setWpPrefix(randomWPPrefix())}>Randomize</Button>
                      <Button
                        loading={appBusy}
                        disabled={!wpPrefix.trim()}
                        onClick={() => {
                          const next = wpPrefix.trim().endsWith("_") ? wpPrefix.trim() : `${wpPrefix.trim()}_`;
                          modal.confirm({
                            title: `Change table prefix to ${next}?`,
                            content: `Renames all ${app.wordpress?.prefix || "wp_"} tables and updates wp-config.php. Back up the database first.`,
                            okText: "Change prefix",
                            onOk: () => runApp(appSite, { action: "prefix", prefix: next }),
                          });
                        }}
                      >
                        Save prefix
                      </Button>
                    </Space.Compact>
                    {app.output ? (
                      <pre style={{ maxHeight: 240, overflow: "auto", background: "#fafafa", padding: 12, borderRadius: 8, whiteSpace: "pre-wrap" }}>{app.output}</pre>
                    ) : null}
                  </Space>
                ),
              },
              {
                key: "wpcli",
                label: "WP-CLI",
                children: (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <ArtisanRun
                      kind="wp"
                      busy={appBusy}
                      commands={app.wordpress?.wpCmds || []}
                      onRun={(command) => void runApp(appSite, { action: "wpcli", wpcli: command })}
                    />
                    {app.output ? (
                      <pre style={{ maxHeight: 320, overflow: "auto", background: "#fafafa", padding: 12, borderRadius: 8, whiteSpace: "pre-wrap" }}>{app.output}</pre>
                    ) : null}
                  </Space>
                ),
              },
              {
                key: "users",
                label: "Users",
                children: (
                  <Table
                    rowKey="id"
                    size="small"
                    pagination={false}
                    dataSource={app.wordpress?.users || []}
                    columns={[
                      { title: "Login", dataIndex: "login" },
                      { title: "Name", dataIndex: "name" },
                      { title: "Email", dataIndex: "email" },
                      { title: "Roles", dataIndex: "roles" },
                      {
                        title: "",
                        width: 220,
                        render: (_, u) => (
                          <Space>
                            <Button size="small" loading={appBusy} onClick={() => void runApp(appSite, { action: "login", userId: u.id })}>
                              Login as
                            </Button>
                            <Button
                              size="small"
                              loading={appBusy}
                              onClick={() => {
                                const next = randomWPLogin();
                                modal.confirm({
                                  title: `Rename ${u.login} to ${next}?`,
                                  content: "Existing wp-admin logins must use the new username. Password is unchanged.",
                                  okText: "Rename",
                                  onOk: () => runApp(appSite, { action: "rename-user", userId: u.id, login: next }),
                                });
                              }}
                            >
                              {u.login.toLowerCase() === "admin" ? "Randomize admin" : "Randomize login"}
                            </Button>
                          </Space>
                        ),
                      },
                    ]}
                  />
                ),
              },
              {
                key: "plugins",
                label: "Plugins",
                children: (
                  <Table
                    rowKey="name"
                    size="small"
                    pagination={false}
                    dataSource={app.wordpress?.plugins || []}
                    columns={[
                      { title: "Plugin", render: (_, p) => p.title || p.name },
                      { title: "Version", dataIndex: "version", width: 90 },
                      {
                        title: "Status",
                        width: 110,
                        render: (_, p) => <Tag color={p.status === "active" ? "success" : "default"}>{p.status}</Tag>,
                      },
                      {
                        title: "",
                        width: 210,
                        render: (_, p) => (
                          <Space>
                            {p.status === "active" ? (
                              <Button size="small" loading={appBusy} onClick={() => void runApp(appSite, { action: "plugin-off", plugin: p.name })}>
                                Disable
                              </Button>
                            ) : (
                              <Button size="small" loading={appBusy} onClick={() => void runApp(appSite, { action: "plugin-on", plugin: p.name })}>
                                Enable
                              </Button>
                            )}
                            <Button
                              size="small"
                              danger
                              loading={appBusy}
                              onClick={() =>
                                modal.confirm({
                                  title: `Uninstall ${p.title || p.name}?`,
                                  okText: "Uninstall",
                                  okButtonProps: { danger: true },
                                  onOk: () => runApp(appSite, { action: "plugin-uninstall", plugin: p.name }),
                                })
                              }
                            >
                              Uninstall
                            </Button>
                          </Space>
                        ),
                      },
                    ]}
                  />
                ),
              },
              {
                key: "updates",
                label: "Updates",
                children: (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <Space wrap>
                      <Button loading={appBusy} onClick={() => void runApp(appSite, { action: "updates" })}>
                        Check updates
                      </Button>
                      <Button loading={appBusy} onClick={() => void runApp(appSite, { action: "update-core" })}>
                        Update core
                      </Button>
                      <Button loading={appBusy} onClick={() => void runApp(appSite, { action: "update-plugins" })}>
                        Update plugins
                      </Button>
                    </Space>
                    {app.wordpress?.updates ? (
                      <>
                        <Typography.Text>Core: {app.wordpress.updates.core || "up to date"}</Typography.Text>
                        {(app.wordpress.updates.plugins || []).map((p) => (
                          <div key={p.name}>
                            {p.title || p.name} {p.version} → {p.update}
                          </div>
                        ))}
                        {(app.wordpress.updates.themes || []).map((t) => (
                          <div key={t}>Theme {t}</div>
                        ))}
                      </>
                    ) : (
                      <Typography.Text type="secondary">Click Check updates.</Typography.Text>
                    )}
                    {app.output ? (
                      <pre style={{ maxHeight: 200, overflow: "auto", background: "#fafafa", padding: 12, borderRadius: 8 }}>{app.output}</pre>
                    ) : null}
                  </Space>
                ),
              },
              {
                key: "security",
                label: "Security",
                children: (
                  <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <Space wrap align="center">
                      <Switch
                        checked={!!app.wordpress?.xmlrpcBlocked}
                        loading={appBusy}
                        onChange={(on) => void runApp(appSite, { action: "xmlrpc", xmlrpc: on })}
                      />
                      <Typography.Text>Block xmlrpc.php</Typography.Text>
                    </Space>
                    <Typography.Text type="secondary">Stops XML-RPC brute force and the WordPress mobile app endpoint.</Typography.Text>
                    <Space wrap align="center">
                      <Switch
                        checked={!!app.wordpress?.pingbacksOff}
                        loading={appBusy}
                        onChange={(on) => void runApp(appSite, { action: "pingbacks", pingbacks: on })}
                      />
                      <Typography.Text>Block pingbacks</Typography.Text>
                    </Space>
                    <Typography.Text type="secondary">Closes pingbacks/trackbacks on posts and removes the X-Pingback header.</Typography.Text>
                    <Space wrap align="center">
                      <Switch
                        checked={!!app.wordpress?.uploadsPhpBlocked}
                        loading={appBusy}
                        onChange={(on) => void runApp(appSite, { action: "uploads-php", uploadsPhp: on })}
                      />
                      <Typography.Text>Block PHP in uploads folder</Typography.Text>
                    </Space>
                    <Typography.Text type="secondary">
                      Returns 403 for PHP under wp-content/uploads and rejects PHP file uploads in WordPress.
                    </Typography.Text>
                    <Button loading={appBusy} onClick={() => void runApp(appSite, { action: "security" })}>
                      Analyze
                    </Button>
                    <Table
                      rowKey="id"
                      size="small"
                      pagination={false}
                      dataSource={app.wordpress?.security || []}
                      columns={[
                        {
                          title: "Level",
                          width: 90,
                          render: (_, i) => (
                            <Tag color={i.level === "ok" ? "success" : i.level === "warn" ? "warning" : "default"}>{i.level}</Tag>
                          ),
                        },
                        { title: "Issue", dataIndex: "title" },
                        { title: "Detail", dataIndex: "detail" },
                      ]}
                    />
                  </Space>
                ),
              },
            ]}
          />
        ) : null}
      </Modal>

      <Modal
        title="WordPress login as"
        open={loginUrls.length > 0}
        onCancel={() => setLoginUrls([])}
        footer={<Button onClick={() => setLoginUrls([])}>Close</Button>}
      >
        <Typography.Paragraph>The link works once and expires in 2 minutes. Use the port that matches how you open the site.</Typography.Paragraph>
        <Space direction="vertical">
          {loginUrls.map((u) => (
            <Typography.Link key={u} href={u} target="_blank" rel="noreferrer">
              {u}
            </Typography.Link>
          ))}
        </Space>
      </Modal>
    </div>
  );
}
