import { useEffect, useMemo, useState } from "react";
import { Alert, Card, Col, Progress, Row, Space, Switch, Table, Tag, Tooltip, Typography } from "antd";
import { CloudServerOutlined } from "@ant-design/icons";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";
import { StatusNav } from "@/components/StatusNav";

type Score = { key: string; title: string; count?: number };
type Worker = {
  srv: string;
  pid: number;
  acc?: string;
  state: string;
  label: string;
  cpu: number;
  ss: number;
  reqMs: number;
  client?: string;
  protocol?: string;
  vhost?: string;
  request?: string;
};
type VHost = { name: string; port: number; file?: string };
type Status = {
  installed: boolean;
  active: boolean;
  ready: boolean;
  version?: string;
  serverVersion?: string;
  mpm?: string;
  built?: string;
  currentTime?: string;
  restartTime?: string;
  uptimeSec: number;
  totalAccesses: number;
  totalBytes: number;
  cpuLoad: number;
  reqPerSec: number;
  bytesPerSec: number;
  bytesPerReq: number;
  busyWorkers: number;
  idleWorkers: number;
  totalWorkers: number;
  busyPercent: number;
  connsTotal: number;
  connsWriting: number;
  connsKeepAlive: number;
  connsClosing: number;
  load1: number;
  load5: number;
  load15: number;
  scoreboard?: string;
  scoreCounts?: Score[];
  scoreLegend?: Score[];
  workers?: Worker[];
  vhosts?: VHost[];
  fetchedAt?: string;
  message?: string;
};

const scoreColor: Record<string, string> = {
  W: "#4f46e5",
  K: "#16a34a",
  R: "#0ea5e9",
  S: "#38bdf8",
  D: "#f59e0b",
  C: "#f97316",
  L: "#a855f7",
  G: "#14b8a6",
  I: "#2dd4bf",
  _: "#94a3b8",
  ".": "#e2e8f0",
};

function uptime(sec: number) {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (d) return `${d}d ${h}h ${m}m`;
  if (h) return `${h}h ${m}m ${s}s`;
  if (m) return `${m}m ${s}s`;
  return `${s}s`;
}

function fmt(n: number, digits = 2) {
  if (!Number.isFinite(n)) return "0";
  if (Math.abs(n) >= 100) return n.toFixed(0);
  if (Math.abs(n) >= 10) return n.toFixed(1);
  return n.toFixed(digits);
}

function stateTag(state: string, label: string) {
  const color =
    state === "W" ? "geekblue" : state === "K" ? "green" : state === "R" ? "blue" : state === "C" ? "orange" : "default";
  return <Tag color={color}>{label}</Tag>;
}

