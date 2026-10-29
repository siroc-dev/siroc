import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Alert, App, Button, Card, Form, Input, InputNumber, Modal, Popconfirm, Progress, Select, Space, Switch, Table, Tabs, Tag, Typography, Upload } from "antd";
import { api, uploadWasCancelled } from "@/lib/api";
import { PageSkeleton } from "@/components/PageSkeleton";
import { formatBytes, formatRate } from "@/lib/usage";
import { formatPanelTime } from "@/lib/panelTime";
import { Rclone } from "@/pages/Rclone";
import { PathPicker } from "@/components/PathPicker";

type Account = { username: string };
type Dest = {
  id: number;
  name: string;
  kind: string;
  host?: string;
  port?: number;
  user?: string;
  path?: string;
  bucket?: string;
  region?: string;
  endpoint?: string;
  prefix?: string;
  useSSL?: boolean;
  hasSecret?: boolean;
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
  destId: number;
  destName?: string;
  includeDB: boolean;
  cycle: string;
  minute: number;
  hour: number;
  weekday: number;
  monthday: number;
  retain: number;
  enabled: boolean;
  lastRun?: string;
  lastStatus?: string;
};

const weekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

function destFormValues(d: Dest | null) {
  if (!d) {
    return { name: "", kind: "local", path: "", host: "", port: 21, user: "", password: "", endpoint: "", region: "", bucket: "", prefix: "", accessKey: "", secretKey: "", useSSL: true };
  }
  return {
    name: d.name,
    kind: d.kind,
    path: d.path ?? "",
    host: d.host ?? "",
    port: d.port || (d.kind === "ftp" ? 21 : undefined),
    user: d.user ?? "",
    password: "",
    endpoint: d.endpoint ?? "",
    region: d.region ?? "",
    bucket: d.bucket ?? "",
    prefix: d.prefix ?? "",
    accessKey: d.kind === "s3" ? d.user ?? "" : "",
    secretKey: "",
    useSSL: !!d.useSSL,
  };
}

function cronFormValues(c: Cron | null) {
  if (!c) {
    return { name: "", username: "*", cycle: "daily", hour: 1, minute: 30, weekday: 0, monthday: 1, retain: 3, includeDB: true };
  }
  return {
    name: c.name,
    username: c.account,
    destId: c.destId,
    cycle: c.cycle,
    hour: c.hour,
    minute: c.minute,
    weekday: c.weekday,
    monthday: c.monthday || 1,
    retain: c.retain,
    includeDB: c.includeDB,
  };
}

function jobWhen(raw?: string) {
  return formatPanelTime(raw) || "—";
}

function cronWhen(c: Cron) {
  const hm = `${String(c.hour).padStart(2, "0")}:${String(c.minute).padStart(2, "0")}`;
  if (c.cycle === "hourly") return `Hourly at :${String(c.minute).padStart(2, "0")}`;
  if (c.cycle === "weekly") return `Weekly on ${weekdays[c.weekday] || "Sunday"} at ${hm}`;
  if (c.cycle === "monthly") return `Monthly on day ${c.monthday} at ${hm}`;
  return `Daily at ${hm}`;
}

