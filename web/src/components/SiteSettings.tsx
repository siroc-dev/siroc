import { useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Col, Descriptions, Input, InputNumber, Popconfirm, Row, Select, Space, Switch, Table, Tabs, Tag, Typography } from "antd";
import { matchNginxRewrite, nginxRewriteBody, NGINX_REWRITE_OPTIONS } from "@/lib/nginxRewrites";
import { ProxySettings, type ProxySettingsValue } from "@/components/ProxySettings";
import { cacheDir, cacheFromSaved, cachePayload, cacheSizeBytes, cacheSizeToken, NginxCacheForm, type NginxCacheValue } from "@/components/NginxCache";
import { AppPackages, type PkgHit, type PkgRow } from "@/components/AppPackages";
import { api, RequestError } from "@/lib/api";
import { formatPanelTime } from "@/lib/panelTime";
import { formatBytes } from "@/lib/usage";

export type SiteOptions = {
  index?: string[];
  access?: "" | "allow" | "deny";
  ips?: string[];
  hotlink?: boolean;
  hotlinkDomains?: string[];
  maintenance?: boolean;
  redirects?: { from: string; to: string; code: number }[];
  proxy?: ProxySettingsValue;
  static?: boolean;
  staticExt?: string;
  forceHttps?: boolean;
  accessLog?: boolean;
  errorLog?: boolean;
  php?: SitePHP;
  composerRoot?: string;
  cache?: {
    on?: boolean;
    enabled?: boolean;
    valid?: string;
    path?: string;
    preset?: string;
    size?: string;
    idle?: string;
    stale?: boolean;
    refresh?: boolean;
    honor?: boolean;
    cookies?: string;
    skip?: string;
  };
};

export type SitePHP = {
  memoryLimit?: string;
  maxExecutionTime?: number;
  maxInputTime?: number;
  postMaxSize?: string;
  uploadMaxFilesize?: string;
  timezone?: string;
  displayErrors?: boolean;
  disableFunctions?: string;
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
  aliasAdded?: string[];
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
  brand?: string;
  cert?: string;
  key?: string;
  message?: string;
};

function certTypeLabel(site: SiteSettingsSite) {
  if (site.sslKind === "letsencrypt") return "Let's Encrypt";
  if (site.sslKind === "local") return "Local certificate";
  return "Other certificate";
}

function httpsForced(site: SiteSettingsSite) {
  return site.options?.forceHttps === true;
}

function expDay(v?: string) {
  const when = formatPanelTime(v);
  return when ? when.slice(0, 10) : "—";
}

function expPhrase(v?: string) {
  if (!v) return "";
  const end = Date.parse(v);
  if (!Number.isFinite(end)) return "";
  const days = Math.ceil((end - Date.now()) / 86400000);
  if (days < 0) return `Expired ${Math.abs(days)} days ago`;
  if (days === 0) return "Expires today";
  return `Exp in ${days} days`;
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
  { key: "domain", label: "Domains" },
  { key: "directory", label: "Directory" },
  { key: "access", label: "Access" },
  { key: "rewrite", label: "Rewrite" },
  { key: "index", label: "Index files" },
  { key: "config", label: "Config" },
  { key: "ssl", label: "HTTPS" },
  { key: "php", label: "PHP" },
  { key: "server", label: "Web server" },
  { key: "git", label: "Git" },
  { key: "composer", label: "Composer" },
  { key: "redirect", label: "Redirects" },
  { key: "proxy", label: "Runtime" },
  { key: "hotlink", label: "Hotlink" },
  { key: "maintenance", label: "Maintenance" },
  { key: "logs", label: "Logs" },
];

export function isSiteSection(value: string | null): value is SiteSection {
  return !!value && NAV.some((item) => item.key === value);
}

const PHP_ZONES = ["UTC", "Asia/Bangkok", "Asia/Tokyo", "Asia/Singapore", "Europe/London", "Europe/Berlin", "America/New_York", "America/Los_Angeles"];

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
  return formatPanelTime(raw);
}

