import { useEffect, useMemo, useState } from "react";
import { Link, useOutletContext, useSearchParams } from "react-router-dom";
import { Alert, App, AutoComplete, Button, Card, Dropdown, Form, Input, Modal, Popconfirm, Progress, Select, Space, Table, Tabs, Tag, Typography } from "antd";
import { ReloadOutlined } from "@ant-design/icons";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";
import { InstallLog } from "@/components/InstallLog";
import { DatabaseMonitorPanel } from "@/pages/DatabaseMonitor";
import { RedisConfigPanel } from "@/pages/RedisConfig";
import { RedisStatusPanel, type RedisStatusData } from "@/pages/RedisStatus";
import { PageSkeleton } from "@/components/PageSkeleton";

type Account = { username: string };
type DB = { id: number; username: string; dbName: string; dbUser: string; engine: string; hasPassword?: boolean; size?: number };
type CfgItem = { name: string; label: string; value: string; live?: string; recommend?: string };
type DBConfig = {
  engine: string;
  installed: boolean;
  active: boolean;
  version?: string;
  confPath?: string;
  totalRAM: number;
  totalRAMGB: number;
  suggestedGB: number;
  ramGB: number;
  settings?: CfgItem[];
  message?: string;
};

const ramOptions = Array.from({ length: 128 }, (_, i) => i + 1);

