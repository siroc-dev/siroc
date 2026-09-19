import { useEffect, useMemo, useState } from "react";
import { Alert, Card, Col, Progress, Row, Table, Tag, Typography } from "antd";
import { DatabaseOutlined } from "@ant-design/icons";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";
import { StatusHero, StatusNav, fmtNum, fmtUptime } from "@/components/StatusNav";

type Proc = { id: number; user?: string; host?: string; db?: string; command?: string; time: number; state?: string; info?: string };
type Status = {
  engine: string;
  installed: boolean;
  active: boolean;
  ready: boolean;
  version?: string;
  comment?: string;
  uptimeSec: number;
  threadsConnected: number;
  threadsRunning: number;
  maxUsedConnections: number;
  maxConnections: number;
  questions: number;
  slowQueries: number;
  connections: number;
  abortedConnects: number;
  bytesReceived: number;
  bytesSent: number;
  qps: number;
  bytesPerSec: number;
  openTables: number;
  dataDir?: string;
  processes?: Proc[];
  fetchedAt?: string;
  message?: string;
};

export function DatabaseEngine({ engine }: { engine: "mysql" | "mariadb" }) {
  const [data, setData] = useState<Status | null>(null);
  const [error, setError] = useState("");
  const [live, setLive] = useState(true);
  const [hist, setHist] = useState<number[]>([]);
  const title = engine === "mysql" ? "MySQL" : "MariaDB";

  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const st = await api.get<Status>(`/api/${engine}/status`);
        if (stop) return;
        setData(st);
        setError("");
        if (st.ready) setHist((h) => [...h, st.qps].slice(-40));
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
  }, [live, engine]);

  const maxQ = useMemo(() => Math.max(0.01, ...hist), [hist]);

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
        <Typography.Text type="secondary">Loading {title} status…</Typography.Text>
      </div>
    );
  }
  if (!data.installed) {
    return (
      <div className="cp-page">
        <StatusNav />
        <Alert type="info" showIcon message={data.message || `Install ${title} from Software to see live status.`} />
      </div>
    );
  }

  const usedPct = data.maxConnections ? Math.round((data.threadsConnected / data.maxConnections) * 100) : 0;

  return (
    <div className="cp-page">
      <StatusNav />
      <StatusHero
        tone={engine}
        kicker="SQL engine"
        icon={<DatabaseOutlined />}
        title={`${title} Status`}
        subtitle={`${data.version || title}${data.comment ? ` · ${data.comment}` : ""}${data.ready && data.uptimeSec ? ` · up ${fmtUptime(data.uptimeSec)}` : ""}`}
        live={live}
        setLive={setLive}
        active={data.active}
        extra={<Tag color="processing">{data.threadsConnected} threads</Tag>}
      />
      {data.message && !data.ready ? <Alert type="warning" showIcon message={data.message} /> : null}
      {data.ready ? (
        <>
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Connections</div>
                <div className="apache-stat-value">
                  {data.threadsConnected}
                  <span> / {data.maxConnections || "—"}</span>
                </div>
                <Progress percent={Math.min(100, usedPct)} showInfo={false} strokeColor={usedPct >= 85 ? "#ff4d4f" : engine === "mysql" ? "#ea580c" : "#0284c7"} />
                <div className="apache-stat-sub">{data.threadsRunning} running · peak {data.maxUsedConnections}</div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Queries / sec</div>
                <div className="apache-stat-value">{fmtNum(data.qps)}</div>
                <div className="apache-spark">
                  {hist.map((v, i) => (
                    <i key={i} style={{ height: `${Math.max(8, (v / maxQ) * 100)}%`, background: engine === "mysql" ? "#ea580c" : "#0284c7" }} />
                  ))}
                </div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Traffic</div>
                <div className="apache-stat-value">{formatBytes(data.bytesPerSec)}/s</div>
                <div className="apache-stat-sub">
                  {formatBytes(data.bytesReceived)} in · {formatBytes(data.bytesSent)} out
                </div>
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="apache-stat" size="small">
                <div className="apache-stat-label">Questions</div>
                <div className="apache-stat-value">{data.questions.toLocaleString()}</div>
                <div className="apache-stat-sub">{data.slowQueries} slow · {data.abortedConnects} aborted</div>
              </Card>
            </Col>
          </Row>
          <Card size="small" title="Process list" extra={<Typography.Text type="secondary">{data.processes?.length || 0}</Typography.Text>}>
            <div className="cp-table-wrap">
            <Table
              size="small"
              rowKey={(r) => String(r.id) + r.user + r.info}
              pagination={false}
              dataSource={data.processes || []}
              locale={{ emptyText: "No client threads" }}
              scroll={{ x: "max-content" }}
              columns={[
                { title: "Id", dataIndex: "id", width: 80 },
                { title: "User", dataIndex: "user", width: 110, ellipsis: true },
                { title: "Host", dataIndex: "host", width: 160, ellipsis: true, responsive: ["md"] },
                { title: "DB", dataIndex: "db", width: 120, ellipsis: true, responsive: ["sm"] },
                { title: "Command", dataIndex: "command", width: 110, render: (v: string) => <Tag>{v || "—"}</Tag> },
                { title: "Time", dataIndex: "time", width: 80, responsive: ["sm"], render: (v: number) => `${v}s` },
                { title: "State", dataIndex: "state", width: 140, ellipsis: true, responsive: ["lg"] },
                { title: "Info", dataIndex: "info", ellipsis: true, responsive: ["md"] },
              ]}
            />
            </div>
          </Card>
          <Typography.Text type="secondary">
            {data.dataDir ? `datadir ${data.dataDir} · ` : ""}
            {data.openTables} open tables
            {data.fetchedAt ? ` · ${data.fetchedAt}` : ""}
          </Typography.Text>
        </>
      ) : null}
    </div>
  );
}

export function MySQL() {
  return <DatabaseEngine engine="mysql" />;
}

export function MariaDB() {
  return <DatabaseEngine engine="mariadb" />;
}