function baseOpts(site: SiteSettingsSite): SiteOptions {
  const o = site.options || {};
  return {
    index: o.index || [],
    access: o.access || "",
    ips: o.ips || [],
    hotlink: !!o.hotlink,
    hotlinkDomains: o.hotlinkDomains || [],
    maintenance: !!o.maintenance,
    redirects: o.redirects || [],
    proxy: o.proxy,
    static: o.static,
    staticExt: o.staticExt,
    forceHttps: o.forceHttps,
    accessLog: o.accessLog,
    errorLog: o.errorLog,
    php: o.php,
    composerRoot: o.composerRoot,
    cache: o.cache,
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
  onSection,
}: {
  site: SiteSettingsSite;
  section: SiteSection;
  busy: boolean;
  phpVersions: { value: string; label: string }[];
  publicName: boolean;
  onClose: () => void;
  onPatch: (body: Record<string, unknown>, ok: string) => Promise<{ aliases?: string[]; skipped?: string[] } | void>;
  onIssueSSL: () => void;
  sslLog?: string;
  onLocalSSL: () => void;
  onDisableSSL: () => void;
  onCustomSSL?: (cert: string, key: string) => Promise<void>;
  onGit: () => void;
  onLogs: () => void;
  onSSH: () => void;
  onLaravel: () => void;
  onSection?: (section: SiteSection) => void;
}) {
  const [tab, setTab] = useState<SiteSection>(section);
  const [draft, setDraft] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const [domainPage, setDomainPage] = useState(1);
  const [domainPageSize, setDomainPageSize] = useState(20);
  const [docRoot, setDocRoot] = useState("");
  const [rewrite, setRewrite] = useState("");
  const [php, setPhp] = useState("");
  const [phpMem, setPhpMem] = useState("");
  const [phpExec, setPhpExec] = useState<number | null>(null);
  const [phpInput, setPhpInput] = useState<number | null>(null);
  const [phpPost, setPhpPost] = useState("");
  const [phpUpload, setPhpUpload] = useState("");
  const [phpZone, setPhpZone] = useState("");
  const [phpErrors, setPhpErrors] = useState<"" | "on" | "off">("");
  const [phpDisable, setPhpDisable] = useState("");
  const [phpAccount, setPhpAccount] = useState<SitePHP>({});
  const [composerPath, setComposerPath] = useState("");
  const [composerDir, setComposerDir] = useState("");
  const [composerPkgs, setComposerPkgs] = useState<PkgRow[]>([]);
  const [hasComposer, setHasComposer] = useState(false);
  const [composerOut, setComposerOut] = useState("");
  const [composerBusy, setComposerBusy] = useState(false);
  const [sslInfo, setSslInfo] = useState<CertInfo | null>(null);
  const [certPEM, setCertPEM] = useState("");
  const [keyPEM, setKeyPEM] = useState("");
  const [sslTab, setSslTab] = useState<"current" | "le" | "holder">("current");
  const [kind, setKind] = useState("php");
  const [proxyPass, setProxyPass] = useState("");
  const [appCmd, setAppCmd] = useState("");
  const [appPort, setAppPort] = useState("");
  const [cache, setCache] = useState<NginxCacheValue>(() => cacheFromSaved(undefined, ""));
  const [cacheMax, setCacheMax] = useState(0);
  const [cacheBusy, setCacheBusy] = useState(false);
  const [access, setAccess] = useState<"" | "allow" | "deny">("");
  const [ips, setIps] = useState("");
  const [index, setIndex] = useState("");
  const [hotlink, setHotlink] = useState(false);
  const [hotlinkExtra, setHotlinkExtra] = useState<string[]>([]);
  const [hotlinkDraft, setHotlinkDraft] = useState("");
  const [maintenance, setMaintenance] = useState(false);
  const [staticOn, setStaticOn] = useState(true);
  const [staticExt, setStaticExt] = useState(DEFAULT_STATIC_EXT);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [code, setCode] = useState(301);
  const [redirects, setRedirects] = useState<{ from: string; to: string; code: number }[]>([]);
  const [nginxConf, setNginxConf] = useState("");
  const [apacheConf, setApacheConf] = useState("");
  const [cfgCustom, setCfgCustom] = useState(false);
  const [cfgLog, setCfgLog] = useState("");
  const [cfgBusy, setCfgBusy] = useState("");
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
        if (stop) return;
        setSslInfo(info);
        setCertPEM(info.cert || "");
        setKeyPEM(info.key || "");
      })
      .catch(() => {
        if (!stop) setSslInfo(null);
      });
    return () => {
      stop = true;
    };
  }, [tab, site.id, site.ssl, site.sslKind]);

  useEffect(() => {
    if (tab !== "config") return;
    let stop = false;
    api
      .get<{ nginx?: string; apache?: string; custom?: boolean }>(`/api/sites/${site.id}/config`)
      .then((out) => {
        if (stop) return;
        setNginxConf(out.nginx || "");
        setApacheConf(out.apache || "");
        setCfgCustom(!!out.custom);
        setCfgLog("");
      })
      .catch((err: unknown) => {
        if (!stop) message.error(err instanceof Error ? err.message : "Could not read the config");
      });
    return () => {
      stop = true;
    };
  }, [tab, site.id, message]);

  useEffect(() => {
    const phpHere = !APP.has(kind) && kind !== "proxy" && kind !== "nginx";
    if (tab !== "php" || !phpHere) return;
    let stop = false;
    api
      .get<{ settings?: SitePHP }>(`/api/accounts/${encodeURIComponent(site.username)}/php`)
      .then((out) => {
        if (!stop) setPhpAccount(out.settings || {});
      })
      .catch(() => {
        if (!stop) setPhpAccount({});
      });
    return () => {
      stop = true;
    };
  }, [tab, site.username, kind]);

  useEffect(() => {
    if (tab !== "composer") return;
    let stop = false;
    setComposerBusy(true);
    api
      .post<{ composerRoot?: string; laravel?: { hasComposer?: boolean; composerPkgs?: PkgRow[] }; output?: string }>(`/api/sites/${site.id}/app`, { action: "packages" })
      .then((out) => {
        if (stop) return;
        setComposerDir(out.composerRoot || "");
        setHasComposer(!!out.laravel?.hasComposer);
        setComposerPkgs(out.laravel?.composerPkgs || []);
      })
      .catch((err: unknown) => {
        if (!stop) message.error(err instanceof Error ? err.message : "Could not read composer.json");
      })
      .finally(() => {
        if (!stop) setComposerBusy(false);
      });
    return () => {
      stop = true;
    };
  }, [tab, site.id, site.options?.composerRoot, message]);

  useEffect(() => {
    if (tab !== "server") return;
    const path = cache.path.trim();
    let stop = false;
    const timer = window.setTimeout(() => {
      const q = path ? `?path=${encodeURIComponent(path)}` : "";
      api
        .get<{ total?: number; max?: number }>(`/api/sites/${site.id}/cache-disk${q}`)
        .then((row) => {
          if (!stop) setCacheMax(row?.max || 0);
        })
        .catch(() => {
          if (!stop) setCacheMax(0);
        });
    }, 300);
    return () => {
      stop = true;
      window.clearTimeout(timer);
    };
  }, [tab, site.id, cache.path]);

  useEffect(() => {
    const o = baseOpts(site);
    setDocRoot(toRel(site.username, site.docRoot));
    setRewrite(site.rewrite || "");
    setPhp(site.phpVersion);
    const phpOpt = o.php || {};
    setPhpMem(phpOpt.memoryLimit || "");
    setPhpExec(phpOpt.maxExecutionTime || null);
    setPhpInput(phpOpt.maxInputTime || null);
    setPhpPost(phpOpt.postMaxSize || "");
    setPhpUpload(phpOpt.uploadMaxFilesize || "");
    setPhpZone(phpOpt.timezone || "");
    setPhpErrors(phpOpt.displayErrors === true ? "on" : phpOpt.displayErrors === false ? "off" : "");
    setPhpDisable(phpOpt.disableFunctions || "");
    setComposerPath(o.composerRoot || "");
    setKind(site.kind || "php");
    setProxyPass(site.proxyPass || "");
    setAppCmd(site.appCmd || "");
    setAppPort(site.appPort ? String(site.appPort) : "");
    setCache(cacheFromSaved(o.cache, site.domain));
    setAccess(o.access || "");
    setIps((o.ips || []).join("\n"));
    setIndex((o.index || []).join(" "));
    setHotlink(!!o.hotlink);
    setHotlinkExtra(o.hotlinkDomains || []);
    setHotlinkDraft("");
    setMaintenance(!!o.maintenance);
    setStaticOn(o.static !== false);
    setStaticExt(o.staticExt || DEFAULT_STATIC_EXT);
    setRedirects(o.redirects || []);
    setDraft("");
    setPicked([]);
  }, [site]);

  const when = addedAt(site.createdAt);
  const label = site.displayName || site.domain;
  const meta = [when ? `Added ${when}` : "", label !== site.domain ? site.domain : ""].filter(Boolean).join(" · ");
  const port = site.ssl ? "80, 443" : "80";
  const rows = useMemo(
    () => [
      { name: site.domain, label: site.displayName || site.domain, primary: true, added: addedAt(site.createdAt) },
      ...(site.aliases || []).map((name, i) => ({ name, label: site.displayAliases?.[i] || name, primary: false, added: addedAt(site.aliasAdded?.[i]) })),
    ],
    [site.domain, site.displayName, site.aliases, site.displayAliases, site.aliasAdded, site.createdAt],
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
    const existing = new Set([site.domain, ...(site.aliases || [])]);
    const fresh: string[] = [];
    let skipped = 0;
    for (const line of lines) {
      if (existing.has(line) || fresh.includes(line)) {
        skipped++;
        continue;
      }
      fresh.push(line);
    }
    if (!fresh.length) {
      if (skipped > 0) message.info(skipped === 1 ? "1 duplicate domain skipped" : `${skipped} duplicate domains skipped`);
      setDraft("");
      return;
    }
    const next = [...(site.aliases || []), ...fresh];
    const saved = await onPatch({ aliases: next }, "Domains added");
    if (!saved) return;
    skipped += saved.skipped?.length || 0;
    if (skipped > 0) message.info(skipped === 1 ? "1 duplicate domain skipped" : `${skipped} duplicate domains skipped`);
    setDraft("");
  }

  async function deletePicked() {
    const drop = new Set(picked);
    const next = (site.aliases || []).filter((a) => !drop.has(a));
    const saved = await onPatch({ aliases: next }, picked.length === 1 ? "Alias removed" : "Aliases removed");
    if (saved) setPicked([]);
  }

  function saveOpts(partial: SiteOptions, ok: string) {
    return onPatch({ options: { ...baseOpts(site), ...partial } }, ok);
  }

  async function purgeCache() {
    setCacheBusy(true);
    try {
      await api.post(`/api/sites/${site.id}/cache/purge`);
      message.success("Cache cleared");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Could not clear the cache");
    } finally {
      setCacheBusy(false);
    }
  }

  function addHotlink() {
    let name = hotlinkDraft.trim().toLowerCase();
    name = name.replace(/^https?:\/\//, "").split(/[/?#]/)[0].replace(/\.$/, "");
    if (!name) return;
    const owned = new Set([site.domain, ...(site.aliases || [])].map((item) => item.toLowerCase()));
    if (owned.has(name) || hotlinkExtra.some((item) => item.toLowerCase() === name)) {
      setHotlinkDraft("");
      return;
    }
    setHotlinkExtra((list) => [...list, name]);
    setHotlinkDraft("");
  }

  function savePHP() {
    const phpOpt: SitePHP = {};
    if (phpMem.trim()) phpOpt.memoryLimit = phpMem.trim();
    if (phpExec) phpOpt.maxExecutionTime = phpExec;
    if (phpInput) phpOpt.maxInputTime = phpInput;
    if (phpPost.trim()) phpOpt.postMaxSize = phpPost.trim();
    if (phpUpload.trim()) phpOpt.uploadMaxFilesize = phpUpload.trim();
    if (phpZone.trim()) phpOpt.timezone = phpZone.trim();
    if (phpErrors === "on") phpOpt.displayErrors = true;
    if (phpErrors === "off") phpOpt.displayErrors = false;
    if (phpDisable.trim()) phpOpt.disableFunctions = phpDisable.trim();
    return onPatch({ phpVersion: php, options: { ...baseOpts(site), php: Object.keys(phpOpt).length ? phpOpt : undefined } }, "PHP saved");
  }

  async function runComposer(command: string) {
    setComposerBusy(true);
    setComposerOut("");
    try {
      const out = await api.post<{ composerRoot?: string; output?: string; laravel?: { hasComposer?: boolean; composerPkgs?: PkgRow[] } }>(`/api/sites/${site.id}/app`, {
        action: "composer",
        composer: command,
      });
      setComposerOut(out.output || "");
      setHasComposer(!!out.laravel?.hasComposer);
      setComposerPkgs(out.laravel?.composerPkgs || []);
      if (out.composerRoot) setComposerDir(out.composerRoot);
      message.success("Composer finished");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Composer failed");
    } finally {
      setComposerBusy(false);
    }
  }

  async function searchComposer(query: string): Promise<PkgHit[]> {
    const out = await api.post<{ pkgHits?: PkgHit[] }>(`/api/sites/${site.id}/app`, { action: "pkg-search", name: "composer", composer: query });
    return out.pkgHits || [];
  }

  async function saveConfig() {
    setCfgBusy("save");
    setCfgLog("");
    try {
      const out = await api.post<{ log?: string; nginx?: string; apache?: string }>(`/api/sites/${site.id}/config`, { nginx: nginxConf, apache: apacheConf });
      const nextNginx = out.nginx ?? nginxConf;
      const nextApache = out.apache ?? apacheConf;
      setNginxConf(nextNginx);
      setApacheConf(nextApache);
      setCfgCustom(true);
      setCfgLog(out.log || "");
      message.success("Config tested and saved");
    } catch (err: unknown) {
      setCfgLog(err instanceof RequestError ? err.log || "" : "");
      message.error(err instanceof Error ? err.message : "Config test failed");
    } finally {
      setCfgBusy("");
    }
  }

  async function resetConfig() {
    setCfgBusy("reset");
    try {
      await api.post(`/api/sites/${site.id}/config/reset`);
      const out = await api.get<{ nginx?: string; apache?: string }>(`/api/sites/${site.id}/config`);
      setNginxConf(out.nginx || "");
      setApacheConf(out.apache || "");
      setCfgCustom(false);
      setCfgLog("");
      message.success("Generated config restored");
    } catch (err: unknown) {
      message.error(err instanceof Error ? err.message : "Could not reset the config");
    } finally {
      setCfgBusy("");
    }
  }

  function installPasted() {
    if (!onCustomSSL) return;
    return onCustomSSL(certPEM, keyPEM).catch(() => undefined);
  }

  function certEditors() {
    return (
      <div className="ssl-pair">
        <label>
          Private key (KEY)
          <textarea
            value={keyPEM}
            spellCheck={false}
            placeholder={"-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"}
            onChange={(e) => setKeyPEM(e.target.value)}
          />
        </label>
        <label>
          Certificate (CRT/PEM)
          <textarea
            value={certPEM}
            spellCheck={false}
            placeholder={"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"}
            onChange={(e) => setCertPEM(e.target.value)}
          />
        </label>
      </div>
    );
  }

  const phpSite = !APP.has(kind) && kind !== "proxy" && kind !== "nginx";

  return (
    <div className="site-mod-page">
      <div className="site-mod-head">
        <Button onClick={onClose}>Back</Button>
        <div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            {label}
          </Typography.Title>
          {meta ? (
            <Typography.Paragraph type="secondary" className="site-mod-meta">
              {meta}
            </Typography.Paragraph>
          ) : null}
        </div>
      </div>
      <div className="site-mod-pills" role="tablist">
        {NAV.map((item) => (
          <button
            key={item.key}
            type="button"
            role="tab"
            aria-selected={tab === item.key}
            className={tab === item.key ? "active" : ""}
            onClick={() => {
              setTab(item.key);
              onSection?.(item.key);
            }}
          >
            {item.label}
          </button>
        ))}
      </div>
      <div className="site-mod-pane">
          {tab === "domain" ? (
            <>
              <Typography.Paragraph type="secondary">
                One domain per line, up to 500 aliases. Duplicate names are skipped. The site listens on ports 80 and 443. A Let's Encrypt certificate covers the primary name and the first 99 aliases. A wildcard looks like *.example.com. Rename the site to change the primary name.
              </Typography.Paragraph>
              <Input.TextArea rows={3} value={draft} placeholder={"www.example.com\n*.example.com"} onChange={(e) => setDraft(e.target.value)} />
              <Space wrap style={{ marginTop: 12, marginBottom: 16 }}>
                <Button type="primary" loading={busy} onClick={() => void addDomains()}>
                  Add names
                </Button>
                <Popconfirm
                  title={picked.length === 1 ? "Remove this alias?" : `Remove ${picked.length} aliases?`}
                  description="The primary name stays on the site."
                  okText="Remove"
                  okButtonProps={{ danger: true }}
                  disabled={!picked.length}
                  onConfirm={() => void deletePicked()}
                >
                  <Button danger disabled={!picked.length} loading={busy}>
                    {picked.length ? `Remove ${picked.length} selected` : "Remove selected"}
                  </Button>
                </Popconfirm>
              </Space>
              <Table
                size="small"
                rowKey="name"
                pagination={{
                  current: domainPage,
                  pageSize: domainPageSize,
                  showSizeChanger: true,
                  pageSizeOptions: ["10", "20", "50", "100"],
                  showTotal: (n) => `Total ${n}`,
                  hideOnSinglePage: true,
                  onChange: (page, size) => {
                    setDomainPage(page);
                    setDomainPageSize(size);
                  },
                }}
                dataSource={rows}
                rowSelection={{
                  selectedRowKeys: picked,
                  preserveSelectedRowKeys: true,
                  onChange: (keys) => setPicked(keys.map(String)),
                  getCheckboxProps: (row) => ({ disabled: row.primary }),
                }}
                columns={[
                  {
                    title: "Domain name",
                    dataIndex: "label",
                    render: (label: string, row: { name: string; primary: boolean }) => (
                      <span>
                        {label} {row.primary ? <Tag>Primary</Tag> : null}
                        {label !== row.name ? <div className="site-list-puny">{row.name}</div> : null}
                      </span>
                    ),
                  },
                  { title: "Added", dataIndex: "added", width: 170, render: (v: string) => v || "—" },
                  { title: "Port", width: 110, render: () => port },
                  {
                    title: "",
                    width: 120,
                    render: (_, row) =>
                      row.primary ? null : (
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
                message={cfgCustom ? "Custom configuration" : "Live configuration"}
                description={
                  cfgCustom
                    ? "These files are used as written. Save tests them first and keeps the edit only when the test passes. Other site settings leave them alone until you reset."
                    : "Edit the files on disk. Save tests them first and writes them only when the test passes. A saved file is kept until you reset it."
                }
              />
              <div className="site-mod-config-grid">
                <label className="site-mod-config-col">
                  <Typography.Text type="secondary">Nginx</Typography.Text>
                  <textarea className="site-mod-config" value={nginxConf} spellCheck={false} onChange={(e) => setNginxConf(e.target.value)} />
                </label>
                <label className="site-mod-config-col">
                  <Typography.Text type="secondary">Apache</Typography.Text>
                  <textarea className="site-mod-config" value={apacheConf} spellCheck={false} onChange={(e) => setApacheConf(e.target.value)} />
                </label>
              </div>
              <Typography.Paragraph type="secondary">Leave Apache empty to keep the current Apache file.</Typography.Paragraph>
              <Space wrap>
                <Button type="primary" loading={cfgBusy === "save"} disabled={cfgBusy !== "" && cfgBusy !== "save"} onClick={() => void saveConfig()}>
                  Save
                </Button>
                <Button loading={cfgBusy === "reset"} disabled={cfgBusy !== "" && cfgBusy !== "reset"} onClick={() => void resetConfig()}>
                  Reset
                </Button>
              </Space>
              {cfgLog ? <pre className="site-mod-config">{cfgLog}</pre> : null}
            </>
          ) : null}

          {tab === "ssl" ? (
            <>
              <Tabs
                className="ssl-switch"
                activeKey={sslTab}
                onChange={(key) => setSslTab(key as "current" | "le" | "holder")}
                items={[
                  { key: "current", label: "Certificate" },
                  { key: "le", label: "Let's Encrypt" },
                  { key: "holder", label: "Paste a certificate" },
                ]}
              />
              {sslTab === "current" ? (
                <>
                  {site.ssl && sslInfo?.ok ? (
                    <Descriptions size="small" column={1} style={{ marginBottom: 16 }}>
                      <Descriptions.Item label="Type">{certTypeLabel(site)}</Descriptions.Item>
                      <Descriptions.Item label="Issuer">{sslInfo.brand || sslInfo.issuer || "—"}</Descriptions.Item>
                      <Descriptions.Item label="Names">{(sslInfo.dnsNames || []).join(", ") || "—"}</Descriptions.Item>
                      <Descriptions.Item label="Expires">
                        {expDay(sslInfo.notAfter)}
                        {expPhrase(sslInfo.notAfter) ? ` · ${expPhrase(sslInfo.notAfter)}` : ""}
                      </Descriptions.Item>
                    </Descriptions>
                  ) : (
                    <Typography.Paragraph type="secondary">{sslInfo?.message || "No certificate is deployed."}</Typography.Paragraph>
                  )}
                  {site.ssl && sslInfo?.ok ? (
                    <div className="ssl-force">
                      <span>Redirect HTTP to HTTPS</span>
                      <Switch checked={httpsForced(site)} loading={busy} onChange={(on) => void saveOpts({ forceHttps: on }, "HTTPS redirect saved")} />
                    </div>
                  ) : null}
                  {site.ssl && sslInfo?.ok ? (
                    <Typography.Paragraph type="secondary">HTTP stays on HTTP until this is on. A new site uses the local certificate.</Typography.Paragraph>
                  ) : null}
                  {certEditors()}
                  <Space wrap>
                    <Button type="primary" loading={busy} disabled={!certPEM.trim() || !keyPEM.trim()} onClick={() => void installPasted()}>
                      Save certificate
                    </Button>
                    {site.ssl ? (
                      <Button loading={busy} onClick={onDisableSSL}>
                        Disable SSL
                      </Button>
                    ) : null}
                  </Space>
                  <Typography.Paragraph type="secondary">
                    Paste the private key and the certificate. If the browser reports an incomplete chain, put the intermediate certificates after the leaf certificate in the PEM box.
                  </Typography.Paragraph>
                </>
              ) : null}
              {sslTab === "le" ? (
                <>
                  {!publicName ? (
                    <Alert
                      type="warning"
                      showIcon
                      style={{ marginBottom: 12 }}
                      message={`${label} is not a public TLD`}
                      description="Let's Encrypt will refuse this name. Use local HTTPS, paste a certificate, or set a custom ACME server."
                    />
                  ) : (
                    <Typography.Paragraph type="secondary">Issue a certificate for this site. A renewal uses the same button.</Typography.Paragraph>
                  )}
                  <Space wrap>
                    <Button type="primary" loading={busy} onClick={onIssueSSL}>
                      {site.sslKind === "letsencrypt" ? "Renew Let's Encrypt" : "Issue Let's Encrypt"}
                    </Button>
                    {site.sslKind !== "local" || !site.ssl ? (
                      <Button loading={busy} onClick={onLocalSSL}>
                        Use local HTTPS
                      </Button>
                    ) : null}
                  </Space>
                  {sslLog ? <pre className="cmd-out">{sslLog}</pre> : null}
                </>
              ) : null}
              {sslTab === "holder" ? (
                <>
                  <Typography.Paragraph type="secondary">
                    Paste a certificate you already hold. Saving it deploys that certificate on this site.
                  </Typography.Paragraph>
                  {certEditors()}
                  <Button type="primary" loading={busy} disabled={!certPEM.trim() || !keyPEM.trim()} onClick={() => void installPasted()}>
                    Save certificate
                  </Button>
                </>
              ) : null}
            </>
          ) : null}

          {tab === "php" ? (
            phpSite ? (
              <>
                <Typography.Paragraph type="secondary">Blank fields keep this account's PHP settings. Values here apply only to this site.</Typography.Paragraph>
                <Row gutter={[16, 16]}>
                  <Col xs={24} md={8}>
                    <Typography.Text type="secondary">PHP version</Typography.Text>
                    <Select style={{ width: "100%", marginTop: 4 }} value={php} options={phpVersions} onChange={setPhp} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Typography.Text type="secondary">memory_limit</Typography.Text>
                    <Input style={{ marginTop: 4 }} value={phpMem} placeholder={phpAccount.memoryLimit || "256M"} onChange={(e) => setPhpMem(e.target.value)} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Typography.Text type="secondary">max_execution_time</Typography.Text>
                    <InputNumber style={{ width: "100%", marginTop: 4 }} min={1} max={3600} value={phpExec} placeholder={phpAccount.maxExecutionTime ? String(phpAccount.maxExecutionTime) : "60"} onChange={(v) => setPhpExec(v)} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Typography.Text type="secondary">max_input_time</Typography.Text>
                    <InputNumber style={{ width: "100%", marginTop: 4 }} min={1} max={3600} value={phpInput} placeholder={phpAccount.maxInputTime ? String(phpAccount.maxInputTime) : "60"} onChange={(v) => setPhpInput(v)} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Typography.Text type="secondary">post_max_size</Typography.Text>
                    <Input style={{ marginTop: 4 }} value={phpPost} placeholder={phpAccount.postMaxSize || "64M"} onChange={(e) => setPhpPost(e.target.value)} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Typography.Text type="secondary">upload_max_filesize</Typography.Text>
                    <Input style={{ marginTop: 4 }} value={phpUpload} placeholder={phpAccount.uploadMaxFilesize || "64M"} onChange={(e) => setPhpUpload(e.target.value)} />
                  </Col>
                  <Col xs={24} md={8}>
                    <Typography.Text type="secondary">date.timezone</Typography.Text>
                    <Select
                      style={{ width: "100%", marginTop: 4 }}
                      allowClear
                      showSearch
                      value={phpZone || undefined}
                      placeholder={phpAccount.timezone || "UTC"}
                      onChange={(v) => setPhpZone(v || "")}
                      options={PHP_ZONES.map((z) => ({ value: z, label: z }))}
                    />
                  </Col>
                  <Col xs={24} md={8}>
                    <Typography.Text type="secondary">display_errors</Typography.Text>
                    <Select
                      style={{ width: "100%", marginTop: 4 }}
                      value={phpErrors}
                      onChange={setPhpErrors}
                      options={[
                        { value: "", label: "Account default" },
                        { value: "on", label: "On" },
                        { value: "off", label: "Off" },
                      ]}
                    />
                  </Col>
                  <Col span={24}>
                    <Typography.Text type="secondary">disable_functions</Typography.Text>
                    <Input style={{ marginTop: 4 }} value={phpDisable} placeholder={phpAccount.disableFunctions || "exec,passthru,shell_exec,system,popen,show_source"} onChange={(e) => setPhpDisable(e.target.value)} />
                  </Col>
                </Row>
                <Button style={{ marginTop: 16 }} type="primary" loading={busy} onClick={() => void savePHP()}>
                  Save
                </Button>
              </>
            ) : (
              <Typography.Paragraph>
                {kind === "nginx"
                  ? "Nginx serves this document root directly. PHP is not used."
                  : `This site is ${kind}. PHP-FPM is not in front of it. Change the runtime under Runtime.`}
              </Typography.Paragraph>
            )
          ) : null}

          {tab === "server" ? (
            <>
              <Typography.Text type="secondary">Type</Typography.Text>
              <Select
                style={{ width: 240, display: "block", margin: "8px 0 16px" }}
                value={kind}
                options={KINDS}
                onChange={(v) => {
                  setKind(v);
                  if (APP.has(v) && !appCmd) setAppCmd(defaultCmd(v));
                }}
              />
              {phpSite ? (
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
                </>
              ) : null}
              {kind === "nginx" ? (
                <Typography.Paragraph>
                  Nginx serves files from the document root. A missing file returns 404, and PHP files are not executed.
                </Typography.Paragraph>
              ) : null}
              {kind === "proxy" ? (
                <>
                  <Typography.Text type="secondary">Proxy URL</Typography.Text>
                  <Input style={{ marginTop: 8 }} value={proxyPass} placeholder="https://doujinfast.com" onChange={(e) => setProxyPass(e.target.value)} />
                </>
              ) : null}
              {APP.has(kind) ? (
                <Space direction="vertical" style={{ width: "100%" }}>
                  <Input placeholder={defaultCmd(kind)} value={appCmd} onChange={(e) => setAppCmd(e.target.value)} />
                  <Input placeholder="port, empty = auto" value={appPort} onChange={(e) => setAppPort(e.target.value)} />
                </Space>
              ) : null}
              <Typography.Title level={5} style={{ marginTop: 8 }}>
                Nginx cache
              </Typography.Title>
              <Space>
                <Switch checked={cache.on} onChange={(on) => setCache({ ...cache, on })} />
                <span>Cache</span>
              </Space>
              <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
                Nginx remembers pages it fetched from Apache or the app and serves them again. Files read straight from disk are not stored. A reverse proxy that already has its own cache keeps that cache. A custom nginx file is left as written.
              </Typography.Paragraph>
              <NginxCacheForm value={cache} maxBytes={cacheMax} onChange={setCache} />
              <Popconfirm title="Clear the saved cache for this site?" onConfirm={() => void purgeCache()}>
                <Button loading={cacheBusy}>Clear cache</Button>
              </Popconfirm>
              <Button
                style={{ marginTop: 16 }}
                type="primary"
                loading={busy}
                onClick={() => {
                  if (cache.on && cacheSizeToken(cache.size) === null) {
                    message.error("Disk limit must be a size such as 500m or 2g.");
                    return;
                  }
                  const bytes = cacheSizeBytes(cache.size);
                  if (cache.on && cacheMax > 0 && bytes > cacheMax) {
                    message.error(`Disk limit is at most ${formatBytes(cacheMax)}, 90% of this disk.`);
                    return;
                  }
                  const ext = staticExt.trim().replace(/\s+/g, " ");
                  const options = {
                    ...baseOpts(site),
                    ...(phpSite ? { static: staticOn, staticExt: ext === DEFAULT_STATIC_EXT ? "" : ext } : {}),
                    cache: { ...cachePayload(cache), path: cache.path.trim() || cacheDir(site.domain) },
                  };
                  if (kind === "proxy") {
                    return onPatch({ kind: "proxy", proxyPass: proxyPass.trim(), appCmd: "", appPort: 0, options }, "Web server saved");
                  }
                  if (APP.has(kind)) {
                    return onPatch({ kind, proxyPass: "", appCmd, appPort: appPort ? Number(appPort) : 0, options }, "Web server saved");
                  }
                  return onPatch({ kind: kind === "nginx" ? "nginx" : "php", proxyPass: "", appCmd: "", appPort: 0, options }, "Web server saved");
                }}
              >
                Save
              </Button>
            </>
          ) : null}

          {tab === "git" ? (
            <>
              <Typography.Paragraph type="secondary">Force-pull a repository into this site and install a webhook.</Typography.Paragraph>
              <Button type="primary" onClick={onGit}>
                Open Git
              </Button>
            </>
          ) : null}

          {tab === "composer" ? (
            <>
              <Typography.Paragraph type="secondary">
                Composer runs as {site.username}. Leave the directory empty to use the folder above the document root when composer.json is there.
              </Typography.Paragraph>
              <Typography.Text type="secondary">composer.json directory</Typography.Text>
              <Space.Compact style={{ width: "100%", marginTop: 4, marginBottom: 8 }}>
                <Input value={composerPath} placeholder={composerDir || "domains/example.com"} onChange={(e) => setComposerPath(e.target.value)} />
                <Button loading={busy} onClick={() => void saveOpts({ composerRoot: composerPath.trim() }, "Composer path saved")}>
                  Save path
                </Button>
              </Space.Compact>
              {composerDir ? (
                <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
                  Using {composerDir}
                </Typography.Paragraph>
              ) : null}
              {hasComposer ? (
                <AppPackages
                  kind="composer"
                  busy={composerBusy}
                  packages={composerPkgs}
                  onRun={runComposer}
                  onSearch={searchComposer}
                  onUpdate={(name) => void runComposer(`update ${name}`)}
                />
              ) : (
                <Typography.Paragraph>No composer.json in this directory.</Typography.Paragraph>
              )}
              {composerOut ? <pre className="cmd-out">{composerOut}</pre> : null}
              <Space wrap>
                <Button onClick={onSSH}>Open SSH</Button>
                {phpSite ? (
                  <Button onClick={onLaravel}>Laravel tools</Button>
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
                Block image, CSS, and JS requests whose Referer is another site. Empty and blocked referers are still allowed. This site's primary name and aliases are allowed. Add any other site that may embed these files.
              </Typography.Paragraph>
              <Space>
                <Switch checked={hotlink} onChange={setHotlink} />
                <span>{hotlink ? "On" : "Off"}</span>
              </Space>
              <div style={{ marginTop: 12 }}>
                <Space wrap>
                  <Tag>{site.displayName || site.domain}</Tag>
                  {(site.aliases || []).map((name, i) => (
                    <Tag key={name}>{site.displayAliases?.[i] || name}</Tag>
                  ))}
                  {hotlinkExtra.map((name) => (
                    <Tag key={name} closable onClose={() => setHotlinkExtra((list) => list.filter((item) => item !== name))}>
                      {name}
                    </Tag>
                  ))}
                </Space>
                <Space.Compact style={{ width: "100%", maxWidth: 420, marginTop: 12 }}>
                  <Input
                    value={hotlinkDraft}
                    placeholder="cdn.example.com"
                    onChange={(e) => setHotlinkDraft(e.target.value)}
                    onPressEnter={addHotlink}
                  />
                  <Button onClick={addHotlink}>Add</Button>
                </Space.Compact>
              </div>
              <div>
                <Button style={{ marginTop: 12 }} type="primary" loading={busy} onClick={() => saveOpts({ hotlink, hotlinkDomains: hotlinkExtra }, "Hotlink protection saved")}>
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
              <Typography.Paragraph type="secondary">Turn each log on or off for this site. Nginx and Apache use the same choice.</Typography.Paragraph>
              <Space direction="vertical" size={12}>
                <div>
                  <Space>
                    <Switch checked={site.options?.accessLog !== false} loading={busy} onChange={(on) => void saveOpts({ accessLog: on }, on ? "Access log on" : "Access log off")} />
                    <span>Access log</span>
                  </Space>
                  <div>
                    <Typography.Text type="secondary">Records each request. Off writes nothing for this site.</Typography.Text>
                  </div>
                </div>
                <div>
                  <Space>
                    <Switch checked={site.options?.errorLog !== false} loading={busy} onChange={(on) => void saveOpts({ errorLog: on }, on ? "Error log on" : "Error log off")} />
                    <span>Error log</span>
                  </Space>
                  <div>
                    <Typography.Text type="secondary">Records server errors. Off discards them for this site.</Typography.Text>
                  </div>
                </div>
              </Space>
              <div>
                <Button style={{ marginTop: 16 }} type="primary" onClick={onLogs}>
                  Open logs
                </Button>
              </div>
            </>
          ) : null}
        </div>
    </div>
  );
}
