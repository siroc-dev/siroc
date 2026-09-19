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

export function Dashboard() {
  const [data, setData] = useState<Dash | null>(null);
  const [usage, setUsage] = useState<UserUsage[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    api.get<Dash>("/api/dashboard").then(setData).catch((e) => setError(e.message));
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
  if (error) return <Alert type="error" message={error} />;
  if (!data) return <Typography.Text type="secondary">Loading…</Typography.Text>;
  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Dashboard
        </Typography.Title>
        <Tag color={data.agentOk ? "success" : "warning"}>{data.agentOk ? "Agent online" : "Agent unreachable"}</Tag>
      </div>
      {typeof data.cpuPercent === "number" ? (
        <Row gutter={16}>
          <Col xs={24} md={8}>
            <Card size="small">
              <Typography.Text type="secondary">CPU</Typography.Text>
              <Progress percent={Math.round(data.cpuPercent)} status={data.cpuPercent >= 90 ? "exception" : "normal"} />
              <Typography.Text type="secondary">{data.cpuCores || 0} cores{data.hostname ? ` · ${data.hostname}` : ""}</Typography.Text>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card size="small">
              <Typography.Text type="secondary">Memory</Typography.Text>
              <Progress percent={Math.round(data.memPercent || 0)} status={(data.memPercent || 0) >= 90 ? "exception" : "normal"} />
              <Typography.Text type="secondary">
                {formatBytes(data.memUsed || 0)} / {formatBytes(data.memTotal || 0)}
              </Typography.Text>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card size="small">
              <Typography.Text type="secondary">Disk /</Typography.Text>
              <Progress percent={Math.round(data.diskPercent || 0)} status={(data.diskPercent || 0) >= 90 ? "exception" : "normal"} />
              <Typography.Text type="secondary">
                {formatBytes(data.diskUsed || 0)} / {formatBytes(data.diskTotal || 0)}
                {data.uptimeSec ? ` · up ${uptime(data.uptimeSec)}` : ""}
              </Typography.Text>
            </Card>
          </Col>
        </Row>
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