export function Databases() {
  const { message } = App.useApp();
  const { admin } = useOutletContext<{ user: string; admin?: boolean }>();
  const [search, setSearch] = useSearchParams();
  const wantUser = (search.get("user") || "").trim();
  const wantDomain = (search.get("domain") || "").trim();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [list, setList] = useState<DB[]>([]);
  const [engine, setEngine] = useState("");
  const [created, setCreated] = useState("");
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm();
  const [passForm] = Form.useForm();
  const [passTarget, setPassTarget] = useState<DB | null>(null);
  const [newPass, setNewPass] = useState("");
  const [visiblePass, setVisiblePass] = useState<Record<number, string>>({});
  const watchUser = Form.useWatch("username", form);
  const [cfg, setCfg] = useState<DBConfig | null>(null);
  const [ramGB, setRamGB] = useState(1);
  const [values, setValues] = useState<Record<string, string>>({});
  const [cfgBusy, setCfgBusy] = useState(false);
  const [cfgErr, setCfgErr] = useState("");
  const [redis, setRedis] = useState<RedisStatusData | null>(null);
  const [redisErr, setRedisErr] = useState("");
  const [ioBusy, setIoBusy] = useState("");
  const [importPct, setImportPct] = useState(0);
  const [importFile, setImportFile] = useState("");
  const [importId, setImportId] = useState(0);
  const [importDbName, setImportDbName] = useState("");
  const [importLive, setImportLive] = useState(false);
  const [importErr, setImportErr] = useState("");
  const [importOk, setImportOk] = useState("");
  const [ready, setReady] = useState(false);
  const [logOpen, setLogOpen] = useState(false);
  const [logToken, setLogToken] = useState(0);

  async function load() {
    const [a, d, e] = await Promise.all([
      api.get<Account[]>("/api/accounts"),
      api.get<DB[]>("/api/databases"),
      api.get<{ engine: string }>("/api/databases/engine").catch(() => ({ engine: "" })),
    ]);
    setAccounts(a);
    setList(d);
    setEngine(e.engine);
    const pick = wantUser && a.some((x) => x.username === wantUser) ? wantUser : a[0]?.username;
    if (pick && !form.getFieldValue("username")) form.setFieldValue("username", pick);
  }

  async function loadConfig(gb?: number) {
    if (!admin) return;
    setCfgErr("");
    try {
      const q = gb && gb > 0 ? `?ramGB=${gb}` : "";
      const st = await api.get<DBConfig>(`/api/databases/config${q}`);
      setCfg(st);
      const nextGB = gb && gb > 0 ? gb : st.ramGB || st.suggestedGB || 1;
      setRamGB(nextGB);
      setValues((prev) => {
        const next: Record<string, string> = {};
        for (const s of st.settings || []) {
          if (s.name === "bind_address") {
            next[s.name] = gb && prev.bind_address ? prev.bind_address : s.value;
          } else if (gb && s.recommend) {
            next[s.name] = s.recommend;
          } else {
            next[s.name] = s.value;
          }
        }
        return next;
      });
    } catch (err) {
      setCfgErr(err instanceof Error ? err.message : "Failed to load config");
    }
  }

  useEffect(() => {
    load()
      .catch((err) => message.error(err.message))
      .finally(() => setReady(true));
  }, []);

  async function loadRedis() {
    if (!admin) return;
    try {
      const st = await api.get<RedisStatusData>("/api/redis/status");
      setRedis(st);
      setRedisErr("");
    } catch (err) {
      setRedisErr(err instanceof Error ? err.message : "Failed to load Redis");
    }
  }

  useEffect(() => {
    if (!admin) return;
    loadConfig().catch(() => undefined);
    loadRedis().catch(() => undefined);
    const t = setInterval(() => {
      loadRedis().catch(() => undefined);
    }, 4000);
    return () => clearInterval(t);
  }, [admin]);

  async function fillRandom(username?: string, kind: "all" | "suffix" | "password" = "all") {
    const user = username || form.getFieldValue("username") || wantUser || accounts[0]?.username || "";
    try {
      const q = user ? `?username=${encodeURIComponent(user)}` : "";
      const data = await api.get<{ suffix: string; password: string }>(`/api/databases/suggest${q}`);
      const next: { dbName?: string; dbUser?: string; password?: string } = {};
      if (kind === "all" || kind === "suffix") {
        next.dbName = data.suffix;
        next.dbUser = data.suffix;
      }
      if (kind === "all" || kind === "password") next.password = data.password;
      form.setFieldsValue(next);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Cannot generate database names");
    }
  }

  function openCreate() {
    setOpen(true);
    setTimeout(() => void fillRandom(), 0);
  }

  async function create(v: { username: string; dbName: string; dbUser: string; password: string }) {
    setBusy(true);
    setCreated("");
    try {
      const res = await api.post<{ database: DB; password: string }>("/api/databases", v);
      setVisiblePass((prev) => ({ ...prev, [res.database.id]: res.password }));
      setCreated(`Created ${res.database.dbName} / ${res.database.dbUser}. The password is saved and shown in the list.`);
      form.resetFields(["dbName", "dbUser", "password"]);
      message.success("Database created");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function showDbPassword(d: DB) {
    try {
      const out = await api.get<{ password: string }>(`/api/databases/${d.id}/password`);
      setVisiblePass((prev) => ({ ...prev, [d.id]: out.password }));
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Password is not stored");
    }
  }

  function openReset(d: DB) {
    setPassTarget(d);
    setNewPass("");
    passForm.resetFields();
  }

  async function randomResetPassword() {
    try {
      const data = await api.get<{ password: string }>("/api/password/suggest");
      passForm.setFieldValue("password", data.password);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Cannot generate password");
    }
  }

  async function resetDbPassword(values: { password: string }) {
    if (!passTarget) return;
    setBusy(true);
    try {
      const out = await api.put<{ password: string }>(`/api/databases/${passTarget.id}/password`, {
        password: values.password,
      });
      const pw = out.password || values.password;
      setNewPass(pw);
      setVisiblePass((prev) => ({ ...prev, [passTarget.id]: pw }));
      message.success(`Password reset for ${passTarget.dbUser}`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: number) {
    try {
      await api.delete(`/api/databases/${id}`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  async function exportDb(d: DB, format: string) {
    setIoBusy(`export:${d.id}`);
    try {
      const res = await fetch(`/api/databases/${d.id}/export?format=${encodeURIComponent(format)}`, { credentials: "include" });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error((data as { error?: string }).error || res.statusText);
      }
      const blob = await res.blob();
      const match = /filename="([^"]+)"/.exec(res.headers.get("Content-Disposition") || "");
      const fallback = format === "gz" ? `${d.dbName}.sql.gz` : format === "zip" ? `${d.dbName}.zip` : `${d.dbName}.sql`;
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = match?.[1] || fallback;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Export failed");
    } finally {
      setIoBusy("");
    }
  }

  function importDb(d: DB) {
    Modal.confirm({
      title: `Import into ${d.dbName}?`,
      content: "Tables in this database are replaced by the file. Use a .sql, .gz, or .zip dump up to 10GB.",
      okText: "Choose file",
      onOk: () => {
        const input = document.createElement("input");
        input.type = "file";
        input.accept = ".sql,.gz,.zip,application/gzip,application/zip,application/sql";
        input.onchange = () => {
          const file = input.files?.[0];
          if (!file) return;
          setIoBusy(`import:${d.id}`);
          setImportFile(file.name);
          setImportId(d.id);
          setImportDbName(d.dbName);
          setImportPct(0);
          setImportErr("");
          setImportOk("");
          setImportLive(true);
          setLogToken((n) => n + 1);
          setLogOpen(true);
          api.uploadProgress(`/api/databases/${d.id}/import`, file, setImportPct)
            .then(() => {
              setImportOk(`Imported into ${d.dbName}`);
              message.success(`Imported into ${d.dbName}`);
            })
            .catch((err) => {
              const text = err instanceof Error ? err.message : "Import failed";
              setImportErr(text);
              message.error(text);
            })
            .finally(() => {
              setIoBusy("");
              setImportLive(false);
            });
        };
        input.click();
      },
    });
  }

  async function applyConfig() {
    setCfgBusy(true);
    try {
      const st = await api.put<DBConfig>("/api/databases/config", { ramGB, settings: values });
      setCfg(st);
      const map: Record<string, string> = {};
      for (const s of st.settings || []) map[s.name] = s.value;
      setValues(map);
      message.success("Saved and restarted " + (st.engine === "mariadb" ? "MariaDB" : "MySQL"));
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setCfgBusy(false);
    }
  }

  const tab = search.get("tab") || "list";
  const adminTab = tab === "redis" || tab === "monitor" || tab === "config" ? tab : "list";
  const engineLabel = engine === "mariadb" ? "MariaDB" : engine === "mysql" ? "MySQL" : "";
  const ramSelect = useMemo(
    () =>
      ramOptions.map((n) => ({
        value: n,
        label: cfg?.suggestedGB === n ? `${n} GB (server RAM)` : `${n} GB`,
      })),
    [cfg?.suggestedGB],
  );

  const shown = wantUser ? list.filter((d) => d.username === wantUser) : list;

  const databasesTab = (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      {created ? <Alert type="success" message={created} showIcon /> : null}
      <Card>
        <Table
          rowKey="id"
          dataSource={shown}
          pagination={false}
          locale={{ emptyText: "No databases yet." }}
          columns={[
            { title: "Database", dataIndex: "dbName" },
            { title: "Size", dataIndex: "size", width: 110, render: (n: number) => formatBytes(n || 0) },
            { title: "User", dataIndex: "dbUser" },
            { title: "Account", dataIndex: "username" },
            {
              title: "Password",
              render: (_: unknown, d: DB) => {
                const pw = visiblePass[d.id];
                if (pw) {
                  return (
                    <Space size={4}>
                      <Typography.Text copyable>{pw}</Typography.Text>
                      <Button size="small" type="link" onClick={() => setVisiblePass((prev) => {
                        const next = { ...prev };
                        delete next[d.id];
                        return next;
                      })}>
                        Hide
                      </Button>
                    </Space>
                  );
                }
                if (!d.hasPassword) return <Typography.Text type="secondary">Not stored</Typography.Text>;
                return (
                  <Button size="small" onClick={() => void showDbPassword(d)}>
                    Show
                  </Button>
                );
              },
            },
            {
              title: "",
              align: "right" as const,
              render: (_: unknown, d: DB) => (
                <Space wrap>
                  <Dropdown
                    menu={{
                      items: [
                        { key: "sql", label: "SQL" },
                        { key: "gz", label: "GZ" },
                        { key: "zip", label: "ZIP" },
                      ],
                      onClick: ({ key }) => void exportDb(d, key),
                    }}
                  >
                    <Button size="small" loading={ioBusy === `export:${d.id}`}>Export</Button>
                  </Dropdown>
                  <Button size="small" loading={ioBusy === `import:${d.id}`} onClick={() => importDb(d)}>
                    Import
                  </Button>
                  <Button
                    size="small"
                    onClick={() => {
                      if (importLive && importId === d.id) {
                        setLogOpen(true);
                        return;
                      }
                      setImportId(d.id);
                      setImportDbName(d.dbName);
                      setImportFile("");
                      setImportErr("");
                      setImportOk("");
                      setImportLive(false);
                      setLogToken((n) => n + 1);
                      setLogOpen(true);
                    }}
                    disabled={importLive && importId !== d.id}
                  >
                    Log
                  </Button>
                  <Button
                    size="small"
                    onClick={async () => {
                      try {
                        const out = await api.post<{ url: string }>(`/api/databases/${d.id}/phpmyadmin`);
                        window.open(out.url, "_blank", "noopener");
                      } catch (err) {
                        message.error(err instanceof Error ? err.message : "phpMyAdmin failed");
                      }
                    }}
                  >
                    phpMyAdmin
                  </Button>
                  <Button size="small" onClick={() => openReset(d)}>
                    Reset password
                  </Button>
                  <Popconfirm title={`Delete ${d.dbName}?`} onConfirm={() => remove(d.id)}>
                    <Button size="small" danger>
                      Delete
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Card>
    </Space>
  );

  const configTab = (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      {cfgErr ? <Alert type="error" showIcon message={cfgErr} /> : null}
      {cfg && !cfg.installed ? <Alert type="info" showIcon message={cfg.message || "Install MySQL or MariaDB first"} /> : null}
      {cfg?.installed ? (
        <>
          <Card>
            <Space direction="vertical" size={12} style={{ width: "100%" }}>
              <Space wrap align="center" size={12} style={{ width: "100%", justifyContent: "space-between" }}>
                <Space wrap align="center">
                  <Typography.Text>Optimize for RAM</Typography.Text>
                  <Select
                    showSearch
                    style={{ width: 180 }}
                    value={ramGB}
                    options={ramSelect}
                    onChange={(n) => {
                      setRamGB(n);
                      void loadConfig(n);
                    }}
                  />
                  {cfg.totalRAM ? (
                    <Typography.Text type="secondary">
                      Server {formatBytes(cfg.totalRAM)}
                      {cfg.active ? "" : " · service stopped"}
                    </Typography.Text>
                  ) : null}
                  {cfg.version ? <Tag>{cfg.engine}</Tag> : null}
                </Space>
                <Space>
                  <Button onClick={() => void loadConfig(cfg.suggestedGB)}>Use server RAM</Button>
                  <Popconfirm
                    title={`Restart ${engineLabel || "database"} with this config?`}
                    onConfirm={() => void applyConfig()}
                  >
                    <Button type="primary" loading={cfgBusy} disabled={!cfg.installed}>
                      Apply and restart
                    </Button>
                  </Popconfirm>
                </Space>
              </Space>
              <Space wrap align="center">
                <Typography.Text>Bind address</Typography.Text>
                <AutoComplete
                  style={{ width: 320 }}
                  value={values.bind_address || ""}
                  onChange={(v) => setValues((cur) => ({ ...cur, bind_address: v }))}
                  options={[
                    { value: "127.0.0.1", label: "127.0.0.1 — local only" },
                    { value: "0.0.0.0", label: "0.0.0.0 — all IPv4" },
                    { value: "::", label: ":: — all IPv6" },
                    { value: "*", label: "* — all addresses" },
                  ]}
                  placeholder="127.0.0.1"
                  filterOption={(input, opt) => (opt?.value || "").includes(input) || String(opt?.label || "").toLowerCase().includes(input.toLowerCase())}
                />
                <Typography.Text type="secondary">
                  Live {cfg.settings?.find((s) => s.name === "bind_address")?.live || "—"}
                </Typography.Text>
              </Space>
            </Space>
          </Card>
          <Card size="small" title="my.cnf settings" extra={cfg.confPath ? <Typography.Text type="secondary">{cfg.confPath}</Typography.Text> : null}>
            <Table
              size="small"
              rowKey="name"
              pagination={false}
              dataSource={(cfg.settings || []).filter((s) => s.name !== "bind_address")}
              columns={[
                { title: "Setting", dataIndex: "label", width: 220, render: (v: string, r: CfgItem) => (
                  <span>
                    {v}
                    <div><Typography.Text type="secondary" style={{ fontSize: 12 }}>{r.name}</Typography.Text></div>
                  </span>
                ) },
                {
                  title: "Value",
                  render: (_: unknown, r: CfgItem) => (
                    <Input
                      value={values[r.name] ?? r.value}
                      onChange={(e) => setValues((cur) => ({ ...cur, [r.name]: e.target.value }))}
                    />
                  ),
                },
                { title: "Live", dataIndex: "live", width: 140, responsive: ["md"], render: (v: string) => v || "—" },
                { title: "Preset", dataIndex: "recommend", width: 140, responsive: ["lg"], render: (v: string) => v || "—" },
              ]}
            />
          </Card>
        </>
      ) : null}
    </Space>
  );

  const redisTab = (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      {redisErr ? <Alert type="error" showIcon message={redisErr} /> : null}
      {!redis ? (
        <PageSkeleton bare cards={2} rows={3} />
      ) : !redis.installed ? (
        <Alert type="info" showIcon message={redis.message || "Install Redis from Software first"} />
      ) : !redis.active || !redis.ready ? (
        <Alert type="warning" showIcon message={redis.message || "Redis is installed but not running"} />
      ) : (
        <>
          <Space wrap>
            <Tag color="success">Running</Tag>
            {redis.version ? <Tag>{redis.version}</Tag> : null}
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void loadRedis()}>
              Refresh
            </Button>
            <Link to="/redis">Open live status</Link>
          </Space>
          <RedisStatusPanel data={redis} />
        </>
      )}
      <RedisConfigPanel />
    </Space>
  );

  if (!ready) return <PageSkeleton />;

  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            {wantDomain ? `Databases for ${wantDomain}` : "Databases"}
          </Typography.Title>
          <Typography.Text type="secondary">
            {engine ? `Engine: ${engine}` : "Install MySQL or MariaDB first"}
            {admin && redis ? (
              <>
                {" · "}
                Redis:{" "}
                <Tag color={redis.active && redis.ready ? "success" : redis.installed ? "error" : "default"} style={{ marginInlineEnd: 0 }}>
                  {redis.active && redis.ready ? "Running" : redis.installed ? "Stopped" : "Not installed"}
                </Tag>
              </>
            ) : null}
          </Typography.Text>
        </div>
        <Button type="primary" disabled={!engine} onClick={openCreate}>
          Create database
        </Button>
      </div>
      {admin ? (
        <Tabs
          activeKey={adminTab}
          onChange={(key) => {
            const next = new URLSearchParams(search);
            if (key === "list") next.delete("tab");
            else next.set("tab", key);
            setSearch(next, { replace: true });
          }}
          items={[
            { key: "list", label: <Link to="/databases">Databases</Link>, children: databasesTab },
            { key: "redis", label: <Link to="/databases?tab=redis">Redis</Link>, children: redisTab },
            { key: "monitor", label: <Link to="/databases?tab=monitor">Monitor</Link>, children: <DatabaseMonitorPanel /> },
            { key: "config", label: <Link to="/databases?tab=config">Config</Link>, children: configTab },
          ]}
        />
      ) : (
        databasesTab
      )}

      <Modal
        title={passTarget ? `Reset password · ${passTarget.dbName}` : "Reset password"}
        open={!!passTarget}
        onCancel={() => {
          setPassTarget(null);
          setNewPass("");
        }}
        onOk={() => (newPass ? setPassTarget(null) : passForm.submit())}
        confirmLoading={busy}
        destroyOnHidden
        okText={newPass ? "Done" : "Reset password"}
      >
        <Form form={passForm} layout="vertical" onFinish={resetDbPassword} requiredMark={false} style={{ marginTop: 8 }}>
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 12 }}
            message="This changes the MySQL user password and saves it so it can be shown again."
          />
          {newPass ? (
            <>
              <Typography.Text type="secondary">New password</Typography.Text>
              <Typography.Paragraph copyable style={{ marginBottom: 0 }}>
                {newPass}
              </Typography.Paragraph>
            </>
          ) : (
            <>
              <Form.Item name="password" label="New password" rules={[{ required: true, min: 8 }]}>
                <Input.Password autoComplete="new-password" />
              </Form.Item>
              <Button icon={<ReloadOutlined />} onClick={() => void randomResetPassword()}>
                Random password
              </Button>
            </>
          )}
        </Form>
      </Modal>

      <Modal
        title="Create database"
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={busy}
        destroyOnHidden
        okText="Create"
        okButtonProps={{ disabled: !engine }}
      >
        <Form form={form} layout="vertical" onFinish={create} requiredMark={false} disabled={!engine} style={{ marginTop: 8 }}>
          <Form.Item name="username" label="Account" rules={[{ required: true }]}>
            <Select
              options={accounts.map((a) => ({ value: a.username, label: a.username }))}
              onChange={(v) => void fillRandom(v)}
            />
          </Form.Item>
          <Form.Item name="dbName" label="DB name suffix">
            <Input placeholder="auto" addonBefore={`${watchUser || "user"}_`} />
          </Form.Item>
          <Form.Item name="dbUser" label="DB user suffix">
            <Input placeholder="same as DB name" addonBefore={`${watchUser || "user"}_`} />
          </Form.Item>
          <Form.Item name="password" label="Password">
            <Input.Password placeholder="auto" />
          </Form.Item>
          <Space wrap style={{ marginBottom: 12 }}>
            <Button icon={<ReloadOutlined />} onClick={() => void fillRandom(undefined, "suffix")}>
              Random suffix
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => void fillRandom(undefined, "password")}>
              Random password
            </Button>
          </Space>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            Empty suffix or password is filled with a random value. Final names look like alice_k7m2nq.
          </Typography.Paragraph>
        </Form>
      </Modal>
      <Modal
        title={importDbName ? `Import log · ${importDbName}` : "Import log"}
        open={logOpen}
        width={760}
        footer={importLive ? null : <Button onClick={() => setLogOpen(false)}>Close</Button>}
        closable={!importLive}
        maskClosable={false}
        onCancel={() => {
          if (!importLive) setLogOpen(false);
        }}
        destroyOnHidden
      >
        {importLive ? (
          <>
            <Typography.Paragraph style={{ marginTop: 0 }}>{importFile}</Typography.Paragraph>
            <Progress percent={importPct} status="active" />
            <Typography.Paragraph type="secondary">
              {importPct >= 100 ? "File sent. MySQL import is still running." : "Sending the file. The log updates as the server receives it."}
            </Typography.Paragraph>
          </>
        ) : null}
        {importOk ? <Alert type="success" showIcon message={importOk} style={{ marginBottom: 12 }} /> : null}
        {importErr ? <Alert type="error" showIcon message={importErr} style={{ marginBottom: 12 }} /> : null}
        {importId ? (
          <InstallLog key={logToken} name={String(importId)} live={importLive} endpoint={`/api/databases/${importId}/import-log`} />
        ) : null}
      </Modal>
    </div>
  );
}
