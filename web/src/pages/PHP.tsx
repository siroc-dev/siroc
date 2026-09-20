import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { Alert, App, Button, Card, Checkbox, Col, Input, InputNumber, Row, Select, Space, Switch, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { asList } from "@/lib/lists";
import { jobLabel, type InstallJob, type InstallQueue } from "@/lib/jobs";
import { formatBytes, type UserUsage } from "@/lib/usage";

type Account = { username: string };
type PHPExt = {
  name: string;
  title: string;
  source: string;
  aptPkg?: string;
  pecl?: string;
  installed: boolean;
  available: boolean;
  zend?: boolean;
};
type FPM = {
  pm: string;
  maxChildren: number;
  startServers: number;
  minSpare: number;
  maxSpare: number;
  idleTimeout: string;
  maxRequests: number;
  memoryLimit: string;
  maxExecutionTime: number;
  maxInputTime: number;
  postMaxSize: string;
  uploadMaxFilesize: string;
  displayErrors: boolean;
  timezone: string;
  disableFunctions: string;
  openBasedir: boolean;
  extensions: Record<string, string[]>;
};

const timezones = ["UTC", "Asia/Bangkok", "Asia/Tokyo", "Asia/Singapore", "Europe/London", "Europe/Berlin", "America/New_York", "America/Los_Angeles"];

function emptyFPM(): FPM {
  return {
    pm: "ondemand",
    maxChildren: 20,
    startServers: 2,
    minSpare: 1,
    maxSpare: 4,
    idleTimeout: "10s",
    maxRequests: 500,
    memoryLimit: "256M",
    maxExecutionTime: 60,
    maxInputTime: 60,
    postMaxSize: "64M",
    uploadMaxFilesize: "64M",
    displayErrors: false,
    timezone: "UTC",
    disableFunctions: "exec,passthru,shell_exec,system,proc_open,popen,show_source",
    openBasedir: true,
    extensions: {},
  };
}

export function PHP() {
  const { message } = App.useApp();
  const [params] = useSearchParams();
  const [admin, setAdmin] = useState(false);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [username, setUsername] = useState(params.get("user") || "");
  const [phps, setPhps] = useState<string[]>([]);
  const [phpVer, setPhpVer] = useState("8.3");
  const [exts, setExts] = useState<PHPExt[]>([]);
  const [settings, setSettings] = useState<FPM>(emptyFPM());
  const [custom, setCustom] = useState(false);
  const [peclName, setPeclName] = useState("");
  const [busy, setBusy] = useState("");
  const [jobs, setJobs] = useState<InstallJob[]>([]);
  const [current, setCurrent] = useState<InstallJob | null>(null);
  const [queue, setQueue] = useState<InstallJob[]>([]);
  const [usage, setUsage] = useState<UserUsage | null>(null);

  const selected = settings.extensions[phpVer] || [];
  const activeJobs = useMemo(() => {
    const list = [...queue];
    if (current) list.unshift(current);
    return list.filter((j) => j.name === "php-ext");
  }, [current, queue]);

  function extQueued(name: string) {
    const prefix = `${phpVer}:${name.toLowerCase()}`;
    return activeJobs.some((j) => j.version.startsWith(prefix));
  }

  async function loadAccounts() {
    const me = await api.get<{ admin?: boolean }>("/api/me");
    setAdmin(!!me.admin);
    const [a, p] = await Promise.all([
      api.get<Account[]>("/api/accounts"),
      api.get<string[]>("/api/software/php-versions").catch(() => [] as string[]),
    ]);
    const accounts = asList(a);
    const phps = asList(p);
    setAccounts(accounts);
    if (phps.length) {
      setPhps(phps);
      setPhpVer((cur) => (phps.includes(cur) ? cur : phps[phps.length - 1]));
    } else {
      setPhps([]);
    }
    setUsername((cur) => {
      if (cur && accounts.some((x) => x.username === cur)) return cur;
      return accounts[0]?.username || "";
    });
  }

  async function loadPHP(user: string) {
    if (!user) return;
    const data = await api.get<{ settings: FPM; custom: boolean }>(`/api/accounts/${user}/php`);
    setSettings({ ...emptyFPM(), ...data.settings, extensions: data.settings.extensions || {} });
    setCustom(!!data.custom);
  }

  async function loadExts(ver: string) {
    if (!ver) return;
    const data = await api.get<{ extensions: PHPExt[] }>(`/api/php/extensions?version=${encodeURIComponent(ver)}`);
    setExts(data.extensions || []);
  }

  async function loadJobs() {
    if (!admin) return 0;
    const data = await api.get<InstallQueue>("/api/software/jobs");
    setJobs(data.jobs || []);
    setCurrent(data.current || null);
    setQueue(data.queue || []);
    return data.active || 0;
  }

  useEffect(() => {
    loadAccounts().catch((e) => message.error(e.message));
  }, []);

  useEffect(() => {
    if (!username) return;
    loadPHP(username).catch((e) => message.error(e.message));
  }, [username]);

  useEffect(() => {
    if (!username) {
      setUsage(null);
      return;
    }
    let stop = false;
    async function poll() {
      try {
        const rows = await api.get<UserUsage[]>("/api/usage");
        if (stop) return;
        setUsage(rows.find((u) => u.user === username) || null);
      } catch {
        if (!stop) setUsage(null);
      }
    }
    poll();
    const t = setInterval(poll, 4000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, [username]);

  useEffect(() => {
    if (!phpVer) return;
    loadExts(phpVer).catch((e) => message.error(e.message));
  }, [phpVer]);

  useEffect(() => {
    if (!admin) return;
    loadJobs().catch(() => undefined);
    const t = setInterval(() => {
      loadJobs()
        .then((active) => {
          if (active === 0) loadExts(phpVer).catch(() => undefined);
        })
        .catch(() => undefined);
    }, 2000);
    return () => clearInterval(t);
  }, [admin, phpVer]);

  function patch<K extends keyof FPM>(key: K, value: FPM[K]) {
    setSettings((cur) => ({ ...cur, [key]: value }));
  }

  function toggleExt(name: string, on: boolean) {
    setSettings((cur) => {
      const have = new Set(cur.extensions[phpVer] || []);
      if (on) have.add(name);
      else have.delete(name);
      return { ...cur, extensions: { ...cur.extensions, [phpVer]: [...have] } };
    });
  }

  async function save() {
    if (!username) return;
    setBusy("save");
    try {
      await api.put(`/api/accounts/${username}/php`, { ...settings, openBasedir: true });
      setCustom(true);
      message.success(`PHP-FPM saved for ${username}`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  async function installExt(name: string, source: string) {
    setBusy(`${name}:${source}`);
    try {
      await api.post("/api/php/extensions", { version: phpVer, name, source });
      message.success(`Queued PHP ${phpVer} ${name} (${source})`);
      await loadJobs();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  async function installPecl() {
    const name = peclName.trim().toLowerCase();
    if (!name) return;
    await installExt(name, "pecl");
    setPeclName("");
  }

  const installing = activeJobs.length > 0;

  return (
    <div className="cp-page">
      <div>
        <Typography.Title level={3} style={{ margin: 0 }}>
          PHP
        </Typography.Title>
        <Typography.Paragraph type="secondary">
          Per-account PHP-FPM pools, php.ini limits, and extensions (apt + PECL). Set each website PHP version on Websites.
        </Typography.Paragraph>
      </div>

      <Card
        title="Account"
        extra={
          <Space wrap>
            {usage ? (
              <>
                <Tag>PHP {usage.phpProcesses}</Tag>
                <Tag>RAM {formatBytes(usage.memory)}</Tag>
                <Tag>CPU {usage.cpu.toFixed(1)}%</Tag>
                <Tag>Disk {formatBytes(usage.diskUsed)}</Tag>
              </>
            ) : null}
            <Tag color={custom ? "success" : "default"}>{custom ? "Custom applied" : "Defaults"}</Tag>
          </Space>
        }
      >
        <Select
          style={{ minWidth: 220 }}
          value={username || undefined}
          placeholder="Linux user"
          onChange={setUsername}
          options={accounts.map((a) => ({ value: a.username, label: a.username }))}
        />
      </Card>

      <Card title="PHP-FPM pool">
        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">Process manager</Typography.Text>
            <Select
              style={{ width: "100%", marginTop: 4 }}
              value={settings.pm}
              onChange={(v) => patch("pm", v)}
              options={[
                { value: "ondemand", label: "ondemand" },
                { value: "dynamic", label: "dynamic" },
                { value: "static", label: "static" },
              ]}
            />
          </Col>
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">Max children</Typography.Text>
            <InputNumber style={{ width: "100%", marginTop: 4 }} min={1} max={256} value={settings.maxChildren} onChange={(v) => patch("maxChildren", Number(v || 1))} />
          </Col>
          {settings.pm === "dynamic" ? (
            <>
              <Col xs={24} md={8}>
                <Typography.Text type="secondary">Start servers</Typography.Text>
                <InputNumber style={{ width: "100%", marginTop: 4 }} min={1} value={settings.startServers} onChange={(v) => patch("startServers", Number(v || 1))} />
              </Col>
              <Col xs={24} md={8}>
                <Typography.Text type="secondary">Min spare</Typography.Text>
                <InputNumber style={{ width: "100%", marginTop: 4 }} min={1} value={settings.minSpare} onChange={(v) => patch("minSpare", Number(v || 1))} />
              </Col>
              <Col xs={24} md={8}>
                <Typography.Text type="secondary">Max spare</Typography.Text>
                <InputNumber style={{ width: "100%", marginTop: 4 }} min={1} value={settings.maxSpare} onChange={(v) => patch("maxSpare", Number(v || 1))} />
              </Col>
            </>
          ) : null}
          {settings.pm === "ondemand" ? (
            <Col xs={24} md={8}>
              <Typography.Text type="secondary">Idle timeout</Typography.Text>
              <Input style={{ marginTop: 4 }} value={settings.idleTimeout} onChange={(e) => patch("idleTimeout", e.target.value)} />
            </Col>
          ) : null}
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">Max requests</Typography.Text>
            <InputNumber style={{ width: "100%", marginTop: 4 }} min={0} value={settings.maxRequests} onChange={(v) => patch("maxRequests", Number(v || 0))} />
          </Col>
        </Row>
      </Card>

      <Card title="php.ini">
        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">memory_limit</Typography.Text>
            <Input style={{ marginTop: 4 }} value={settings.memoryLimit} onChange={(e) => patch("memoryLimit", e.target.value)} />
          </Col>
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">max_execution_time</Typography.Text>
            <InputNumber style={{ width: "100%", marginTop: 4 }} min={0} value={settings.maxExecutionTime} onChange={(v) => patch("maxExecutionTime", Number(v || 0))} />
          </Col>
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">max_input_time</Typography.Text>
            <InputNumber style={{ width: "100%", marginTop: 4 }} min={0} value={settings.maxInputTime} onChange={(v) => patch("maxInputTime", Number(v || 0))} />
          </Col>
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">post_max_size</Typography.Text>
            <Input style={{ marginTop: 4 }} value={settings.postMaxSize} onChange={(e) => patch("postMaxSize", e.target.value)} />
          </Col>
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">upload_max_filesize</Typography.Text>
            <Input style={{ marginTop: 4 }} value={settings.uploadMaxFilesize} onChange={(e) => patch("uploadMaxFilesize", e.target.value)} />
          </Col>
          <Col xs={24} md={8}>
            <Typography.Text type="secondary">date.timezone</Typography.Text>
            <Select
              style={{ width: "100%", marginTop: 4 }}
              showSearch
              value={settings.timezone}
              onChange={(v) => patch("timezone", v)}
              options={timezones.map((z) => ({ value: z, label: z }))}
            />
          </Col>
          <Col span={24}>
            <Space size="large">
              <Space>
                <Switch checked={settings.displayErrors} onChange={(v) => patch("displayErrors", v)} />
                display_errors
              </Space>
            </Space>
            <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
              open_basedir is always the account home (<Typography.Text code>/home/{username}/</Typography.Text>
              ). PHP cannot read files outside that user. Temp, uploads, and sessions use{" "}
              <Typography.Text code>~/tmp</Typography.Text>.
            </Typography.Paragraph>
          </Col>
          <Col span={24}>
            <Typography.Text type="secondary">disable_functions</Typography.Text>
            <Input style={{ marginTop: 4 }} value={settings.disableFunctions} onChange={(e) => patch("disableFunctions", e.target.value)} />
          </Col>
        </Row>
      </Card>

      <Card title="Extensions" extra={phps.length ? <Typography.Text type="secondary">PHP {phpVer}</Typography.Text> : null}>
        {phps.length === 0 ? (
          <Typography.Text type="secondary">No PHP-FPM versions installed. Install PHP from Software.</Typography.Text>
        ) : (
          <Space wrap style={{ marginBottom: 16 }}>
            {phps.map((v) => (
              <Button key={v} type={v === phpVer ? "primary" : "default"} onClick={() => setPhpVer(v)}>
                PHP {v}
              </Button>
            ))}
          </Space>
        )}
        {installing ? (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message={`Installing ${current && current.name === "php-ext" ? jobLabel(current) : "PHP extension"}`}
            description={queue.filter((j) => j.name === "php-ext").length ? `${queue.filter((j) => j.name === "php-ext").length} waiting` : undefined}
          />
        ) : null}
        <Row gutter={[8, 8]}>
          {exts
            .filter((ext) => ext.source !== "system")
            .map((ext) => {
              const on = selected.includes(ext.name);
              const queued = extQueued(ext.name);
              return (
                <Col xs={24} md={12} key={ext.name}>
                  <Card size="small">
                    <FlexRow>
                      <Checkbox disabled={!ext.installed} checked={on} onChange={(e) => toggleExt(ext.name, e.target.checked)}>
                        <Typography.Text strong>{ext.title}</Typography.Text>{" "}
                        <Typography.Text type="secondary">{ext.name}</Typography.Text>
                      </Checkbox>
                      <Space size={4}>
                        {ext.installed ? <Tag color="success">installed</Tag> : <Tag>{ext.source}</Tag>}
                        {admin && !ext.installed && (ext.source === "apt" || ext.source === "both") ? (
                          <Button size="small" disabled={queued || busy !== ""} onClick={() => installExt(ext.name, "apt")}>
                            {queued ? "Queued" : "apt"}
                          </Button>
                        ) : null}
                        {admin && !ext.installed && (ext.source === "pecl" || ext.source === "both") ? (
                          <Button size="small" disabled={queued || busy !== ""} onClick={() => installExt(ext.name, "pecl")}>
                            {queued ? "Queued" : "PECL"}
                          </Button>
                        ) : null}
                      </Space>
                    </FlexRow>
                  </Card>
                </Col>
              );
            })}
        </Row>
        {admin ? (
          <Space style={{ marginTop: 16 }} wrap>
            <Input style={{ width: 240 }} value={peclName} onChange={(e) => setPeclName(e.target.value)} placeholder="redis, imagick, swoole…" />
            <Button disabled={!peclName.trim() || busy !== "" || !phpVer || extQueued(peclName.trim())} onClick={() => void installPecl()}>
              Queue PECL for PHP {phpVer}
            </Button>
          </Space>
        ) : null}
        {jobs.some((j) => j.name === "php-ext" && (j.status === "error" || j.status === "done")) ? (
          <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
            Last jobs:{" "}
            {jobs
              .filter((j) => j.name === "php-ext")
              .slice(0, 5)
              .map((j) => `${jobLabel(j)} ${j.status}`)
              .join(" · ")}
          </Typography.Paragraph>
        ) : null}
      </Card>

      <div>
        <Button type="primary" loading={busy === "save"} disabled={!username} onClick={() => void save()}>
          Save PHP-FPM for {username || "account"}
        </Button>
      </div>
    </div>
  );
}

function FlexRow({ children }: { children: ReactNode }) {
  return <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8 }}>{children}</div>;
}
