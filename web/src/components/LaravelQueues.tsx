import { useEffect, useMemo, useState } from "react";
import { Button, Input, InputNumber, List, Space, Switch, Table, Tag, Typography } from "antd";

export type LaravelQueueProc = {
  name: string;
  queue?: string;
  status: string;
  pid?: number;
  uptime?: string;
  lastRun?: string;
  nextRun?: string;
};

export type LaravelQueueRow = {
  name: string;
  workers: number;
  running?: number;
  status?: string;
  lastRun?: string;
  nextRun?: string;
  processes?: LaravelQueueProc[];
};

export type LaravelSchedule = {
  expression: string;
  command: string;
  lastRun?: string;
  nextRun?: string;
};

export type LaravelQueueState = {
  queue: boolean;
  queueName?: string;
  workers: number;
  queues?: LaravelQueueRow[];
  processes?: LaravelQueueProc[];
  scheduler: boolean;
  schedule?: LaravelSchedule[];
  scheduleLast?: string;
};

export function fmtWhen(v?: string) {
  if (!v) return "—";
  if (v === "listening") return "Listening";
  if (v === "waiting") return "Waiting";
  const t = Date.parse(v);
  if (!Number.isNaN(t)) return new Date(t).toLocaleString();
  return v;
}

function statusColor(status?: string) {
  const s = (status || "").toUpperCase();
  if (s === "RUNNING") return "success";
  if (s === "BACKOFF" || s === "STARTING" || s === "CONFIGURED") return "warning";
  if (s === "FATAL" || s === "EXITED" || s === "STOPPED") return "error";
  return "default";
}

function rowsFromLaravel(laravel: LaravelQueueState): LaravelQueueRow[] {
  const qs = laravel.queues?.filter((q) => q.name) || [];
  if (qs.length) {
    return qs.map((q) => ({ name: q.name, workers: q.workers || 1, running: q.running, status: q.status, lastRun: q.lastRun, nextRun: q.nextRun, processes: q.processes }));
  }
  const names = (laravel.queueName || "default")
    .split(",")
    .map((n) => n.trim())
    .filter(Boolean);
  const workers = laravel.workers || 1;
  return names.map((name) => ({ name, workers }));
}

export function LaravelQueues({
  laravel,
  busy,
  onToggle,
  onApply,
  onScheduler,
}: {
  laravel: LaravelQueueState;
  busy: boolean;
  onToggle: (on: boolean, rows: LaravelQueueRow[]) => void;
  onApply: (rows: LaravelQueueRow[]) => void;
  onScheduler: (on: boolean) => void;
}) {
  const [rows, setRows] = useState<LaravelQueueRow[]>(() => rowsFromLaravel(laravel));

  useEffect(() => {
    setRows(rowsFromLaravel(laravel));
  }, [laravel.queue, laravel.queueName, laravel.workers, laravel.queues]);

  const processes = useMemo(() => {
    if (laravel.processes?.length) return laravel.processes;
    return rows.flatMap((r) => r.processes || []);
  }, [laravel.processes, rows]);

  function setRow(i: number, patch: Partial<LaravelQueueRow>) {
    setRows((cur) => cur.map((r, idx) => (idx === i ? { ...r, ...patch } : r)));
  }

  return (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Space wrap align="center">
        <Typography.Text>Queue workers</Typography.Text>
        <Switch checked={!!laravel.queue} loading={busy} onChange={(on) => onToggle(on, rows.length ? rows : [{ name: "default", workers: 1 }])} />
        <Typography.Text type="secondary">One supervisor program per queue name. Workers is how many processes listen to that queue.</Typography.Text>
      </Space>
      <Table
        size="small"
        rowKey={(r, i) => `${r.name}-${i}`}
        pagination={false}
        dataSource={rows}
        columns={[
          {
            title: "Queue name",
            dataIndex: "name",
            render: (_, r, i) => (
              <Input value={r.name} disabled={busy} onChange={(e) => setRow(i, { name: e.target.value })} placeholder="default" />
            ),
          },
          {
            title: "Workers",
            dataIndex: "workers",
            width: 110,
            render: (_, r, i) => (
              <InputNumber min={1} max={8} value={r.workers} disabled={busy} onChange={(n) => setRow(i, { workers: Number(n) || 1 })} />
            ),
          },
          {
            title: "Status",
            dataIndex: "status",
            width: 120,
            render: (v: string, r) => <Tag color={statusColor(v)}>{v || (laravel.queue ? `${r.running || 0} running` : "off")}</Tag>,
          },
          {
            title: "Last run",
            dataIndex: "lastRun",
            render: (v: string) => fmtWhen(v),
          },
          {
            title: "Next",
            dataIndex: "nextRun",
            render: (v: string) => fmtWhen(v),
          },
          {
            title: "",
            width: 80,
            render: (_, __, i) => (
              <Button type="link" disabled={busy || rows.length <= 1} onClick={() => setRows((cur) => cur.filter((_, idx) => idx !== i))}>
                Remove
              </Button>
            ),
          },
        ]}
      />
      <Space wrap>
        <Button disabled={busy || rows.length >= 8} onClick={() => setRows((cur) => [...cur, { name: "", workers: 1 }])}>
          Add queue
        </Button>
        <Button type="primary" disabled={busy || !laravel.queue || rows.every((r) => !r.name.trim())} onClick={() => onApply(rows.filter((r) => r.name.trim()))}>
          Apply queues
        </Button>
      </Space>
      <div>
        <Typography.Text strong>Running processes</Typography.Text>
        <List
          size="small"
          bordered
          style={{ marginTop: 8 }}
          dataSource={processes}
          locale={{ emptyText: laravel.queue ? "No supervisor processes yet. Apply queues to start workers." : "Queue workers are off." }}
          renderItem={(p) => (
            <List.Item>
              <List.Item.Meta
                title={
                  <Space wrap>
                    <Typography.Text>{p.name}</Typography.Text>
                    {p.queue ? <Tag>{p.queue}</Tag> : null}
                    <Tag color={statusColor(p.status)}>{p.status}</Tag>
                    {p.pid ? <Typography.Text type="secondary">pid {p.pid}</Typography.Text> : null}
                    {p.uptime ? <Typography.Text type="secondary">up {p.uptime}</Typography.Text> : null}
                  </Space>
                }
                description={`Last ${fmtWhen(p.lastRun)} · Next ${fmtWhen(p.nextRun)}`}
              />
            </List.Item>
          )}
        />
      </div>
      <Space wrap align="center">
        <Typography.Text>Scheduler (cron * * * * *)</Typography.Text>
        <Switch checked={!!laravel.scheduler} loading={busy} onChange={onScheduler} />
        {laravel.scheduleLast ? <Typography.Text type="secondary">Last schedule:run {fmtWhen(laravel.scheduleLast)}</Typography.Text> : null}
      </Space>
      {laravel.scheduler ? (
        <Table
          size="small"
          rowKey={(r, i) => `${r.expression}-${r.command}-${i}`}
          pagination={false}
          dataSource={laravel.schedule || []}
          locale={{ emptyText: "No scheduled tasks returned by artisan schedule:list." }}
          columns={[
            { title: "Task", dataIndex: "command", ellipsis: true },
            { title: "Expression", dataIndex: "expression", width: 160 },
            { title: "Last run", dataIndex: "lastRun", width: 180, render: (v: string) => fmtWhen(v) },
            { title: "Next", dataIndex: "nextRun", width: 220, render: (v: string) => fmtWhen(v) },
          ]}
        />
      ) : null}
    </Space>
  );
}