export function Apache() {
  const [data, setData] = useState<Status | null>(null);
  const [error, setError] = useState("");
  const [live, setLive] = useState(true);
  const [hist, setHist] = useState<number[]>([]);

  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const st = await api.get<Status>("/api/apache/status");
        if (stop) return;
        setData(st);
        setError("");
        if (st.ready) {
          setHist((h) => [...h, st.reqPerSec].slice(-40));
        }
      } catch (e) {
        if (!stop) setError(e instanceof Error ? e.message : "Failed");
      }
    }
    load();
    if (!live) {
      return () => {
        stop = true;
      };
    }
    const t = setInterval(load, 2500);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, [live]);

  const maxReq = useMemo(() => Math.max(0.01, ...hist), [hist]);

  if (error && !data) {
    return (
      <div className="cp-page">
        <StatusNav />
        <Alert type="error" showIcon message={error} />
      </div>
    );
  }
  if (!data) {
    return (
      <div className="cp-page">
        <StatusNav />
        <Typography.Text type="secondary">Loading Apache status…</Typography.Text>
      </div>
    );
  }

  if (!data.installed) {
    return (
      <div className="cp-page">
        <StatusNav />
        <Alert type="info" showIcon message={data.message || "Install Apache from Software to see live status."} />
      </div>
    );
  }

  return (
    <div className="cp-page apache-status">
      <StatusNav />
      <div className="apache-hero">
        <div>
          <div className="apache-kicker">
            <CloudServerOutlined /> HTTP engine
          </div>
          <Typography.Title level={3} style={{ margin: "4px 0 6px", color: "#fff" }}>
            Apache Server Status
          </Typography.Title>
          <Typography.Text style={{ color: "rgba(255,255,255,0.78)" }}>
            {data.serverVersion || data.version || "Apache"}
            {data.mpm ? ` · MPM ${data.mpm}` : ""}
            {data.ready && data.uptimeSec ? ` · up ${uptime(data.uptimeSec)}` : ""}
          </Typography.Text>
        </div>
        <Space wrap align="center">
          <span className={live && data.active ? "apache-live" : "apache-live is-off"}>
            <i />
            {live && data.active ? "Live" : data.active ? "Paused" : "Offline"}
          </span>
          <Tag color={data.active ? "success" : "error"}>{data.active ? "Running" : "Stopped"}</Tag>
          {data.ready ? <Tag color="processing">{data.totalWorkers} workers</Tag> : null}
          <Switch checked={live} onChange={setLive} checkedChildren="Live" unCheckedChildren="Paused" />
        </Space>
      </div>

      {data.message && !data.ready ? <Alert type="warning" showIcon message={data.message} /> : null}

      {data.ready ? (
        <>
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Busy workers</div>
                <div className="apache-stat-value">
                  {data.busyWorkers}
                  <span> / {data.totalWorkers}</span>
                </div>
                <Progress
                  percent={Math.round(data.busyPercent)}
                  showInfo={false}
                  strokeColor={data.busyPercent >= 85 ? "#ff4d4f" : "#4f46e5"}
                />
                <div className="apache-stat-sub">{data.idleWorkers} idle</div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Requests / sec</div>
                <div className="apache-stat-value">{fmt(data.reqPerSec)}</div>
                <div className="apache-spark">
                  {hist.map((v, i) => (
                    <i key={i} style={{ height: `${Math.max(8, (v / maxReq) * 100)}%` }} />
                  ))}
                </div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Traffic</div>
                <div className="apache-stat-value">{formatBytes(data.bytesPerSec)}/s</div>
                <div className="apache-stat-sub">
                  {formatBytes(data.totalBytes)} total · {fmt(data.bytesPerReq, 0)} B/req
                </div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Connections</div>
                <div className="apache-stat-value">{data.connsTotal}</div>
                <div className="apache-stat-sub">
                  {data.connsKeepAlive} keepalive · {data.connsWriting} writing · {data.connsClosing} closing
                </div>
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]}>
            <Col xs={24} md={8}>
              <Card size="small" title="Load">
                <Space size="large">
                  <div>
                    <div className="apache-stat-label">1 min</div>
                    <div className="apache-mini">{fmt(data.load1)}</div>
                  </div>
                  <div>
                    <div className="apache-stat-label">5 min</div>
                    <div className="apache-mini">{fmt(data.load5)}</div>
                  </div>
                  <div>
                    <div className="apache-stat-label">15 min</div>
                    <div className="apache-mini">{fmt(data.load15)}</div>
                  </div>
                </Space>
                <div className="apache-stat-sub" style={{ marginTop: 12 }}>
                  CPU load {fmt(data.cpuLoad)} · {data.totalAccesses.toLocaleString()} requests
                </div>
              </Card>
            </Col>
            <Col xs={24} md={16}>
              <Card
                size="small"
                title="Scoreboard"
                extra={<Typography.Text type="secondary">{data.scoreboard?.length || 0} slots</Typography.Text>}
              >
                <div className="apache-board">
                  {(data.scoreboard || "").split("").map((ch, i) =>
                    ch === " " || ch === "\n" ? null : (
                      <Tooltip key={i} title={`${scoreTitle(ch, data.scoreLegend)} (${ch})`}>
                        <span className="apache-cell" style={{ background: scoreColor[ch] || "#64748b" }} />
                      </Tooltip>
                    ),
                  )}
                </div>
                <div className="apache-legend">
                  {(data.scoreCounts || []).map((s) => (
                    <span key={s.key}>
                      <i style={{ background: scoreColor[s.key] || "#64748b" }} />
                      {s.title} {s.count}
                    </span>
                  ))}
                </div>
              </Card>
            </Col>
          </Row>

          <Card size="small" title="Current requests" extra={<Typography.Text type="secondary">{data.workers?.length || 0} busy / keepalive</Typography.Text>}>
            <div className="cp-table-wrap">
            <Table
              size="small"
              rowKey={(r) => r.srv + "-" + r.pid + "-" + r.request}
              pagination={false}
              dataSource={data.workers || []}
              locale={{ emptyText: "No in-flight requests" }}
              scroll={{ x: "max-content" }}
              columns={[
                { title: "State", dataIndex: "state", width: 130, render: (_, r) => stateTag(r.state, r.label) },
                {
                  title: "PID",
                  dataIndex: "pid",
                  width: 80,
                  responsive: ["sm"],
                  render: (v: number) => (v ? v : "—"),
                },
                { title: "Client", dataIndex: "client", width: 140, ellipsis: true, responsive: ["md"] },
                { title: "VHost", dataIndex: "vhost", width: 180, ellipsis: true, responsive: ["lg"] },
                {
                  title: "Request",
                  dataIndex: "request",
                  ellipsis: true,
                  render: (v: string, r: Worker) => (
                    <Tooltip title={[r.protocol, v].filter(Boolean).join(" · ")}>
                      <span>{v || "—"}</span>
                    </Tooltip>
                  ),
                },
                { title: "CPU", dataIndex: "cpu", width: 70, responsive: ["md"], render: (v: number) => fmt(v) },
                { title: "Age", dataIndex: "ss", width: 70, responsive: ["sm"], render: (v: number) => `${v}s` },
                { title: "ms", dataIndex: "reqMs", width: 70, responsive: ["md"], render: (v: number) => (v ? `${v}` : "—") },
              ]}
            />
            </div>
          </Card>

          <Card size="small" title="Virtual hosts" extra={<Typography.Text type="secondary">{data.vhosts?.length || 0}</Typography.Text>}>
            <div className="cp-table-wrap">
            <Table
              size="small"
              rowKey={(r) => r.name + r.port + (r.file || "")}
              pagination={false}
              dataSource={data.vhosts || []}
              locale={{ emptyText: "No name-based virtual hosts" }}
              scroll={{ x: "max-content" }}
              columns={[
                { title: "ServerName", dataIndex: "name", ellipsis: true },
                { title: "Port", dataIndex: "port", width: 90 },
                { title: "Config", dataIndex: "file", ellipsis: true, responsive: ["md"] },
              ]}
            />
            </div>
          </Card>
          <Typography.Text type="secondary">
            Status is read from 127.0.0.1 only. Public /server-status is blocked on nginx.
            {data.fetchedAt ? ` · ${data.fetchedAt}` : ""}
          </Typography.Text>
        </>
      ) : null}
    </div>
  );
}

function scoreTitle(ch: string, legend?: Score[]) {
  const hit = legend?.find((s) => s.key === ch);
  return hit?.title || ch;
}
