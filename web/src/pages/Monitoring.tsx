import { useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Card, Col, InputNumber, Progress, Row, Space, Switch, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes, type UserUsage } from "@/lib/usage";

type CPU = { cores: number; model?: string; percent: number };
type Mem = { total: number; used: number; free: number; available?: number; percent: number };
type Disk = { mount: string; device: string; fs: string; total: number; used: number; free: number; percent: number };
type Net = { name: string; rxBytes: number; txBytes: number; rxRate: number; txRate: number };
type Proc = { pid: number; user: string; name: string; cpu: number; memory: number; memPct: number };
type Svc = { name: string; title: string; active: boolean };
type Listen = { proto: string; address: string; port: number; pid: number; user: string; name: string };
type Logs = {
  retentionDays: number;
  cloudflarePrefixes: number;
  cloudflareUpdated?: string;
  cloudflareSource?: string;
  cloudflareOK: boolean;
  realIPModule: boolean;
  goaccess: boolean;
  logrotate: boolean;
  nginxLogs?: string;
};
type Stats = {
  hostname: string;
  os: string;
  kernel: string;
  arch: string;
  uptimeSec: number;
  time: string;
  cpu: CPU;
  memory: Mem;
  swap: Mem;
  load: { one: number; five: number; fifteen: number };
  disks: Disk[];
  network: Net[];
  processes: number;
  top: Proc[];
  services: Svc[];
  users?: UserUsage[];
  listen?: Listen[];
};

function rate(n: number) {
  return `${formatBytes(n)}/s`;
}

function uptime(sec: number) {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d) return `${d}d ${h}h ${m}m`;
  if (h) return `${h}h ${m}m`;
  return `${m}m`;
}

function tone(p: number) {
  if (p >= 90) return "exception" as const;
  if (p >= 75) return "normal" as const;
  return "success" as const;
}

function Spark({ values }: { values: number[] }) {
  if (values.length < 2) return null;
  return (
    <div style={{ display: "flex", alignItems: "flex-end", gap: 2, height: 28, marginTop: 8 }}>
      {values.map((v, i) => (
        <div
          key={i}
          title={`${v.toFixed(0)}%`}
          style={{
            flex: 1,
            minWidth: 3,
            height: `${Math.max(4, Math.min(100, v))}%`,
            background: v >= 90 ? "#ff4d4f" : v >= 75 ? "#faad14" : "#4f46e5",
            borderRadius: 1,
            opacity: 0.85,
          }}
        />
      ))}
    </div>
  );
}

