import { useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Card, Input, Popconfirm, Select, Space, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";

type Item = { name: string; label: string; group: string; value: string; live?: string; recommend?: string };
type Cfg = {
  nginxInstalled: boolean;
  apacheInstalled: boolean;
  nginxActive: boolean;
  apacheActive: boolean;
  totalRAM: number;
  totalRAMGB: number;
  suggestedGB: number;
  ramGB: number;
  cpus: number;
  settings?: Item[];
  message?: string;
};

const ramOptions = Array.from({ length: 64 }, (_, i) => i + 1);

export function WebOptimize() {
  const { message } = App.useApp();
  const [cfg, setCfg] = useState<Cfg | null>(null);
  const [ramGB, setRamGB] = useState(2);
  const [values, setValues] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function load(ram?: number) {
    const data = await api.get<Cfg>(`/api/webserver/optimize${ram ? `?ramGB=${ram}` : ""}`);
    setCfg(data);
    setRamGB(data.ramGB);
    const map: Record<string, string> = {};
    for (const s of data.settings || []) map[s.name] = s.value;
    setValues(map);
    setError("");
  }

  useEffect(() => {
    load().catch((e) => setError(e instanceof Error ? e.message : "Failed"));
  }, []);

  async function apply() {
    setBusy(true);
    try {
      const st = await api.put<Cfg>("/api/webserver/optimize", { ramGB, settings: values });
      setCfg(st);
      const map: Record<string, string> = {};
      for (const s of st.settings || []) map[s.name] = s.value;
      setValues(map);
      message.success("Nginx and Apache were optimized and restarted");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  const ramSelect = useMemo(
    () => ramOptions.map((n) => ({ value: n, label: cfg?.suggestedGB === n ? `${n} GB (server RAM)` : `${n} GB` })),
    [cfg?.suggestedGB],
  );

  const rows = (group: string) => (cfg?.settings || []).filter((s) => s.group === group);

  return (
    <Card
      size="small"
      title="Web server optimize"
      extra={
        <Space wrap>
          {cfg?.nginxInstalled ? <Tag color={cfg.nginxActive ? "success" : "default"}>Nginx</Tag> : null}
          {cfg?.apacheInstalled ? <Tag color={cfg.apacheActive ? "success" : "default"}>Apache</Tag> : null}
        </Space>
      }
    >
      {error ? <Alert type="error" showIcon message={error} style={{ marginBottom: 12 }} /> : null}
      {cfg?.message ? <Alert type="info" showIcon message={cfg.message} style={{ marginBottom: 12 }} /> : null}
      {cfg && (cfg.nginxInstalled || cfg.apacheInstalled) ? (
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          <Space wrap align="center" style={{ width: "100%", justifyContent: "space-between" }}>
            <Space wrap align="center">
              <Typography.Text>Optimize for RAM</Typography.Text>
              <Select
                showSearch
                style={{ width: 180 }}
                value={ramGB}
                options={ramSelect}
                onChange={(n) => {
                  setRamGB(n);
                  void load(n);
                }}
              />
              {cfg.totalRAM ? (
                <Typography.Text type="secondary">
                  Server {formatBytes(cfg.totalRAM)} · {cfg.cpus} CPU
                </Typography.Text>
              ) : null}
            </Space>
            <Space>
              <Button onClick={() => void load(cfg.suggestedGB)}>Use server RAM</Button>
              <Popconfirm title="Restart Nginx and Apache with this config?" onConfirm={() => void apply()}>
                <Button type="primary" loading={busy}>
                  Apply and restart
                </Button>
              </Popconfirm>
            </Space>
          </Space>
          <Typography.Text type="secondary">
            Caps Nginx workers to RAM (not CPU count), enables gzip, file cache, and proxy buffers. Apache behind Nginx
            gets KeepAlive off, a shorter timeout, and MPM event sized to memory.
          </Typography.Text>
          {cfg.nginxInstalled ? (
            <Table
              size="small"
              rowKey="name"
              pagination={false}
              title={() => "Nginx"}
              dataSource={rows("nginx")}
              columns={[
                {
                  title: "Setting",
                  width: 240,
                  render: (_: unknown, r: Item) => (
                    <span>
                      {r.label}
                      <div>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          {r.name}
                        </Typography.Text>
                      </div>
                    </span>
                  ),
                },
                {
                  title: "Value",
                  render: (_: unknown, r: Item) => (
                    <Input value={values[r.name] ?? r.value} onChange={(e) => setValues((cur) => ({ ...cur, [r.name]: e.target.value }))} />
                  ),
                },
                { title: "Current", dataIndex: "live", width: 160, responsive: ["md"], render: (v: string) => v || "—" },
                { title: "Preset", dataIndex: "recommend", width: 160, responsive: ["lg"], render: (v: string) => v || "—" },
              ]}
            />
          ) : null}
          {cfg.apacheInstalled ? (
            <Table
              size="small"
              rowKey="name"
              pagination={false}
              title={() => "Apache"}
              dataSource={rows("apache")}
              columns={[
                {
                  title: "Setting",
                  width: 240,
                  render: (_: unknown, r: Item) => (
                    <span>
                      {r.label}
                      <div>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          {r.name}
                        </Typography.Text>
                      </div>
                    </span>
                  ),
                },
                {
                  title: "Value",
                  render: (_: unknown, r: Item) => (
                    <Input value={values[r.name] ?? r.value} onChange={(e) => setValues((cur) => ({ ...cur, [r.name]: e.target.value }))} />
                  ),
                },
                { title: "Current", dataIndex: "live", width: 160, responsive: ["md"], render: (v: string) => v || "—" },
                { title: "Preset", dataIndex: "recommend", width: 160, responsive: ["lg"], render: (v: string) => v || "—" },
              ]}
            />
          ) : null}
        </Space>
      ) : null}
    </Card>
  );
}
