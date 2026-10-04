import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";

type Account = { username: string };
type Dest = {
  id: number;
  name: string;
  kind: string;
  host?: string;
  path?: string;
  bucket?: string;
  endpoint?: string;
};
type Job = {
  id: number;
  account: string;
  destName?: string;
  status: string;
  message?: string;
  size: number;
  localPath?: string;
  remote?: string;
  createdAt: string;
};
type Cron = {
  id: number;
  name: string;
  account: string;
  destName?: string;
  includeDB: boolean;
  cycle: string;
  minute: number;
  hour: number;
  weekday: number;
  monthday: number;
  retain: number;
  lastRun?: string;
  lastStatus?: string;
};

const weekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

function cronWhen(c: Cron) {
  const hm = `${String(c.hour).padStart(2, "0")}:${String(c.minute).padStart(2, "0")}`;
  if (c.cycle === "hourly") return `Hourly at :${String(c.minute).padStart(2, "0")}`;
  if (c.cycle === "weekly") return `Weekly on ${weekdays[c.weekday] || "Sunday"} at ${hm}`;
  if (c.cycle === "monthly") return `Monthly on day ${c.monthday} at ${hm}`;
  return `Daily at ${hm}`;
}

export function Backup() {
  const { message } = App.useApp();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [dests, setDests] = useState<Dest[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [crons, setCrons] = useState<Cron[]>([]);
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [runOpen, setRunOpen] = useState(false);
  const [cronOpen, setCronOpen] = useState(false);
  const [form] = Form.useForm();
  const [runForm] = Form.useForm();
  const [cronForm] = Form.useForm();
  const kind = Form.useWatch("kind", form);
  const cronCycle = Form.useWatch("cycle", cronForm);

  async function load() {
    const [a, d, j, crons] = await Promise.all([
      api.get<Account[]>("/api/accounts"),
      api.get<Dest[]>("/api/backup/dests").catch(() => [] as Dest[]),
      api.get<Job[]>("/api/backup/jobs"),
      api.get<Cron[]>("/api/backup/crons").catch(() => [] as Cron[]),
    ]);
    setAccounts(a);
    setDests(d);
    setJobs(j);
    setCrons(crons);
  }
  useEffect(() => {
    load().catch((e) => message.error(e.message));
  }, []);

  async function saveDest(values: Record<string, unknown>) {
    setBusy(true);
    try {
      await api.post("/api/backup/dests", values);
      form.resetFields();
      setOpen(false);
      message.success("Destination saved");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  const [restoreOpen, setRestoreOpen] = useState(false);
  const [restorePath, setRestorePath] = useState("");

  async function run(values: { username: string; destId: number; includeDB: boolean }) {
    setBusy(true);
    try {
      await api.post("/api/backup/run", values);
      setRunOpen(false);
      message.success(values.username === "*" ? "Created one archive per account" : "Backup finished");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function restore(path: string, username?: string) {
    if (!path) return;
    setBusy(true);
    try {
      const out = await api.post<{ createdUser?: boolean; message?: string; manifest?: { username?: string } }>("/api/backup/restore", {
        username: username || "",
        path,
      });
      const name = out.manifest?.username || username || "account";
      message.success(out.createdUser ? `Created user ${name} and restored` : out.message || `Restored ${name}`);
      setRestoreOpen(false);
      setRestorePath("");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            Backup
          </Typography.Title>
          <Typography.Text type="secondary">One tar.gz per hosting user. Restore recreates a missing Linux user and their sites and databases.</Typography.Text>
        </div>
        <Space>
          <Button onClick={() => setOpen(true)}>Add destination</Button>
          <Button onClick={() => setRestoreOpen(true)}>Restore archive</Button>
          <Button onClick={() => setCronOpen(true)} disabled={!dests.length}>
            Add schedule
          </Button>
          <Button type="primary" onClick={() => setRunOpen(true)} disabled={!dests.length}>
            Run backup
          </Button>
        </Space>
      </div>
      <Card title="Destinations">
        <Table
          rowKey="id"
          dataSource={dests}
          pagination={false}
          locale={{ emptyText: "No destinations yet." }}
          columns={[
            { title: "Name", dataIndex: "name" },
            { title: "Kind", dataIndex: "kind", render: (v) => <Tag>{v}</Tag> },
            { title: "Target", render: (_, d) => d.bucket || d.host || d.path || "—" },
            {
              title: "",
              align: "right",
              render: (_, d) => (
                <Popconfirm title="Delete destination?" onConfirm={() => api.delete(`/api/backup/dests/${d.id}`).then(load)}>
                  <Button size="small" danger>
                    Delete
                  </Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
      <Card title="Schedules">
        <Table
          rowKey="id"
          dataSource={crons}
          pagination={false}
          locale={{ emptyText: "No backup schedule yet. Add one to install a cron job." }}
          columns={[
            { title: "Name", dataIndex: "name" },
            { title: "Account", dataIndex: "account", width: 120, render: (v) => (v === "*" ? "All" : v) },
            { title: "When", render: (_, c) => cronWhen(c) },
            { title: "Destination", dataIndex: "destName" },
            { title: "Keep", dataIndex: "retain", width: 70 },
            { title: "Last run", dataIndex: "lastStatus", ellipsis: true, render: (v) => v || "—" },
            {
              title: "",
              width: 90,
              align: "right",
              render: (_, c) => (
                <Popconfirm title="Delete this schedule?" onConfirm={() => api.delete(`/api/backup/crons/${c.id}`).then(load)}>
                  <Button size="small" danger>
                    Delete
                  </Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
      <Card title="Jobs">
        <Table
          rowKey="id"
          dataSource={jobs}
          pagination={false}
          locale={{ emptyText: "No backup jobs yet." }}
          columns={[
            { title: "Account", dataIndex: "account", width: 110 },
            { title: "Destination", dataIndex: "destName" },
            {
              title: "Status",
              dataIndex: "status",
              width: 100,
              render: (v) => <Tag color={v === "ok" ? "success" : v === "error" ? "error" : "processing"}>{v}</Tag>,
            },
            { title: "Size", render: (_, j) => (j.size ? formatBytes(j.size) : "—"), width: 110 },
            { title: "Remote", dataIndex: "remote", ellipsis: true },
            { title: "Message", dataIndex: "message", ellipsis: true },
            {
              title: "",
              width: 110,
              render: (_, j) =>
                j.localPath && j.status === "ok" ? (
                  <Popconfirm
                    title={`Restore ${j.account}?`}
                    description="Creates the Linux user if it is missing, then restores home, databases, and sites."
                    onConfirm={() => void restore(j.localPath || "", j.account)}
                  >
                    <Button size="small" loading={busy}>
                      Restore
                    </Button>
                  </Popconfirm>
                ) : null,
            },
          ]}
        />
      </Card>

      <Modal title="Backup destination" open={open} onCancel={() => setOpen(false)} onOk={() => form.submit()} confirmLoading={busy} destroyOnHidden>
        <Form form={form} layout="vertical" onFinish={saveDest} initialValues={{ kind: "local", useSSL: true, port: 21 }}>
          <Form.Item name="name" label="Name" rules={[{ required: true }]}>
            <Input placeholder="Offsite S3" />
          </Form.Item>
          <Form.Item name="kind" label="Type">
            <Select
              options={[
                { value: "local", label: "Local directory" },
                { value: "ftp", label: "FTP" },
                { value: "s3", label: "S3-compatible" },
              ]}
            />
          </Form.Item>
          {kind === "local" ? (
            <Form.Item name="path" label="Directory" rules={[{ required: true }]}>
              <Input placeholder="/var/backups/offsite" />
            </Form.Item>
          ) : null}
          {kind === "ftp" ? (
            <>
              <Form.Item name="host" label="Host" rules={[{ required: true }]}>
                <Input />
              </Form.Item>
              <Form.Item name="port" label="Port">
                <InputNumber min={1} max={65535} style={{ width: "100%" }} />
              </Form.Item>
              <Form.Item name="user" label="Username">
                <Input />
              </Form.Item>
              <Form.Item name="password" label="Password">
                <Input.Password />
              </Form.Item>
              <Form.Item name="path" label="Remote path">
                <Input placeholder="/backups" />
              </Form.Item>
            </>
          ) : null}
          {kind === "s3" ? (
            <>
              <Form.Item name="endpoint" label="Endpoint" extra="Leave empty for AWS. MinIO/R2/B2: host only or https://host">
                <Input placeholder="s3.amazonaws.com" />
              </Form.Item>
              <Form.Item name="region" label="Region">
                <Input placeholder="us-east-1" />
              </Form.Item>
              <Form.Item name="bucket" label="Bucket" rules={[{ required: true }]}>
                <Input />
              </Form.Item>
              <Form.Item name="prefix" label="Prefix">
                <Input placeholder="siroc/" />
              </Form.Item>
              <Form.Item name="accessKey" label="Access key" rules={[{ required: true }]}>
                <Input />
              </Form.Item>
              <Form.Item name="secretKey" label="Secret key" rules={[{ required: true }]}>
                <Input.Password />
              </Form.Item>
              <Form.Item name="useSSL" label="HTTPS" valuePropName="checked">
                <Switch />
              </Form.Item>
            </>
          ) : null}
        </Form>
      </Modal>

      <Modal title="Backup schedule" open={cronOpen} onCancel={() => setCronOpen(false)} onOk={() => cronForm.submit()} confirmLoading={busy} destroyOnHidden>
        <Form
          form={cronForm}
          layout="vertical"
          initialValues={{ username: "*", cycle: "daily", hour: 1, minute: 30, weekday: 0, monthday: 1, retain: 3, includeDB: true }}
          onFinish={async (values) => {
            setBusy(true);
            try {
              await api.post("/api/backup/crons", values);
              setCronOpen(false);
              cronForm.resetFields();
              message.success("Cron installed");
              await load();
            } catch (err) {
              message.error(err instanceof Error ? err.message : "Failed");
            } finally {
              setBusy(false);
            }
          }}
        >
          <Form.Item name="name" label="Task name">
            <Input placeholder="Backup all accounts" />
          </Form.Item>
          <Form.Item name="username" label="Account" rules={[{ required: true }]}>
            <Select
              options={[
                { value: "*", label: "All accounts" },
                ...accounts.map((a) => ({ value: a.username, label: a.username })),
              ]}
            />
          </Form.Item>
          <Form.Item name="destId" label="Destination" rules={[{ required: true }]}>
            <Select options={dests.map((d) => ({ value: d.id, label: `${d.name} (${d.kind})` }))} />
          </Form.Item>
          <Form.Item name="cycle" label="Execute cycle">
            <Select
              options={[
                { value: "hourly", label: "Hourly" },
                { value: "daily", label: "Daily" },
                { value: "weekly", label: "Weekly" },
                { value: "monthly", label: "Monthly" },
              ]}
            />
          </Form.Item>
          <Space wrap>
            {cronCycle !== "hourly" ? (
              <Form.Item name="hour" label="Hour">
                <InputNumber min={0} max={23} />
              </Form.Item>
            ) : null}
            <Form.Item name="minute" label="Minute">
              <InputNumber min={0} max={59} />
            </Form.Item>
            {cronCycle === "weekly" ? (
              <Form.Item name="weekday" label="Weekday">
                <Select style={{ width: 140 }} options={weekdays.map((label, value) => ({ value, label }))} />
              </Form.Item>
            ) : null}
            {cronCycle === "monthly" ? (
              <Form.Item name="monthday" label="Day">
                <InputNumber min={1} max={28} />
              </Form.Item>
            ) : null}
          </Space>
          <Form.Item name="retain" label="Retain the latest" extra="Older local archives for each account are deleted.">
            <InputNumber min={1} max={30} />
          </Form.Item>
          <Form.Item name="includeDB" label="Include MySQL dumps" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Alert type="info" showIcon message="Installs /etc/cron.d/siroc-backup. The job runs siroc-panel backup-cron and writes /var/log/siroc/backup-cron.log." />
        </Form>
      </Modal>
      <Modal title="Run backup" open={runOpen} onCancel={() => setRunOpen(false)} onOk={() => runForm.submit()} confirmLoading={busy} destroyOnHidden>
        <Form form={runForm} layout="vertical" onFinish={run} initialValues={{ includeDB: true, username: accounts[0]?.username, destId: dests[0]?.id }}>
          <Form.Item name="username" label="Account" rules={[{ required: true }]}>
            <Select
              options={[
                { value: "*", label: "All accounts (one file each)" },
                ...accounts.map((a) => ({ value: a.username, label: a.username })),
              ]}
            />
          </Form.Item>
          <Form.Item name="destId" label="Destination" rules={[{ required: true }]}>
            <Select options={dests.map((d) => ({ value: d.id, label: `${d.name} (${d.kind})` }))} />
          </Form.Item>
          <Form.Item name="includeDB" label="Include MySQL dumps" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Alert type="info" showIcon message="Writes one tar.gz per user: home, databases, sites, FTP, crontab, and account settings. Restore can create the user if it is gone." />
        </Form>
      </Modal>
      <Modal
        title="Restore archive"
        open={restoreOpen}
        onCancel={() => setRestoreOpen(false)}
        onOk={() => void restore(restorePath)}
        confirmLoading={busy}
        okText="Restore"
        destroyOnHidden
      >
        <Typography.Paragraph type="secondary">
          Path to a per-user archive under /var/backups. If the user is not on this server, Siroc creates it from the backup.
        </Typography.Paragraph>
        <Input value={restorePath} onChange={(e) => setRestorePath(e.target.value)} placeholder="/var/backups/siroc/alice/alice-20261001-120000.tar.gz" />
      </Modal>
    </div>
  );
}