export function Monitoring() {
  const { message } = App.useApp();
  const [data, setData] = useState<Stats | null>(null);
  const [error, setError] = useState("");
  const [live, setLive] = useState(true);
  const [cpuHist, setCpuHist] = useState<number[]>([]);
  const [memHist, setMemHist] = useState<number[]>([]);
  const [logs, setLogs] = useState<Logs | null>(null);
  const [days, setDays] = useState(90);
  const [logBusy, setLogBusy] = useState("");

  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const st = await api.get<Stats>("/api/monitoring");
        if (stop) return;
        setData(st);
        setError("");
        setCpuHist((h) => [...h, st.cpu.percent].slice(-30));
        setMemHist((h) => [...h, st.memory.percent].slice(-30));
      } catch (e) {
        if (!stop) setError(e instanceof Error ? e.message : "Failed");
      }
    }
    load();
    if (!live) return () => {
      stop = true;
    };
    const t = setInterval(load, 3000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, [live]);

  useEffect(() => {
    api
      .get<Logs>("/api/logs")
      .then((st) => {
        setLogs(st);
        setDays(st.retentionDays || 90);
      })
      .catch(() => undefined);
  }, []);

  async function saveRetention() {
    setLogBusy("save");
    try {
      const st = await api.put<Logs>("/api/logs", { days });
      setLogs(st);
      setDays(st.retentionDays);
      message.success(`Keep access logs for ${st.retentionDays} days`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setLogBusy("");
    }
  }

  async function refreshCF() {
    setLogBusy("cf");
    try {
      const st = await api.post<Logs>("/api/logs/cloudflare");
      setLogs(st);
      message.success(`Cloudflare real IP updated (${st.cloudflarePrefixes} prefixes)`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setLogBusy("");
    }
  }

  const loadBusy = useMemo(() => {
    if (!data) return 0;
    return data.cpu.cores ? Math.min(100, (data.load.one / data.cpu.cores) * 100) : 0;
  }, [data]);

  if (error && !data) return <Alert type="error" message={error} />;
  if (!data) return <Typography.Text type="secondary">Loading…</Typography.Text>;

  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            System monitoring
          </Typography.Title>
          <Typography.Text type="secondary">
            {data.hostname} · {data.os} · {data.kernel} ({data.arch}) · up {uptime(data.uptimeSec)}
          </Typography.Text>
        </div>
        <Space>
          {error ? <Tag color="warning">{error}</Tag> : <Tag color="success">{data.processes} processes</Tag>}
          <Switch checked={live} onChange={setLive} checkedChildren="Live" unCheckedChildren="Paused" />
        </Space>
      </div>

      <Row gutter={16}>
        <Col xs={24} sm={12} lg={6}>
          <Card size="small" title="CPU">
            <Progress type="dashboard" percent={Math.round(data.cpu.percent)} status={tone(data.cpu.percent)} />
            <Typography.Text type="secondary">
              {data.cpu.cores} cores{data.cpu.model ? ` · ${data.cpu.model}` : ""}
            </Typography.Text>
            <Spark values={cpuHist} />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card size="small" title="Memory">
            <Progress type="dashboard" percent={Math.round(data.memory.percent)} status={tone(data.memory.percent)} />
            <Typography.Text type="secondary">
              {formatBytes(data.memory.used)} / {formatBytes(data.memory.total)}
            </Typography.Text>
            <Spark values={memHist} />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card size="small" title="Swap">
            <Progress type="dashboard" percent={Math.round(data.swap.percent)} status={data.swap.total ? tone(data.swap.percent) : "normal"} />
            <Typography.Text type="secondary">
              {data.swap.total ? `${formatBytes(data.swap.used)} / ${formatBytes(data.swap.total)}` : "No swap"}
            </Typography.Text>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card size="small" title="Load average">
            <Progress type="dashboard" percent={Math.round(loadBusy)} status={tone(loadBusy)} format={() => data.load.one.toFixed(2)} />
            <Typography.Text type="secondary">
              {data.load.one.toFixed(2)} / {data.load.five.toFixed(2)} / {data.load.fifteen.toFixed(2)}
            </Typography.Text>
          </Card>
        </Col>
      </Row>

      <Card title="Disks">
        <Table
          size="small"
          rowKey="mount"
          pagination={false}
          dataSource={data.disks}
          columns={[
            { title: "Mount", dataIndex: "mount" },
            { title: "Device", dataIndex: "device", ellipsis: true },
            { title: "FS", dataIndex: "fs", width: 100 },
            { title: "Used", render: (_, d) => `${formatBytes(d.used)} / ${formatBytes(d.total)}` },
            {
              title: "",
              width: 220,
              render: (_, d) => <Progress percent={Math.round(d.percent)} size="small" status={tone(d.percent)} />,
            },
          ]}
        />
      </Card>

      <Row gutter={16}>
        <Col xs={24} lg={10}>
          <Card title="Network">
            <Table
              size="small"
              rowKey="name"
              pagination={false}
              dataSource={data.network}
              columns={[
                { title: "Iface", dataIndex: "name", width: 90 },
                { title: "RX", render: (_, n) => rate(n.rxRate) },
                { title: "TX", render: (_, n) => rate(n.txRate) },
                { title: "Total", render: (_, n) => `${formatBytes(n.rxBytes)} / ${formatBytes(n.txBytes)}` },
              ]}
            />
          </Card>
        </Col>
        <Col xs={24} lg={14}>
          <Card title="Services">
            <Space wrap>
              {data.services.map((s) => (
                <Tag key={s.name} color={s.active ? "success" : "default"}>
                  {s.title} · {s.active ? "active" : "stopped"}
                </Tag>
              ))}
              {!data.services.length ? <Typography.Text type="secondary">No watched services found.</Typography.Text> : null}
            </Space>
          </Card>
        </Col>
      </Row>

      <Card title="Listening ports">
        <Table
          size="small"
          rowKey={(p) => `${p.proto}-${p.address}-${p.pid}`}
          pagination={false}
          dataSource={data.listen || []}
          locale={{ emptyText: "No listening sockets found." }}
          columns={[
            {
              title: "Proto",
              dataIndex: "proto",
              width: 80,
              render: (v: string) => <Tag>{v}</Tag>,
            },
            { title: "Port", dataIndex: "port", width: 90 },
            { title: "Address", dataIndex: "address", ellipsis: true },
            { title: "Process", dataIndex: "name", ellipsis: true },
            { title: "PID", dataIndex: "pid", width: 90, render: (v: number) => (v ? v : "—") },
            { title: "User", dataIndex: "user", width: 120 },
          ]}
        />
      </Card>

      <Card
        title="Web logs"
        extra={
          <Space wrap>
            <Tag color={logs?.cloudflareOK ? "success" : "default"}>{logs?.cloudflareOK ? "Cloudflare real IP" : "Real IP pending"}</Tag>
            <Tag color={logs?.goaccess ? "success" : "default"}>{logs?.goaccess ? "GoAccess" : "GoAccess not installed"}</Tag>
          </Space>
        }
      >
        <Row gutter={16}>
          <Col xs={24} md={12}>
            <Typography.Text strong>Log cut</Typography.Text>
            <Typography.Paragraph type="secondary" style={{ marginTop: 4 }}>
              Rotate nginx and Apache per-site logs daily. Files older than this many days are deleted. Default is 90.
            </Typography.Paragraph>
            <Space wrap>
              <InputNumber min={1} max={3650} value={days} onChange={(v) => setDays(Number(v || 90))} />
              <Typography.Text>days</Typography.Text>
              <Button type="primary" loading={logBusy === "save"} onClick={saveRetention}>
                Save
              </Button>
            </Space>
          </Col>
          <Col xs={24} md={12}>
            <Typography.Text strong>Cloudflare visitor IP</Typography.Text>
            <Typography.Paragraph type="secondary" style={{ marginTop: 4 }}>
              nginx real_ip uses CF-Connecting-IP. Prefixes refresh every day
              {logs?.cloudflareUpdated ? ` · last ${logs.cloudflareUpdated.replace("T", " ").replace("Z", " UTC")}` : ""}.
              {logs?.cloudflarePrefixes ? ` ${logs.cloudflarePrefixes} networks loaded.` : ""}
            </Typography.Paragraph>
            <Button loading={logBusy === "cf"} onClick={refreshCF}>
              Update Cloudflare IPs now
            </Button>
          </Col>
        </Row>
      </Card>

      <Card title="Top processes">
        <Table
          size="small"
          rowKey="pid"
          pagination={false}
          dataSource={data.top}
          columns={[
            { title: "PID", dataIndex: "pid", width: 80 },
            { title: "User", dataIndex: "user", width: 110 },
            { title: "Name", dataIndex: "name", ellipsis: true },
            { title: "CPU", render: (_, p) => `${p.cpu.toFixed(1)}%`, width: 80 },
            { title: "RSS", render: (_, p) => formatBytes(p.memory), width: 100 },
            { title: "Mem", render: (_, p) => `${p.memPct.toFixed(1)}%`, width: 80 },
          ]}
        />
      </Card>

      <Card title="User usage">
        <Table
          size="small"
          rowKey="user"
          pagination={false}
          dataSource={data.users || []}
          locale={{ emptyText: "No hosting users found." }}
          columns={[
            { title: "User", dataIndex: "user" },
            { title: "PHP", dataIndex: "phpProcesses", width: 90 },
            { title: "Processes", dataIndex: "processes", width: 110 },
            { title: "RAM", render: (_, u) => `${formatBytes(u.memory)} (${u.memPct.toFixed(1)}%)` },
            { title: "CPU", render: (_, u) => `${u.cpu.toFixed(1)}%`, width: 90 },
            { title: "Disk", render: (_, u) => formatBytes(u.diskUsed), width: 120 },
          ]}
        />
      </Card>
    </div>
  );
}
