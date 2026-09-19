import { useEffect, useMemo, useState } from "react";
import { Alert, Card, Col, Progress, Row, Table, Tag, Typography } from "antd";
import { CodeOutlined } from "@ant-design/icons";
import { api } from "@/lib/api";
import { StatusHero, StatusNav, fmtNum, fmtUptime } from "@/components/StatusNav";

type Proc = { pid: number; state?: string; method?: string; uri?: string; user?: string; script?: string; duration?: number; pool?: string };
type Pool = {
  name: string;
  version?: string;
  listen?: string;
  pm?: string;
  startSince: number;
  accepted: number;
  listenQueue: number;
  idle: number;
  active: number;
  total: number;
  maxActive: number;
  maxChildrenReached: number;
  slowRequests: number;
  ready: boolean;
  message?: string;
  processes?: Proc[];
};
type Status = {
  installed: boolean;
  active: boolean;
  ready: boolean;
  versions?: string[];
  pools?: Pool[];
  idle: number;
  activeProcs: number;
  total: number;
  accepted: number;
  fetchedAt?: string;
  message?: string;
};

export function PHPFPM() {
  const [data, setData] = useState<Status | null>(null);
  const [error, setError] = useState("");
  const [live, setLive] = useState(true);
  const [hist, setHist] = useState<number[]>([]);

  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const st = await api.get<Status>("/api/php-fpm/status");
        if (stop) return;
        setData(st);
        setError("");
        if (st.ready) setHist((h) => [...h, st.activeProcs].slice(-40));
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

  const maxActive = useMemo(() => Math.max(1, ...hist), [hist]);
  const requests = useMemo(() => (data?.pools || []).flatMap((p) => (p.processes || []).map((r) => ({ ...r, pool: p.name }))), [data]);

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
        <Typography.Text type="secondary">Loading PHP-FPM status…</Typography.Text>
      </div>
    );
  }
  if (!data.installed) {
    return (
      <div className="cp-page">
        <StatusNav />
        <Alert type="info" showIcon message={data.message || "Install PHP from Software to see live status."} />
      </div>
    );
  }

  return (
    <div className="cp-page">
      <StatusNav />
      <StatusHero
        tone="php"
        kicker="FastCGI"
        icon={<CodeOutlined />}
        title="PHP-FPM Status"
        subtitle={(data.versions || []).map((v) => `PHP ${v}`).join(" · ") || "PHP-FPM"}
        live={live}
        setLive={setLive}
        active={data.active}
        extra={<Tag color="processing">{data.total} workers</Tag>}
      />
      {data.message && !data.ready ? <Alert type="warning" showIcon message={data.message} /> : null}
      {data.ready ? (
        <>
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Active workers</div>
                <div className="apache-stat-value">
                  {data.activeProcs}
                  <span> / {data.total}</span>
                </div>
                <Progress percent={data.total ? Math.round((data.activeProcs / data.total) * 100) : 0} showInfo={false} strokeColor="#7c3aed" />
                <div className="apache-stat-sub">{data.idle} idle</div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Busy (live)</div>
                <div className="apache-stat-value">{fmtNum(data.activeProcs, 0)}</div>
                <div className="apache-spark">
                  {hist.map((v, i) => (
                    <i key={i} style={{ height: `${Math.max(8, (v / maxActive) * 100)}%`, background: "#7c3aed" }} />
                  ))}
                </div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Accepted</div>
                <div className="apache-stat-value">{data.accepted.toLocaleString()}</div>
                <div className="apache-stat-sub">{data.pools?.length || 0} pools</div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Versions</div>
                <div className="apache-stat-value">{data.versions?.length || 0}</div>
                <div className="apache-stat-sub">{(data.versions || []).join(", ")}</div>
              </Card>
            </Col>
          </Row>
          <Card size="small" title="Pools">
            <div className="cp-table-wrap">
            <Table
              size="small"
              rowKey={(r) => r.name + (r.version || "") + (r.listen || "")}
              pagination={false}
              dataSource={data.pools || []}
              scroll={{ x: "max-content" }}
              columns={[
                { title: "Pool", dataIndex: "name", ellipsis: true },
                { title: "PHP", dataIndex: "version", width: 80 },
                { title: "PM", dataIndex: "pm", width: 110, responsive: ["sm"] },
                { title: "Active", dataIndex: "active", width: 80 },
                { title: "Idle", dataIndex: "idle", width: 80, responsive: ["sm"] },
                { title: "Total", dataIndex: "total", width: 80, responsive: ["md"] },
                { title: "Accepted", dataIndex: "accepted", width: 110, responsive: ["md"], render: (v: number) => v.toLocaleString() },
                { title: "Up", dataIndex: "startSince", width: 110, responsive: ["lg"], render: (v: number) => (v ? fmtUptime(v) : "—") },
                {
                  title: "Status",
                  width: 140,
                  render: (_, r) => (r.ready ? <Tag color="success">Ready</Tag> : <Tag>{r.message || "Unavailable"}</Tag>),
                },
              ]}
            />
            </div>
          </Card>
          <Card size="small" title="Current requests" extra={<Typography.Text type="secondary">{requests.length}</Typography.Text>}>
            <div className="cp-table-wrap">
            <Table
              size="small"
              rowKey={(r) => r.pool + "-" + r.pid + "-" + (r.uri || "")}
              pagination={false}
              dataSource={requests}
              locale={{ emptyText: "No in-flight PHP requests" }}
              scroll={{ x: "max-content" }}
              columns={[
                { title: "State", dataIndex: "state", width: 110, render: (v: string) => <Tag color={v === "Running" ? "purple" : "default"}>{v || "—"}</Tag> },
                { title: "PID", dataIndex: "pid", width: 80, responsive: ["sm"] },
                { title: "Pool", dataIndex: "pool", width: 160, ellipsis: true, responsive: ["md"] },
                { title: "Method", dataIndex: "method", width: 90, responsive: ["sm"] },
                { title: "URI", dataIndex: "uri", ellipsis: true },
                { title: "Script", dataIndex: "script", ellipsis: true, responsive: ["lg"] },
                { title: "µs", dataIndex: "duration", width: 90, responsive: ["md"], render: (v: number) => (v ? v.toLocaleString() : "—") },
              ]}
            />
            </div>
          </Card>
          <Typography.Text type="secondary">
            Status is read from each FPM socket. Public /fpm-status is blocked on nginx.
            {data.fetchedAt ? ` · ${data.fetchedAt}` : ""}
          </Typography.Text>
        </>
      ) : null}
    </div>
  );
}
