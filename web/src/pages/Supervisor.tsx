import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Form, Input, InputNumber, Modal, Space, Switch, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";

type Proc = { name: string; status: string; pid?: number; uptime?: string; detail?: string };
type Program = {
  name: string;
  command: string;
  directory?: string;
  user?: string;
  autostart: boolean;
  autorestart: boolean;
  numprocs: number;
  stdoutLog?: string;
  file?: string;
  site?: boolean;
  processes?: Proc[];
};
type Status = { installed: boolean; active: boolean; programs?: Program[] };
type Draft = {
  name: string;
  command: string;
  directory: string;
  user: string;
  autostart: boolean;
  autorestart: boolean;
  numprocs: number;
};

const page = { pageSize: 20, showSizeChanger: true, pageSizeOptions: ["10", "20", "50", "100"], showTotal: (n: number) => `Total ${n}` };

const colors: Record<string, string> = {
  RUNNING: "green",
  PARTIAL: "gold",
  STARTING: "blue",
  BACKOFF: "gold",
  STOPPING: "gold",
  FATAL: "red",
  EXITED: "red",
};

function rowStatus(row: Program) {
  const procs = row.processes || [];
  if (!procs.length) return "NOT LOADED";
  if (procs.every((item) => item.status === "RUNNING")) return "RUNNING";
  if (procs.some((item) => item.status === "RUNNING")) return "PARTIAL";
  return procs[0].status;
}

const emptyDraft: Draft = { name: "", command: "", directory: "", user: "", autostart: true, autorestart: true, numprocs: 1 };

