import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Alert, App, Button, Card, Form, Input, InputNumber, Popconfirm, Progress, Select, Space, Table, Tabs, Typography } from "antd";
import { api } from "@/lib/api";
import { PageSkeleton } from "@/components/PageSkeleton";

type Remote = { name: string; type: string };
type Status = { installed: boolean; version?: string; remotes?: Remote[]; message?: string };
type FileProgress = { name: string; bytes: number; size: number; percent: number; speed?: number };
type Job = {
  ok?: boolean;
  running?: boolean;
  action?: string;
  source?: string;
  dest?: string;
  output?: string;
  message?: string;
  bytes?: number;
  totalBytes?: number;
  percent?: number;
  speed?: number;
  transfers?: number;
  totalTransfers?: number;
  files?: FileProgress[];
};

function formatBytes(n?: number) {
  const v = n || 0;
  if (v < 1024) return `${v} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let x = v / 1024;
  let i = 0;
  while (x >= 1024 && i < units.length - 1) {
    x /= 1024;
    i++;
  }
  return `${x >= 10 ? x.toFixed(0) : x.toFixed(1)} ${units[i]}`;
}

const types = [
  { value: "s3", label: "S3" },
  { value: "b2", label: "Backblaze B2" },
  { value: "sftp", label: "SFTP" },
  { value: "ftp", label: "FTP" },
  { value: "webdav", label: "WebDAV" },
  { value: "local", label: "Local disk" },
];

const actions = [
  { value: "copy", label: "Copy", dest: true, hint: "Copy files and leave the source in place." },
  { value: "sync", label: "Sync", dest: true, danger: true, hint: "Make the destination match the source. Extra files on the destination are deleted." },
  { value: "move", label: "Move", dest: true, danger: true, hint: "Copy files, then delete them from the source." },
  { value: "copyto", label: "Copy one file", dest: true, hint: "Copy a single file to a single destination path." },
  { value: "check", label: "Check", dest: true, hint: "Compare the source and destination without changing either." },
  { value: "ls", label: "List files", dest: false, hint: "List files under the path." },
  { value: "lsd", label: "List directories", dest: false, hint: "List directories under the path." },
  { value: "size", label: "Size", dest: false, hint: "Show the total size of the path." },
  { value: "mkdir", label: "Make directory", dest: false, hint: "Create a directory on a local disk or a remote." },
  { value: "delete", label: "Delete files", dest: false, danger: true, hint: "Delete the files in the path. Directories stay." },
  { value: "purge", label: "Purge", dest: false, danger: true, hint: "Delete the path and everything inside it." },
];

export function Rclone() {
  const { message } = App.useApp();
  const [st, setSt] = useState<Status | null>(null);
  const [busy, setBusy] = useState(false);
  const [job, setJob] = useState<Job | null>(null);
  const running = !!job?.running;
  const files = job?.files || [];
  const wasRunning = useRef(false);
  const pollGen = useRef(0);
  const [remoteForm] = Form.useForm();
  const [jobForm] = Form.useForm();
  const kind = Form.useWatch("type", remoteForm);
  const action = Form.useWatch("action", jobForm) || "copy";
  const spec = actions.find((a) => a.value === action) || actions[0];

  async function load() {
    const data = await api.get<Status>("/api/rclone");
    setSt(data);
  }

  useEffect(() => {
    load()
      .catch((e) => message.error(e instanceof Error ? e.message : "Failed"))
      .finally(() => setSt((cur) => cur || { installed: false }));
  }, []);

  useEffect(() => {
    let stop = false;
    async function poll() {
      const gen = pollGen.current;
      try {
        const data = await api.get<Job>("/api/rclone/run");
        if (stop || gen !== pollGen.current) return;
        if (wasRunning.current && !data.running) {
          if (data.ok) message.success("Finished");
          else message.error(data.message || "Failed");
        }
        wasRunning.current = !!data.running;
        setJob(data);
      } catch {
        // The next poll retries.
      }
    }
    poll();
    const t = setInterval(poll, 1000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);

  async function saveRemote(values: Record<string, string>) {
    setBusy(true);
    try {
      const params: Record<string, string> = {};
      for (const [key, value] of Object.entries(values)) {
        if (key === "name" || key === "type" || value == null || value === "") continue;
        params[key] = String(value);
      }
      const next = await api.post<Status>("/api/rclone/remotes", { name: values.name, type: values.type, params });
      setSt(next);
      remoteForm.resetFields();
      message.success("Remote saved");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function removeRemote(name: string) {
    setBusy(true);
    try {
      const next = await api.delete<Status>(`/api/rclone/remotes/${encodeURIComponent(name)}`);
      setSt(next);
      message.success("Remote removed");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function run(values: { action: string; source: string; dest?: string }) {
    const gen = ++pollGen.current;
    setBusy(true);
    try {
      const out = await api.post<Job>("/api/rclone/run", {
        action: values.action,
        source: values.source,
        dest: spec.dest ? values.dest : "",
      });
      if (gen !== pollGen.current) return;
      wasRunning.current = !!out.running;
      setJob(out);
      if (!out.running && out.ok) message.success("Finished");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  if (!st) return <PageSkeleton bare rows={5} />;

  return (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <div>
        <Typography.Title level={3} style={{ marginBottom: 4 }}>
          rclone
        </Typography.Title>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          {st.version || "Manage remotes and run copy, sync, move, list, and delete."}{" "}
          <Link to="/software">Install from Software</Link>
        </Typography.Paragraph>
      </div>
      {!st.installed ? <Alert type="warning" showIcon message={st.message || "rclone is not installed."} /> : null}
      <Tabs
        items={[
          {
            key: "remotes",
            label: "Remotes",
            children: (
              <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Table
                  size="small"
                  rowKey="name"
                  pagination={false}
                  dataSource={st.remotes || []}
                  locale={{ emptyText: "No remotes yet." }}
                  columns={[
                    { title: "Name", dataIndex: "name" },
                    { title: "Type", dataIndex: "type", width: 140 },
                    {
                      title: "",
                      width: 100,
                      render: (_, row) => (
                        <Popconfirm title={`Remove remote ${row.name}?`} onConfirm={() => void removeRemote(row.name)}>
                          <Button size="small" danger loading={busy}>
                            Remove
                          </Button>
                        </Popconfirm>
                      ),
                    },
                  ]}
                />
                <Card size="small" title="Add remote">
                  <Form form={remoteForm} layout="vertical" onFinish={saveRemote} initialValues={{ type: "s3" }} disabled={!st.installed}>
                    <Form.Item name="name" label="Name" rules={[{ required: true }]} extra="Letters, numbers, _ and -. Used as remote:path.">
                      <Input placeholder="offsite" />
                    </Form.Item>
                    <Form.Item name="type" label="Type">
                      <Select options={types} />
                    </Form.Item>
                    {kind === "s3" ? (
                      <>
                        <Form.Item name="provider" label="Provider">
                          <Select
                            options={[
                              { value: "AWS", label: "AWS" },
                              { value: "Minio", label: "MinIO" },
                              { value: "Cloudflare", label: "Cloudflare R2" },
                              { value: "Other", label: "Other" },
                            ]}
                          />
                        </Form.Item>
                        <Form.Item name="access_key_id" label="Access key" rules={[{ required: true }]}>
                          <Input />
                        </Form.Item>
                        <Form.Item name="secret_access_key" label="Secret key" rules={[{ required: true }]}>
                          <Input.Password />
                        </Form.Item>
                        <Form.Item name="region" label="Region">
                          <Input placeholder="us-east-1" />
                        </Form.Item>
                        <Form.Item name="endpoint" label="Endpoint">
                          <Input placeholder="s3.amazonaws.com" />
                        </Form.Item>
                      </>
                    ) : null}
                    {kind === "b2" ? (
                      <>
                        <Form.Item name="account" label="Account ID" rules={[{ required: true }]}>
                          <Input />
                        </Form.Item>
                        <Form.Item name="key" label="Application key" rules={[{ required: true }]}>
                          <Input.Password />
                        </Form.Item>
                      </>
                    ) : null}
                    {kind === "sftp" || kind === "ftp" ? (
                      <>
                        <Form.Item name="host" label="Host" rules={[{ required: true }]}>
                          <Input />
                        </Form.Item>
                        <Form.Item name="port" label="Port">
                          <InputNumber min={1} max={65535} style={{ width: "100%" }} />
                        </Form.Item>
                        <Form.Item name="user" label="Username" rules={[{ required: true }]}>
                          <Input />
                        </Form.Item>
                        <Form.Item name="pass" label="Password">
                          <Input.Password />
                        </Form.Item>
                      </>
                    ) : null}
                    {kind === "webdav" ? (
                      <>
                        <Form.Item name="url" label="URL" rules={[{ required: true }]}>
                          <Input placeholder="https://cloud.example/remote.php/dav/files/user/" />
                        </Form.Item>
                        <Form.Item name="vendor" label="Vendor" rules={[{ required: true }]}>
                          <Select
                            options={[
                              { value: "nextcloud", label: "Nextcloud" },
                              { value: "owncloud", label: "ownCloud" },
                              { value: "sharepoint", label: "SharePoint" },
                              { value: "other", label: "Other" },
                            ]}
                          />
                        </Form.Item>
                        <Form.Item name="user" label="Username">
                          <Input />
                        </Form.Item>
                        <Form.Item name="pass" label="Password">
                          <Input.Password />
                        </Form.Item>
                      </>
                    ) : null}
                    <Button type="primary" htmlType="submit" loading={busy} disabled={!st.installed}>
                      Save remote
                    </Button>
                  </Form>
                </Card>
              </Space>
            ),
          },
          {
            key: "run",
            label: "Run",
            children: (
              <Card size="small" title="Run a command">
                <Form form={jobForm} layout="vertical" onFinish={run} initialValues={{ action: "copy" }} disabled={!st.installed}>
                  <Form.Item name="action" label="Action">
                    <Select options={actions.map((a) => ({ value: a.value, label: a.label }))} />
                  </Form.Item>
                  <Typography.Paragraph type="secondary">{spec.hint}</Typography.Paragraph>
                  <Form.Item name="source" label={spec.dest ? "Source" : "Path"} rules={[{ required: true }]} extra="Absolute path, or remote:path such as offsite:bucket/files.">
                    <Input placeholder="/var/backups/siroc" />
                  </Form.Item>
                  {spec.dest ? (
                    <Form.Item name="dest" label="Destination" rules={[{ required: true }]}>
                      <Input placeholder="offsite:bucket/siroc" />
                    </Form.Item>
                  ) : null}
                  {spec.danger ? (
                    <Popconfirm title={`Run ${spec.label}?`} description={spec.hint} onConfirm={() => jobForm.submit()}>
                      <Button danger type="primary" loading={busy || running} disabled={!st.installed || running}>
                        {running ? "Running" : `Run ${spec.label}`}
                      </Button>
                    </Popconfirm>
                  ) : (
                    <Button type="primary" htmlType="submit" loading={busy || running} disabled={!st.installed || running}>
                      {running ? "Running" : `Run ${spec.label}`}
                    </Button>
                  )}
                </Form>
                {job && (running || job.percent || (job.files && job.files.length > 0)) ? (
                  <div style={{ marginTop: 16 }}>
                    <Typography.Text>
                      {job.action ? `${job.action} ` : ""}
                      {job.source}
                      {job.dest ? ` → ${job.dest}` : ""}
                    </Typography.Text>
                    <Progress percent={job.percent || 0} status={running ? "active" : job.ok ? "success" : "exception"} />
                    <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                      {[
                        job.totalTransfers ? `${job.transfers || 0} / ${job.totalTransfers} files` : "",
                        job.totalBytes ? `${formatBytes(job.bytes)} / ${formatBytes(job.totalBytes)}` : job.bytes ? formatBytes(job.bytes) : "",
                        job.speed ? `${formatBytes(job.speed)}/s` : "",
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                    </Typography.Paragraph>
                    {files.length ? (
                      <div style={{ marginTop: 12 }}>
                        <Typography.Text type="secondary">{files.length === 1 ? "Current file" : "Current files"}</Typography.Text>
                        {files.map((file) => (
                          <div key={file.name} style={{ marginTop: 8 }}>
                            <Typography.Text code>{file.name}</Typography.Text>
                            <Progress percent={file.percent || 0} size="small" status={running ? "active" : "normal"} />
                            <Typography.Text type="secondary">
                              {file.size > 0 ? `${formatBytes(file.bytes)} / ${formatBytes(file.size)}` : formatBytes(file.bytes)}
                              {file.speed ? ` · ${formatBytes(file.speed)}/s` : ""}
                            </Typography.Text>
                          </div>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ) : null}
                {!running && job?.message && !job.ok ? <Alert style={{ marginTop: 16 }} type="error" showIcon message={job.message} /> : null}
                {job?.output ? <pre className="cmd-out">{job.output}</pre> : null}
              </Card>
            ),
          },
        ]}
      />
    </Space>
  );
}
