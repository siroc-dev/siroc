import { useEffect, useState } from "react";
import { Alert, Card, Col, Progress, Row, Statistic, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes, type UserUsage } from "@/lib/usage";

type Dash = {
  accounts: number;
  sites: number;
  databases: number;
  agentOk: boolean;
  hostname?: string;
  cpuPercent?: number;
  cpuCores?: number;
  memPercent?: number;
  memUsed?: number;
  memTotal?: number;
  diskPercent?: number;
  diskUsed?: number;
  diskTotal?: number;
  load1?: number;
  load5?: number;
  load15?: number;
  uptimeSec?: number;
};

function uptime(sec: number) {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d) return `${d}d ${h}h ${m}m`;
  if (h) return `${h}h ${m}m`;
  return `${m}m`;
}

function gaugeStatus(percent: number) {
  if (percent >= 90) return "exception" as const;
  if (percent >= 75) return "normal" as const;
  return "success" as const;
}

function Gauge({ title, percent, detail }: { title: string; percent: number; detail: string }) {
  const p = Math.round(Math.min(100, Math.max(0, percent)));
  return (
    <div className="cp-gauge notranslate" translate="no">
      <Progress type="dashboard" percent={p} status={gaugeStatus(p)} size={132} />
      <div className="cp-gauge-title">{title}</div>
      <div className="cp-gauge-detail">{detail}</div>
    </div>
  );
}

export function Dashboard() {
  const [data, setData] = useState<Dash | null>(null);
  const [usage, setUsage] = useState<UserUsage[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const row = await api.get<Dash>("/api/dashboard");
        if (!stop) {
          setData(row);
          setError("");
        }
      } catch (e) {
        if (!stop) setError(e instanceof Error ? e.message : "Failed");
      }
    }
    load();
    const t = setInterval(load, 4000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);
  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const rows = await api.get<UserUsage[]>("/api/usage");
        if (!stop) setUsage(rows);
      } catch {
        if (!stop) setUsage([]);
      }
    }
    load();
    const t = setInterval(load, 4000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);
  if (error && !data) return <Alert type="error" message={error} />;
  if (!data) return <Typography.Text type="secondary">Loading…</Typography.Text>;
  const cores = data.cpuCores || 0;
  const loadPct = cores ? Math.min(100, ((data.load1 || 0) / cores) * 100) : 0;
  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Dashboard
        </Typography.Title>
        <Tag color={data.agentOk ? "success" : "warning"}>{data.agentOk ? "Agent online" : "Agent unreachable"}</Tag>
      </div>
      {typeof data.cpuPercent === "number" ? (
        <Card title="Overview" className="cp-overview notranslate" extra={data.hostname || null}>
          <div className="cp-gauges">
            <Gauge
              title="Load"
              percent={loadPct}
              detail={`${(data.load1 || 0).toFixed(2)} / ${(data.load5 || 0).toFixed(2)} / ${(data.load15 || 0).toFixed(2)}`}
            />
            <Gauge title="CPU" percent={data.cpuPercent} detail={`${cores} cores`} />
            <Gauge
              title="RAM"
              percent={data.memPercent || 0}
              detail={`${formatBytes(data.memUsed || 0)} / ${formatBytes(data.memTotal || 0)}`}
            />
            <Gauge
              title="Disk"
              percent={data.diskPercent || 0}
              detail={`${formatBytes(data.diskUsed || 0)} / ${formatBytes(data.diskTotal || 0)}`}
            />
          </div>
          {data.uptimeSec ? (
            <Typography.Text type="secondary" className="cp-overview-up">
              Up {uptime(data.uptimeSec)}
            </Typography.Text>
          ) : null}
        </Card>
      ) : null}
      <Row gutter={16}>
        <Col xs={24} md={8}>
          <Card>
            <Statistic title="Hosting accounts" value={data.accounts} />
          </Card>
        </Col>
        <Col xs={24} md={8}>
          <Card>
            <Statistic title="Websites" value={data.sites} />
          </Card>
        </Col>
        <Col xs={24} md={8}>
          <Card>
            <Statistic title="Databases" value={data.databases} />
          </Card>
        </Col>
      </Row>
      {usage.length ? (
        <Card title="User usage">
          <Table
            size="small"
            rowKey="user"
            pagination={false}
            dataSource={usage}
            columns={[
              { title: "User", dataIndex: "user" },
              { title: "PHP", dataIndex: "phpProcesses", width: 90 },
              { title: "Processes", dataIndex: "processes", width: 110 },
              { title: "RAM", render: (_, u) => `${formatBytes(u.memory)} (${u.memPct.toFixed(1)}%)`, width: 160 },
              { title: "CPU", render: (_, u) => `${u.cpu.toFixed(1)}%`, width: 90 },
              { title: "Disk", render: (_, u) => formatBytes(u.diskUsed), width: 120 },
            ]}
          />
        </Card>
      ) : null}
      <Card title="Stack">
        <Typography.Paragraph type="secondary" style={{ margin: 0 }}>
          Nginx reverse proxy on :80 → Apache on 127.0.0.1:8080 → PHP-FPM per Linux user. Install packages from Software, then create an account and website.
        </Typography.Paragraph>
      </Card>
    </div>
  );
}
