import { useEffect, useMemo, useState } from "react";
import { Alert, Card, Col, Progress, Row, Table, Tag, Typography } from "antd";
import { ThunderboltOutlined } from "@ant-design/icons";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";
import { StatusHero, StatusNav, fmtNum, fmtUptime, fmtUs } from "@/components/StatusNav";
import { PageSkeleton } from "@/components/PageSkeleton";

export type RedisSlow = { id: number; time?: string; durationUs: number; command?: string; client?: string };
export type RedisStatusData = {
  installed: boolean;
  active: boolean;
  ready: boolean;
  version?: string;
  uptimeSec: number;
  connectedClients: number;
  blockedClients: number;
  usedMemory: number;
  usedMemoryPeak: number;
  usedMemoryHuman?: string;
  maxMemory: number;
  opsPerSec: number;
  hits: number;
  misses: number;
  hitRate: number;
  keys: number;
  evictedKeys: number;
  expiredKeys: number;
  role?: string;
  slowLogSlowerThan: number;
  slowLogLen: number;
  slowLog?: RedisSlow[];
  fetchedAt?: string;
  message?: string;
};

export function RedisStatusPanel({ data, hist }: { data: RedisStatusData; hist?: number[] }) {
  const spark = hist && hist.length > 0 ? hist : [data.opsPerSec];
  const maxOps = Math.max(0.01, ...spark);
  const memPct = data.maxMemory ? Math.round((data.usedMemory / data.maxMemory) * 100) : 0;

  return (
    <div>
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card className="apache-stat" size="small">
            <div className="apache-stat-label">Memory</div>
            <div className="apache-stat-value">
              {data.usedMemoryHuman || formatBytes(data.usedMemory)}
              <span>{data.maxMemory ? ` / ${formatBytes(data.maxMemory)}` : ""}</span>
            </div>
            {data.maxMemory ? <Progress percent={Math.min(100, memPct)} showInfo={false} strokeColor={memPct >= 85 ? "#ff4d4f" : "#dc2626"} /> : null}
            <div className="apache-stat-sub">peak {formatBytes(data.usedMemoryPeak)}{data.role ? ` · ${data.role}` : ""}</div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="apache-stat" size="small">
            <div className="apache-stat-label">Ops / sec</div>
            <div className="apache-stat-value">{fmtNum(data.opsPerSec)}</div>
            <div className="apache-spark">
              {spark.map((v, i) => (
                <i key={i} style={{ height: `${Math.max(8, (v / maxOps) * 100)}%`, background: "#dc2626" }} />
              ))}
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="apache-stat" size="small">
            <div className="apache-stat-label">Clients</div>
            <div className="apache-stat-value">{data.connectedClients}</div>
            <div className="apache-stat-sub">{data.blockedClients} blocked · {data.keys.toLocaleString()} keys</div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="apache-stat" size="small">
            <div className="apache-stat-label">Cache hits</div>
            <div className="apache-stat-value">{fmtNum(data.hitRate, 1)}%</div>
            <Progress percent={Math.min(100, Math.round(data.hitRate))} showInfo={false} strokeColor="#dc2626" />
            <div className="apache-stat-sub">{data.hits.toLocaleString()} hits · {data.misses.toLocaleString()} miss</div>
          </Card>
        </Col>
      </Row>
      <Card
        size="small"
        title="Slow commands"
        extra={
          <Typography.Text type="secondary">
            {data.slowLogLen} logged
            {data.slowLogSlowerThan ? ` · slower than ${fmtUs(data.slowLogSlowerThan)}` : ""}
          </Typography.Text>
        }
        style={{ marginTop: 16 }}
      >
        <div className="cp-table-wrap">
          <Table
            size="small"
            rowKey={(r) => String(r.id) + r.time + r.command}
            pagination={false}
            dataSource={data.slowLog || []}
            locale={{ emptyText: "No slow Redis commands" }}
            scroll={{ x: "max-content" }}
            columns={[
              { title: "Id", dataIndex: "id", width: 80 },
              { title: "When", dataIndex: "time", width: 170, responsive: ["sm"] },
              { title: "Duration", dataIndex: "durationUs", width: 110, render: (v: number) => fmtUs(v) },
              { title: "Client", dataIndex: "client", width: 160, ellipsis: true, responsive: ["md"] },
              { title: "Command", dataIndex: "command", ellipsis: true },
            ]}
          />
        </div>
      </Card>
      <Typography.Text type="secondary">
        {data.evictedKeys} evicted · {data.expiredKeys} expired
        {data.fetchedAt ? ` · ${data.fetchedAt}` : ""}
      </Typography.Text>
    </div>
  );
}

export function Redis() {
  const [data, setData] = useState<RedisStatusData | null>(null);
  const [error, setError] = useState("");
  const [live, setLive] = useState(true);
  const [hist, setHist] = useState<number[]>([]);

  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const st = await api.get<RedisStatusData>("/api/redis/status");
        if (stop) return;
        setData(st);
        setError("");
        if (st.ready) setHist((h) => [...h, st.opsPerSec].slice(-40));
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

  const subtitle = useMemo(() => {
    if (!data) return "";
    return `${data.version || "Redis"}${data.ready && data.uptimeSec ? ` · up ${fmtUptime(data.uptimeSec)}` : ""}`;
  }, [data]);

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
        <PageSkeleton bare cards={4} rows={5} />
      </div>
    );
  }
  if (!data.installed) {
    return (
      <div className="cp-page">
        <StatusNav />
        <Alert type="info" showIcon message={data.message || "Install Redis from Software to see live status."} />
      </div>
    );
  }

  return (
    <div className="cp-page">
      <StatusNav />
      <StatusHero
        tone="redis"
        kicker="In-memory store"
        icon={<ThunderboltOutlined />}
        title="Redis Status"
        subtitle={subtitle}
        live={live}
        setLive={setLive}
        active={data.active}
        extra={<Tag color="processing">{data.connectedClients} clients</Tag>}
      />
      {data.message && !data.ready ? <Alert type="warning" showIcon message={data.message} /> : null}
      {data.ready ? <RedisStatusPanel data={data} hist={hist} /> : null}
    </div>
  );
}
