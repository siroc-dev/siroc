import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { PlayCircleFilled, SearchOutlined } from "@ant-design/icons";
import { App, Button, Card, Input, Space, Switch, Table, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";

type Svc = {
  name: string;
  title: string;
  version: string;
  active: boolean;
  monitor: boolean;
  manage?: string;
  memory?: number;
  processes?: number;
};

export function ServiceStatus() {
  const { message } = App.useApp();
  const [svc, setSvc] = useState<Svc[]>([]);
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState("");
  const [ready, setReady] = useState(false);

  async function loadSvc() {
    setSvc(await api.get<Svc[]>("/api/settings/services"));
  }

  useEffect(() => {
    loadSvc()
      .catch((e) => message.error(e instanceof Error ? e.message : "Failed"))
      .finally(() => setReady(true));
  }, [message]);

  async function toggleMonitor(row: Svc, monitor: boolean) {
    setSvc((cur) => cur.map((s) => (s.name === row.name ? { ...s, monitor } : s)));
    try {
      await api.put("/api/settings/services/monitor", { name: row.name, monitor });
    } catch (err) {
      setSvc((cur) => cur.map((s) => (s.name === row.name ? { ...s, monitor: row.monitor } : s)));
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  async function control(row: Svc, action: "start" | "stop" | "restart") {
    setBusy("svc:" + row.name);
    try {
      await api.post("/api/software/service", { name: row.name, action });
      const done = action === "start" ? "Started" : action === "stop" ? "Stopped" : "Restarted";
      message.success(`${done} ${row.title}`);
      await loadSvc();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  const rows = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return svc;
    return svc.filter((s) => `${s.title} ${s.name} ${s.version}`.toLowerCase().includes(needle));
  }, [svc, q]);

  return (
    <Card
      title={
        <Space>
          <span>Service Status</span>
          <Button onClick={() => loadSvc().catch((e) => message.error(e instanceof Error ? e.message : "Failed"))} loading={!ready && svc.length === 0}>
            Refresh
          </Button>
        </Space>
      }
      extra={
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="Please enter the service name"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          style={{ width: 240 }}
        />
      }
    >
      <Table
        size="small"
        rowKey="name"
        loading={!ready}
        dataSource={rows}
        pagination={{ pageSize: 10, showSizeChanger: true, showTotal: (n) => `Total ${n}` }}
        columns={[
          { title: "Service Name", dataIndex: "title" },
          {
            title: "RAM",
            width: 110,
            render: (_, row) => (row.active && row.memory ? formatBytes(row.memory) : "—"),
          },
          {
            title: "Processes",
            width: 110,
            align: "right" as const,
            render: (_, row) => (row.active && row.processes ? row.processes : "—"),
          },
          {
            title: "Monitor",
            width: 110,
            render: (_, row) => <Switch checked={row.monitor} onChange={(v) => toggleMonitor(row, v)} />,
          },
          { title: "Version", dataIndex: "version", width: 180, render: (v: string) => v || "—" },
          {
            title: "Status",
            width: 140,
            render: (_, row) =>
              row.active ? (
                <Typography.Text type="success">
                  <PlayCircleFilled /> Running
                </Typography.Text>
              ) : (
                <Typography.Text type="secondary">Stopped</Typography.Text>
              ),
          },
          {
            title: "Operate",
            width: 240,
            render: (_, row) => (
              <Space>
                {row.active ? (
                  <Button type="link" size="small" loading={busy === "svc:" + row.name} onClick={() => control(row, "stop")}>
                    Stop
                  </Button>
                ) : (
                  <Button type="link" size="small" loading={busy === "svc:" + row.name} onClick={() => control(row, "start")}>
                    Start
                  </Button>
                )}
                <Button type="link" size="small" loading={busy === "svc:" + row.name} onClick={() => control(row, "restart")}>
                  Restart
                </Button>
                {row.manage ? <Link to={row.manage}>Manage</Link> : null}
              </Space>
            ),
          },
        ]}
      />
    </Card>
  );
}
