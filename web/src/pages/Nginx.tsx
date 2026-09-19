import { useEffect, useMemo, useState } from "react";
import { Alert, Card, Col, Progress, Row, Table, Tag, Typography } from "antd";
import { GlobalOutlined } from "@ant-design/icons";
import { api } from "@/lib/api";
import { StatusHero, StatusNav, fmtNum, fmtUptime } from "@/components/StatusNav";

type Site = { name: string; listen?: string; ssl?: boolean; file?: string };
type Status = {
  installed: boolean;
  active: boolean;
  ready: boolean;
  version?: string;
  uptimeSec: number;
  activeConn: number;
  accepts: number;
  handled: number;
  requests: number;
  reading: number;
  writing: number;
  waiting: number;
  reqPerSec: number;
  workerProcesses?: string;
  workerConnections?: number;
  sites?: Site[];
  fetchedAt?: string;
  message?: string;
};

export function Nginx() {
  const [data, setData] = useState<Status | null>(null);
  const [error, setError] = useState("");
  const [live, setLive] = useState(true);
  const [hist, setHist] = useState<number[]>([]);

  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const st = await api.get<Status>("/api/nginx/status");
        if (stop) return;
        setData(st);
        setError("");
        if (st.ready) setHist((h) => [...h, st.reqPerSec].slice(-40));
      } catch (e) {
        if (!stop) setError(e instanceof Error ? e.message : "Failed");
      }
    }
    load();
    if (!live) return () => { stop = true; };
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
        <Typography.Text type="secondary">Loading Nginx status…</Typography.Text>
      </div>
    );
  }
  if (!data.installed) {
    return (
      <div className="cp-page">
        <StatusNav />
        <Alert type="info" showIcon message={data.message || "Install Nginx from Software to see live status."} />
      </div>
    );
  }

  return (
    <div className="cp-page">
      <StatusNav />
      <StatusHero
        tone="nginx"
        kicker="Reverse proxy"
        icon={<GlobalOutlined />}
        title="Nginx Server Status"
        subtitle={`${data.version || "Nginx"}${data.ready && data.uptimeSec ? ` · up ${fmtUptime(data.uptimeSec)}` : ""}`}
        live={live}
        setLive={setLive}
        active={data.active}
        extra={data.workerProcesses ? <Tag color="processing">{data.workerProcesses} workers</Tag> : null}
      />
      {data.message && !data.ready ? <Alert type="warning" showIcon message={data.message} /> : null}
      {data.ready ? (
        <>
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Active connections</div>
                <div className="apache-stat-value">{data.activeConn}</div>
                <Progress
                  percent={data.workerConnections ? Math.min(100, Math.round((data.activeConn / data.workerConnections) * 100)) : 0}
                  showInfo={false}
                  strokeColor="#0f766e"
                />
                <div className="apache-stat-sub">
                  {data.reading} reading · {data.writing} writing · {data.waiting} waiting
                </div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Requests / sec</div>
                <div className="apache-stat-value">{fmtNum(data.reqPerSec)}</div>
                <div className="apache-spark">
                  {hist.map((v, i) => (
                    <i key={i} style={{ height: `${Math.max(8, (v / maxReq) * 100)}%`, background: "#0f766e" }} />
                  ))}
                </div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Total requests</div>
                <div className="apache-stat-value">{data.requests.toLocaleString()}</div>
                <div className="apache-stat-sub">{data.accepts.toLocaleString()} accepted · {data.handled.toLocaleString()} handled</div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Capacity</div>
                <div className="apache-stat-value">{data.workerConnections || "—"}</div>
                <div className="apache-stat-sub">worker_connections · processes {data.workerProcesses || "—"}</div>
              </Card>
            </Col>
          </Row>
          <Card size="small" title="Server blocks" extra={<Typography.Text type="secondary">{data.sites?.length || 0}</Typography.Text>}>
            <div className="cp-table-wrap">
            <Table
              size="small"
              rowKey={(r) => r.name + (r.listen || "") + (r.file || "")}
              pagination={false}
              dataSource={data.sites || []}
              locale={{ emptyText: "No enabled nginx sites" }}
              scroll={{ x: "max-content" }}
              columns={[
                { title: "Server name", dataIndex: "name", ellipsis: true },
                { title: "Listen", dataIndex: "listen", width: 160, responsive: ["sm"] },
                { title: "TLS", dataIndex: "ssl", width: 80, render: (v: boolean) => (v ? <Tag color="success">SSL</Tag> : "—") },
                { title: "Config", dataIndex: "file", ellipsis: true, responsive: ["md"] },
              ]}
            />
            </div>
          </Card>
          <Typography.Text type="secondary">
            stub_status is bound to 127.0.0.1 only. Public /nginx-status is blocked.
            {data.fetchedAt ? ` · ${data.fetchedAt}` : ""}
          </Typography.Text>
        </>
      ) : null}
    </div>
  );
}