export function Backup({ admin, user }: { admin?: boolean; user?: string }) {
  const { message } = App.useApp();
  const [params, setParams] = useSearchParams();
  const tab = admin && params.get("tab") === "rclone" ? "rclone" : "archives";
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [dests, setDests] = useState<Dest[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [crons, setCrons] = useState<Cron[]>([]);
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [editingDest, setEditingDest] = useState<Dest | null>(null);
  const [runOpen, setRunOpen] = useState(false);
  const [cronOpen, setCronOpen] = useState(false);
  const [editingCron, setEditingCron] = useState<Cron | null>(null);
  const [form] = Form.useForm();
  const [runForm] = Form.useForm();
  const [cronForm] = Form.useForm();
  const kind = Form.useWatch("kind", form);
  const cronCycle = Form.useWatch("cycle", cronForm);
  const [ready, setReady] = useState(false);

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
    load()
      .catch((e) => message.error(e.message))
      .finally(() => setReady(true));
  }, []);

  async function saveDest(values: Record<string, unknown>) {
    setBusy(true);
    try {
      if (editingDest) await api.put(`/api/backup/dests/${editingDest.id}`, values);
      else await api.post("/api/backup/dests", values);
      form.resetFields();
      setEditingDest(null);
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
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [uploadPct, setUploadPct] = useState<number | null>(null);
  const [uploadSpeedBps, setUploadSpeedBps] = useState(0);
  const uploadAbort = useRef<AbortController | null>(null);

  async function setCronEnabled(c: Cron, enabled: boolean) {
    try {
      await api.put(`/api/backup/crons/${c.id}`, { enabled });
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

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
      message.success(out.message || (out.createdUser ? `Created user ${name} and restored` : `Restored ${name}`));
      setRestoreOpen(false);
      setRestorePath("");
      setRestoreFile(null);
      setUploadPct(null);
      setUploadSpeedBps(0);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  function cancelRestoreUpload() {
    uploadAbort.current?.abort();
  }

  async function restoreUpload(file: File) {
    const ctrl = new AbortController();
    uploadAbort.current = ctrl;
    setBusy(true);
    setUploadPct(0);
    setUploadSpeedBps(0);
    try {
      const out = await api.uploadProgress<{ createdUser?: boolean; message?: string; manifest?: { username?: string } }>(
        "/api/backup/restore/upload",
        file,
        (pct, speed) => {
          setUploadPct(pct);
          setUploadSpeedBps(speed);
        },
        "file",
        ctrl.signal,
      );
      const name = out.manifest?.username || "account";
      message.success(out.message || (out.createdUser ? `Created user ${name} and restored` : `Restored ${name}`));
      setRestoreOpen(false);
      setRestorePath("");
      setRestoreFile(null);
      await load();
    } catch (err) {
      if (uploadWasCancelled(err)) message.info("Upload cancelled");
      else message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      uploadAbort.current = null;
      setBusy(false);
      setUploadPct(null);
      setUploadSpeedBps(0);
    }
  }

  if (!ready) return <PageSkeleton />;

  return (
    <div className="cp-page">
      <Typography.Title level={3} style={{ margin: 0 }}>
        Backup
      </Typography.Title>
      <Tabs
        activeKey={tab}
        destroyOnHidden
        onChange={(key) => setParams(key === "rclone" ? { tab: "rclone" } : {}, { replace: true })}
        items={[
          {
            key: "archives",
            label: "Archives",
            children: (
              <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 16 }}>
        <Typography.Text type="secondary">Siroc archives and DirectAdmin user backups. Restore creates a missing account, its websites, and its databases.</Typography.Text>
        <Space>
          <Button onClick={() => { setEditingDest(null); setOpen(true); }}>Add destination</Button>
          {admin ? <Button onClick={() => setRestoreOpen(true)}>Restore archive</Button> : null}
          {admin ? (
            <Button onClick={() => { setEditingCron(null); setCronOpen(true); }} disabled={!dests.length}>
              Add schedule
            </Button>
          ) : null}
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
              width: 150,
              align: "right",
              render: (_, d) => (
                <Space>
                  <Button size="small" onClick={() => { setEditingDest(d); setOpen(true); }}>
                    Edit
                  </Button>
                  <Popconfirm title="Delete destination?" onConfirm={() => api.delete(`/api/backup/dests/${d.id}`).then(load)}>
                    <Button size="small" danger>
                      Delete
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Card>
      {admin ? (
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
            {
              title: "Auto",
              width: 80,
              render: (_, c) => <Switch checked={c.enabled} onChange={(on) => void setCronEnabled(c, on)} />,
            },
            { title: "Last run", dataIndex: "lastStatus", ellipsis: true, render: (v) => v || "—" },
            {
              title: "",
              width: 150,
              align: "right",
              render: (_, c) => (
                <Space>
                  <Button size="small" onClick={() => { setEditingCron(c); setCronOpen(true); }}>
                    Edit
                  </Button>
                  <Popconfirm title="Delete this schedule?" onConfirm={() => api.delete(`/api/backup/crons/${c.id}`).then(load)}>
                    <Button size="small" danger>
                      Delete
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Card>
      ) : null}
      <Card title="Jobs">
        <Table
          rowKey="id"
          dataSource={jobs}
          pagination={false}
          locale={{ emptyText: "No backup jobs yet." }}
          columns={[
            { title: "Account", dataIndex: "account", width: 140 },
            { title: "Created", dataIndex: "createdAt", width: 170, render: (v: string) => jobWhen(v) },
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
                admin && j.localPath && j.status === "ok" ? (
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
              </Space>
            ),
          },
          ...(admin
            ? [
                {
                  key: "rclone",
                  label: "rclone",
                  children: <Rclone embedded />,
                },
              ]
            : []),
        ]}
      />

      <Modal
        title={editingDest ? "Edit destination" : "Backup destination"}
        open={open}
        onCancel={() => { setOpen(false); setEditingDest(null); }}
        onOk={() => form.submit()}
        confirmLoading={busy}
        destroyOnHidden
      >
        <Form
          key={editingDest?.id ?? "new"}
          form={form}
          layout="vertical"
          onFinish={saveDest}
          initialValues={destFormValues(editingDest)}
          clearOnDestroy
        >
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
            <Form.Item
              name="path"
              label="Directory"
              extra={admin ? undefined : "Must stay inside your home directory."}
              rules={[{ required: true }]}
            >
              <PathPicker
                user={admin ? "root" : user || accounts[0]?.username || ""}
                dirsOnly
                placeholder={admin ? "/var/backups/offsite" : `/home/${user || accounts[0]?.username || ""}/backups`}
              />
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
              <Form.Item name="password" label="Password" extra={editingDest ? "Leave blank to keep the current password." : undefined}>
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
              <Form.Item name="accessKey" label="Access key" rules={editingDest ? [] : [{ required: true }]}>
                <Input />
              </Form.Item>
              <Form.Item name="secretKey" label="Secret key" rules={editingDest ? [] : [{ required: true }]} extra={editingDest ? "Leave blank to keep the current secret." : undefined}>
                <Input.Password />
              </Form.Item>
              <Form.Item name="useSSL" label="HTTPS" valuePropName="checked">
                <Switch />
              </Form.Item>
            </>
          ) : null}
        </Form>
      </Modal>

      <Modal
        title={editingCron ? "Edit schedule" : "Backup schedule"}
        open={cronOpen}
        onCancel={() => { setCronOpen(false); setEditingCron(null); }}
        onOk={() => cronForm.submit()}
        confirmLoading={busy}
        destroyOnHidden
      >
        <Form
          key={editingCron?.id ?? "new"}
          form={cronForm}
          layout="vertical"
          initialValues={cronFormValues(editingCron)}
          clearOnDestroy
          onFinish={async (values) => {
            setBusy(true);
            try {
              if (editingCron) await api.put(`/api/backup/crons/${editingCron.id}`, values);
              else await api.post("/api/backup/crons", values);
              setCronOpen(false);
              setEditingCron(null);
              cronForm.resetFields();
              message.success(editingCron ? "Schedule saved" : "Cron installed");
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
                ...(admin ? [{ value: "*", label: "All accounts" }] : []),
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
                ...(admin ? [{ value: "*", label: "All accounts (one file each)" }] : []),
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
        onCancel={() => {
          if (busy) return;
          setRestoreOpen(false);
          setRestoreFile(null);
          setUploadPct(null);
          setUploadSpeedBps(0);
        }}
        onOk={() => {
          if (restoreFile) void restoreUpload(restoreFile);
          else void restore(restorePath);
        }}
        confirmLoading={busy}
        okText="Restore"
        okButtonProps={{ disabled: !restoreFile && !restorePath.trim() }}
        destroyOnHidden
      >
        <Typography.Paragraph type="secondary">
          Upload a Siroc or DirectAdmin archive, or give a path already under /var/backups. A DirectAdmin admin backup restores each user in it. Websites and databases are imported. Mail, DNS, and certificates are not. Maximum upload size is 10 GB.
        </Typography.Paragraph>
        <Upload
          accept=".tar,.gz,.tgz,.zst,.tzst"
          maxCount={1}
          beforeUpload={(file) => {
            setRestoreFile(file);
            return false;
          }}
          onRemove={() => {
            setRestoreFile(null);
            return true;
          }}
          fileList={restoreFile ? [{ uid: "-1", name: restoreFile.name }] : []}
        >
          <Button disabled={busy}>Choose archive</Button>
        </Upload>
        {uploadPct != null ? (
          <>
            <Progress percent={uploadPct} style={{ marginTop: 12 }} />
            {uploadSpeedBps > 0 ? <Typography.Text type="secondary">{formatRate(uploadSpeedBps)}/s</Typography.Text> : null}
            {uploadPct < 100 ? (
              <div style={{ marginTop: 8 }}>
                <Button danger onClick={cancelRestoreUpload}>
                  Cancel upload
                </Button>
              </div>
            ) : null}
          </>
        ) : null}
        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 8 }}>
          Or a path on the server
        </Typography.Paragraph>
        <PathPicker value={restorePath} onChange={setRestorePath} user="root" placeholder="/var/backups/siroc/user.alice.tar.zst" disabled={busy} />
      </Modal>
    </div>
  );
}