export function Supervisor() {
  const { message, modal } = App.useApp();
  const [st, setSt] = useState<Status | null>(null);
  const [busy, setBusy] = useState("");
  const [draft, setDraft] = useState<Draft | null>(null);
  const [editing, setEditing] = useState("");
  const [log, setLog] = useState<{ name: string; text: string } | null>(null);

  async function load() {
    const data = await api.get<Status>("/api/supervisor");
    setSt(data);
  }

  useEffect(() => {
    load().catch((err) => message.error(err instanceof Error ? err.message : "Failed"));
  }, [message]);

  async function run(key: string, work: () => Promise<Status | void>) {
    setBusy(key);
    try {
      const next = await work();
      if (next) setSt(next);
      else await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  function save() {
    if (!draft) return;
    const body = {
      name: draft.name.trim(),
      command: draft.command.trim(),
      directory: draft.directory.trim(),
      user: draft.user.trim(),
      autostart: draft.autostart,
      autorestart: draft.autorestart,
      numprocs: draft.numprocs || 1,
    };
    const path = editing ? `/api/supervisor/programs/${encodeURIComponent(editing)}` : "/api/supervisor/programs";
    void run("save", async () => {
      const next = editing ? await api.put<Status>(path, body) : await api.post<Status>(path, body);
      message.success(editing ? "Program updated" : "Program added");
      setDraft(null);
      setEditing("");
      return next;
    });
  }

  const rows = st?.programs || [];
  return (
    <Card
      title="Supervisor"
      extra={
        st ? (
          <Space wrap>
            {!st.installed ? (
              <Button type="primary" loading={busy === "install"} onClick={() => void run("install", () => api.post<Status>("/api/supervisor/install", {}))}>
                Install
              </Button>
            ) : (
              <>
                <Button loading={busy === "start"} onClick={() => void run("start", () => api.post<Status>("/api/supervisor/service", { action: "start" }))}>
                  Start
                </Button>
                <Button loading={busy === "stop"} onClick={() => void run("stop", () => api.post<Status>("/api/supervisor/service", { action: "stop" }))}>
                  Stop
                </Button>
                <Button loading={busy === "restart"} onClick={() => void run("restart", () => api.post<Status>("/api/supervisor/service", { action: "restart" }))}>
                  Restart
                </Button>
                <Button loading={busy === "reload"} onClick={() => void run("reload", () => api.post<Status>("/api/supervisor/reload", {}))}>
                  Reload
                </Button>
              </>
            )}
            <Button type="primary" disabled={!st.installed} onClick={() => { setEditing(""); setDraft({ ...emptyDraft }); }}>
              Add program
            </Button>
          </Space>
        ) : null
      }
    >
      {st && !st.installed ? (
        <Alert style={{ marginBottom: 16 }} type="info" showIcon message="Supervisor is not installed. Install it to add programs and start them." />
      ) : null}
      {st?.installed ? (
        <Typography.Paragraph type="secondary">
          Service is {st.active ? "running" : "stopped"}. Programs named siroc- belong to a website and stay editable from that site.
        </Typography.Paragraph>
      ) : null}
      <Table
        rowKey="name"
        dataSource={rows}
        pagination={page}
        scroll={{ x: 980 }}
        columns={[
          {
            title: "Name",
            dataIndex: "name",
            render: (name: string, row) => (
              <Space direction="vertical" size={0}>
                <Space>
                  <span>{name}</span>
                  {row.site ? <Tag>Website</Tag> : null}
                </Space>
                <Typography.Text type="secondary">{row.user || "default user"}{row.directory ? ` · ${row.directory}` : ""}</Typography.Text>
              </Space>
            ),
          },
          {
            title: "Status",
            width: 140,
            render: (_, row) => {
              const status = rowStatus(row);
              return <Tag color={colors[status]}>{status}</Tag>;
            },
          },
          {
            title: "Command",
            dataIndex: "command",
            ellipsis: true,
            render: (command: string) => command || "—",
          },
          {
            title: "Workers",
            dataIndex: "numprocs",
            width: 100,
            render: (n: number, row) => {
              const running = (row.processes || []).filter((item) => item.status === "RUNNING").length;
              return `${running}/${n || 1}`;
            },
          },
          {
            title: "",
            width: 320,
            render: (_, row) => (
              <Space wrap>
                <Button size="small" disabled={!st?.installed} loading={busy === `start:${row.name}`} onClick={() => void run(`start:${row.name}`, () => api.post<Status>(`/api/supervisor/programs/${encodeURIComponent(row.name)}/start`, {}))}>Start</Button>
                <Button size="small" disabled={!st?.installed} loading={busy === `stop:${row.name}`} onClick={() => void run(`stop:${row.name}`, () => api.post<Status>(`/api/supervisor/programs/${encodeURIComponent(row.name)}/stop`, {}))}>Stop</Button>
                <Button size="small" disabled={!st?.installed} loading={busy === `restart:${row.name}`} onClick={() => void run(`restart:${row.name}`, () => api.post<Status>(`/api/supervisor/programs/${encodeURIComponent(row.name)}/restart`, {}))}>Restart</Button>
                <Button
                  size="small"
                  onClick={() => void run(`log:${row.name}`, async () => {
                    const out = await api.get<{ text: string }>(`/api/supervisor/programs/${encodeURIComponent(row.name)}/log`);
                    setLog({ name: row.name, text: out.text || "" });
                  })}
                >
                  Log
                </Button>
                <Button
                  size="small"
                  disabled={row.site}
                  onClick={() => {
                    setEditing(row.name);
                    setDraft({
                      name: row.name,
                      command: row.command,
                      directory: row.directory || "",
                      user: row.user || "",
                      autostart: row.autostart,
                      autorestart: row.autorestart,
                      numprocs: row.numprocs || 1,
                    });
                  }}
                >
                  Edit
                </Button>
                <Button
                  size="small"
                  danger
                  disabled={row.site}
                  onClick={() => modal.confirm({
                    title: `Delete ${row.name}?`,
                    okText: "Delete",
                    okButtonProps: { danger: true },
                    onOk: () => run(`delete:${row.name}`, async () => {
                      const next = await api.delete<Status>(`/api/supervisor/programs/${encodeURIComponent(row.name)}`);
                      message.success("Program deleted");
                      return next;
                    }),
                  })}
                >
                  Delete
                </Button>
              </Space>
            ),
          },
        ]}
      />
      <Modal
        title={editing ? `Edit ${editing}` : "Add program"}
        open={!!draft}
        onCancel={() => { setDraft(null); setEditing(""); }}
        onOk={save}
        confirmLoading={busy === "save"}
        okText="Save"
      >
        {draft ? (
          <Form layout="vertical">
            <Form.Item label="Name" required>
              <Input disabled={!!editing} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="worker" />
            </Form.Item>
            <Form.Item label="Command" required>
              <Input.TextArea rows={3} value={draft.command} onChange={(e) => setDraft({ ...draft, command: e.target.value })} placeholder="/usr/bin/php /var/www/worker.php" />
            </Form.Item>
            <Form.Item label="Directory">
              <Input value={draft.directory} onChange={(e) => setDraft({ ...draft, directory: e.target.value })} placeholder="/var/www" />
            </Form.Item>
            <Form.Item label="User">
              <Input value={draft.user} onChange={(e) => setDraft({ ...draft, user: e.target.value })} placeholder="www-data" />
            </Form.Item>
            <Form.Item label="Workers">
              <InputNumber min={1} max={32} value={draft.numprocs} onChange={(value) => setDraft({ ...draft, numprocs: value || 1 })} />
            </Form.Item>
            <Space size="large">
              <span>Autostart <Switch checked={draft.autostart} onChange={(checked) => setDraft({ ...draft, autostart: checked })} /></span>
              <span>Autorestart <Switch checked={draft.autorestart} onChange={(checked) => setDraft({ ...draft, autorestart: checked })} /></span>
            </Space>
          </Form>
        ) : null}
      </Modal>
      <Modal title={log ? `Log · ${log.name}` : "Log"} open={!!log} footer={null} onCancel={() => setLog(null)} width={760}>
        <pre style={{ maxHeight: 420, overflow: "auto", whiteSpace: "pre-wrap" }}>{log?.text || "Log is empty."}</pre>
      </Modal>
    </Card>
  );
}
