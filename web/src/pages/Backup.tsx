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

export function Backup() {
  const { message } = App.useApp();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [dests, setDests] = useState<Dest[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [runOpen, setRunOpen] = useState(false);
  const [form] = Form.useForm();
  const [runForm] = Form.useForm();
  const kind = Form.useWatch("kind", form);

  async function load() {
    const [a, d, j] = await Promise.all([
      api.get<Account[]>("/api/accounts"),
      api.get<Dest[]>("/api/backup/dests").catch(() => [] as Dest[]),
      api.get<Job[]>("/api/backup/jobs"),
    ]);
    setAccounts(a);
    setDests(d);
    setJobs(j);
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

  async function run(values: { username: string; destId: number; includeDB: boolean }) {
    setBusy(true);
    try {
      await api.post("/api/backup/run", values);
      setRunOpen(false);
      message.success("Backup finished");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function restore(j: Job) {
    setBusy(true);
    try {
      await api.post("/api/backup/restore", { username: j.account, path: j.localPath });
      message.success("Restored " + j.account);
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
          <Typography.Text type="secondary">Local disk, FTP, or S3-compatible object storage</Typography.Text>
        </div>
        <Space>
          <Button onClick={() => setOpen(true)}>Add destination</Button>
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
                  <Button size="small" loading={busy} onClick={() => void restore(j)}>
                    Restore
                  </Button>
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

      <Modal title="Run backup" open={runOpen} onCancel={() => setRunOpen(false)} onOk={() => runForm.submit()} confirmLoading={busy} destroyOnHidden>
        <Form form={runForm} layout="vertical" onFinish={run} initialValues={{ includeDB: true, username: accounts[0]?.username, destId: dests[0]?.id }}>
          <Form.Item name="username" label="Account" rules={[{ required: true }]}>
            <Select options={accounts.map((a) => ({ value: a.username, label: a.username }))} />
          </Form.Item>
          <Form.Item name="destId" label="Destination" rules={[{ required: true }]}>
            <Select options={dests.map((d) => ({ value: d.id, label: `${d.name} (${d.kind})` }))} />
          </Form.Item>
          <Form.Item name="includeDB" label="Include MySQL dumps" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Alert type="info" showIcon message="Creates a tar.gz of the account home, then uploads to the destination." />
        </Form>
      </Modal>
    </div>
  );
}
