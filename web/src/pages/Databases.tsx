import { useEffect, useMemo, useState } from "react";
import { useOutletContext } from "react-router-dom";
import { Alert, App, AutoComplete, Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tabs, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";

type Account = { username: string };
type DB = { id: number; username: string; dbName: string; dbUser: string; engine: string };
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
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [list, setList] = useState<DB[]>([]);
  const [engine, setEngine] = useState("");
  const [created, setCreated] = useState("");
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm();
  const [cfg, setCfg] = useState<DBConfig | null>(null);
  const [ramGB, setRamGB] = useState(1);
  const [values, setValues] = useState<Record<string, string>>({});
  const [cfgBusy, setCfgBusy] = useState(false);
  const [cfgErr, setCfgErr] = useState("");

  async function load() {
    const [a, d, e] = await Promise.all([
      api.get<Account[]>("/api/accounts"),
      api.get<DB[]>("/api/databases"),
      api.get<{ engine: string }>("/api/databases/engine").catch(() => ({ engine: "" })),
    ]);
    setAccounts(a);
    setList(d);
    setEngine(e.engine);
    if (a[0] && !form.getFieldValue("username")) form.setFieldValue("username", a[0].username);
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
    load().catch((err) => message.error(err.message));
  }, []);

  useEffect(() => {
    if (admin) loadConfig().catch(() => undefined);
  }, [admin]);

  async function create(v: { username: string; dbName: string; dbUser: string; password: string }) {
    setBusy(true);
    setCreated("");
    try {
      const res = await api.post<{ database: DB; password: string }>("/api/databases", v);
      setCreated(`Created ${res.database.dbName} / ${res.database.dbUser}. Password is shown only once: ${res.password}`);
      form.resetFields(["dbName", "dbUser", "password"]);
      message.success("Database created");
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

  const engineLabel = engine === "mariadb" ? "MariaDB" : engine === "mysql" ? "MySQL" : "";
  const ramSelect = useMemo(
    () =>
      ramOptions.map((n) => ({
        value: n,
        label: cfg?.suggestedGB === n ? `${n} GB (server RAM)` : `${n} GB`,
      })),
    [cfg?.suggestedGB],
  );

  const databasesTab = (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      {created ? <Alert type="success" message={created} showIcon /> : null}
      <Card>
        <Table
          rowKey="id"
          dataSource={list}
          pagination={false}
          locale={{ emptyText: "No databases yet." }}
          columns={[
            { title: "Database", dataIndex: "dbName" },
            { title: "User", dataIndex: "dbUser" },
            { title: "Account", dataIndex: "username" },
            {
              title: "",
              align: "right" as const,
              render: (_: unknown, d: DB) => (
                <Space>
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

  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            Databases
          </Typography.Title>
          <Typography.Text type="secondary">{engine ? `Engine: ${engine}` : "Install MySQL or MariaDB first"}</Typography.Text>
        </div>
        <Button type="primary" disabled={!engine} onClick={() => setOpen(true)}>
          Create database
        </Button>
      </div>
      {admin ? (
        <Tabs
          items={[
            { key: "list", label: "Databases", children: databasesTab },
            { key: "config", label: "Config", children: configTab },
          ]}
        />
      ) : (
        databasesTab
      )}

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
            <Select options={accounts.map((a) => ({ value: a.username, label: a.username }))} />
          </Form.Item>
          <Form.Item name="dbName" label="DB name suffix" rules={[{ required: true }]}>
            <Input placeholder="wp" />
          </Form.Item>
          <Form.Item name="dbUser" label="DB user suffix" rules={[{ required: true }]}>
            <Input placeholder="wp" />
          </Form.Item>
          <Form.Item name="password" label="Password" rules={[{ required: true, min: 8 }]}>
            <Input.Password />
          </Form.Item>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            Names are prefixed with the Linux username, e.g. alice_wp.
          </Typography.Paragraph>
        </Form>
      </Modal>
    </div>
  );
}
