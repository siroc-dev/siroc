import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Alert, Card, Col, Row, Space, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { fmtMs, fmtNum } from "@/components/StatusNav";
import { RedisStatusPanel, type RedisStatusData } from "@/pages/RedisStatus";

type SlowQuery = {
  schema?: string;
  query: string;
  count: number;
  avgMs: number;
  maxMs: number;
  rowsExamined: number;
  lastSeen?: string;
};
type Proc = { id: number; user?: string; host?: string; db?: string; command?: string; time: number; state?: string; info?: string };
type SQLMonitor = {
  engine: string;
  installed: boolean;
  active: boolean;
  ready: boolean;
  version?: string;
  slowQueryLog: boolean;
  longQueryTime?: string;
  slowLogFile?: string;
  logOutput?: string;
  logQueriesNotUsingIndexes: boolean;
  slowQueries: number;
  questions: number;
  qps: number;
  threadsConnected: number;
  threadsRunning: number;
  recent?: SlowQuery[];
  running?: Proc[];
  fetchedAt?: string;
  message?: string;
};
export type DatabasesMonitorData = { sql?: SQLMonitor; redis?: RedisStatusData };

export function DatabaseMonitorPanel() {
  const [data, setData] = useState<DatabasesMonitorData | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let stop = false;
    async function load() {
      try {
        const st = await api.get<DatabasesMonitorData>("/api/databases/monitor");
        if (stop) return;
        setData(st);
        setError("");
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

  if (error && !data) return <Alert type="error" showIcon message={error} />;
  if (!data) return <Typography.Text type="secondary">Loading database monitor…</Typography.Text>;

  const sql = data.sql;
  const redis = data.redis;
  const engineLabel = sql?.engine === "mariadb" ? "MariaDB" : sql?.engine === "mysql" ? "MySQL" : "SQL";
  const enginePath = sql?.engine === "mariadb" ? "/mariadb" : "/mysql";

  return (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Card
        size="small"
        title={`${engineLabel} slow queries`}
        extra={sql?.engine ? <Link to={enginePath}>Full {engineLabel} status</Link> : null}
      >
        {!sql?.installed ? (
          <Alert type="info" showIcon message={sql?.message || "Install MySQL or MariaDB first"} />
        ) : !sql.active || !sql.ready ? (
          <Alert type="warning" showIcon message={sql.message || `${engineLabel} is not running`} />
        ) : (
          <Space direction="vertical" size={16} style={{ width: "100%" }}>
            {sql.message ? <Alert type="info" showIcon message={sql.message} /> : null}
            <Row gutter={[16, 16]}>
              <Col xs={24} sm={12} lg={6}>
                <Card className="apache-stat" size="small">
                  <div className="apache-stat-label">Slow query log</div>
                  <div className="apache-stat-value">
                    <Tag color={sql.slowQueryLog ? "success" : "default"}>{sql.slowQueryLog ? "On" : "Off"}</Tag>
                  </div>
                  <div className="apache-stat-sub">threshold {sql.longQueryTime || "—"}s</div>
                </Card>
              </Col>
              <Col xs={24} sm={12} lg={6}>
                <Card className="apache-stat" size="small">
                  <div className="apache-stat-label">Slow queries</div>
                  <div className="apache-stat-value">{sql.slowQueries.toLocaleString()}</div>
                  <div className="apache-stat-sub">{sql.questions.toLocaleString()} questions</div>
                </Card>
              </Col>
              <Col xs={24} sm={12} lg={6}>
                <Card className="apache-stat" size="small">
                  <div className="apache-stat-label">Queries / sec</div>
                  <div className="apache-stat-value">{fmtNum(sql.qps)}</div>
                  <div className="apache-stat-sub">{sql.threadsRunning} running · {sql.threadsConnected} connected</div>
                </Card>
              </Col>
              <Col xs={24} sm={12} lg={6}>
                <Card className="apache-stat" size="small">
                  <div className="apache-stat-label">Unused indexes</div>
                  <div className="apache-stat-value">
                    <Tag color={sql.logQueriesNotUsingIndexes ? "warning" : "default"}>{sql.logQueriesNotUsingIndexes ? "Logged" : "Off"}</Tag>
                  </div>
                  <div className="apache-stat-sub">{sql.logOutput || "FILE"}{sql.slowLogFile ? ` · ${sql.slowLogFile}` : ""}</div>
                </Card>
              </Col>
            </Row>
            {sql.running && sql.running.length > 0 ? (
              <Card size="small" title="Running long queries" extra={<Typography.Text type="secondary">{sql.running.length}</Typography.Text>}>
                <div className="cp-table-wrap">
                  <Table
                    size="small"
                    rowKey={(r) => String(r.id) + r.info}
                    pagination={false}
                    dataSource={sql.running}
                    scroll={{ x: "max-content" }}
                    columns={[
                      { title: "Id", dataIndex: "id", width: 80 },
                      { title: "User", dataIndex: "user", width: 110, ellipsis: true },
                      { title: "DB", dataIndex: "db", width: 120, ellipsis: true, responsive: ["sm"] },
                      { title: "Time", dataIndex: "time", width: 80, render: (v: number) => `${v}s` },
                      { title: "State", dataIndex: "state", width: 140, ellipsis: true, responsive: ["lg"] },
                      { title: "Query", dataIndex: "info", ellipsis: true },
                    ]}
                  />
                </div>
              </Card>
            ) : null}
            <Card size="small" title="Heaviest statements" extra={<Typography.Text type="secondary">{sql.recent?.length || 0}</Typography.Text>}>
              <div className="cp-table-wrap">
                <Table
                  size="small"
                  rowKey={(r) => (r.schema || "") + r.query + r.lastSeen}
                  pagination={false}
                  dataSource={sql.recent || []}
                  locale={{ emptyText: "No statement digest yet. Enable Performance Schema or wait for traffic." }}
                  scroll={{ x: "max-content" }}
                  columns={[
                    { title: "Schema", dataIndex: "schema", width: 110, ellipsis: true, responsive: ["md"], render: (v: string) => v || "—" },
                    { title: "Count", dataIndex: "count", width: 80, render: (v: number) => v.toLocaleString() },
                    { title: "Avg", dataIndex: "avgMs", width: 90, render: (v: number) => fmtMs(v) },
                    { title: "Max", dataIndex: "maxMs", width: 90, render: (v: number) => fmtMs(v) },
                    { title: "Rows", dataIndex: "rowsExamined", width: 90, responsive: ["lg"], render: (v: number) => v.toLocaleString() },
                    { title: "Last", dataIndex: "lastSeen", width: 160, responsive: ["sm"] },
                    { title: "Query", dataIndex: "query", ellipsis: true },
                  ]}
                />
              </div>
            </Card>
          </Space>
        )}
      </Card>
      <Card size="small" title="Redis" extra={<Link to="/redis">Full Redis status</Link>}>
        {!redis?.installed ? (
          <Alert type="info" showIcon message={redis?.message || "Install Redis from Software first"} />
        ) : !redis.active || !redis.ready ? (
          <Alert type="warning" showIcon message={redis.message || "Redis is not running"} />
        ) : (
          <RedisStatusPanel data={redis} />
        )}
      </Card>
    </Space>
  );
}
