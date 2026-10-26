import { useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Card, Checkbox, Form, Input, InputNumber, Modal, Progress, Select, Space, Switch, Table, Tag, Typography, Upload } from "antd";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";

type Site = { domain: string; owner: string };
type SizeRow = { domain: string; owner: string; webBytes: number; dbBytes: number };
type SizeList = { sites?: SizeRow[]; webBytes?: number; dbBytes?: number };
type Item = {
  domain: string;
  owner: string;
  status: string;
  step?: string;
  percent: number;
  message?: string;
};
type Job = { status: string; message?: string; phpVersion?: string; items: Item[] };
type SavedMachine = {
  mode?: string;
  host?: string;
  port?: number;
  username?: string;
  insecure?: boolean;
  keyName?: string;
  hasPassword?: boolean;
  hasKey?: boolean;
  hasPassphrase?: boolean;
};

const statusText: Record<string, string> = {
  queued: "Queued",
  running: "Running",
  records: "Saving",
  done: "Done",
  failed: "Failed",
};

function newestPHP(list: string[]) {
  for (const v of ["8.4", "8.3", "8.2", "8.1", "7.4"]) {
    if (list.includes(v)) return v;
  }
  return list[0] || "";
}

export function Migrate() {
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const [phpList, setPhpList] = useState<string[]>([]);
  const [php, setPhp] = useState("");
  const [sites, setSites] = useState<Site[]>([]);
  const [picked, setPicked] = useState<string[]>([]);
  const [listing, setListing] = useState(false);
  const [measuring, setMeasuring] = useState(false);
  const [starting, setStarting] = useState(false);
  const [sizes, setSizes] = useState<SizeList | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const mode = Form.useWatch("mode", form) || "ssh";
  const [keyText, setKeyText] = useState("");
  const [keyName, setKeyName] = useState("");
  const [keyRemoved, setKeyRemoved] = useState(false);
  const [saved, setSaved] = useState<SavedMachine | null>(null);

  const running = job?.status === "running";

  useEffect(() => {
    api.get<string[]>("/api/software/php-versions").then((list) => {
      const versions = list || [];
      setPhpList(versions);
      setPhp(newestPHP(versions));
    }).catch(() => undefined);
  }, []);

  useEffect(() => {
    api.get<SavedMachine>("/api/migrate/directadmin/machine").then((data) => {
      if (!data) return;
      setSaved(data);
      form.setFieldsValue({
        mode: data.mode || "ssh",
        host: data.host || "",
        port: data.port || (data.mode === "api" ? 2222 : 22),
        username: data.username || "",
        insecure: data.insecure !== false,
      });
      if (data.keyName) setKeyName(data.keyName);
    }).catch(() => undefined);
  }, [form]);

  useEffect(() => {
    let stop = false;
    const poll = () => {
      api.get<Job>("/api/migrate/directadmin").then((data) => {
        if (!stop) setJob(data);
      }).catch(() => undefined);
    };
    poll();
    const t = setInterval(poll, 2000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);

  function connection(values: { host: string; port: number; username: string; password?: string; insecure?: boolean; mode?: string; keyPassphrase?: string }) {
    const ssh = (values.mode || "ssh") !== "api";
    const useSavedKey = ssh && !keyText && !keyRemoved && !!saved?.hasKey;
    return {
      ...values,
      keyName: ssh ? keyName : "",
      privateKey: ssh && keyText ? keyText : "",
      keyPassphrase: ssh ? values.keyPassphrase || "" : "",
      useSavedKey,
      useSavedPassword: !values.password && !!saved?.hasPassword,
      useSavedPassphrase: ssh && !values.keyPassphrase && !!saved?.hasPassphrase && !keyText && useSavedKey,
    };
  }

  async function refreshSaved() {
    const data = await api.get<SavedMachine>("/api/migrate/directadmin/machine");
    setSaved(data);
    setKeyName(data?.keyName || "");
    setKeyText("");
    setKeyRemoved(false);
    form.setFieldValue("password", "");
    form.setFieldValue("keyPassphrase", "");
  }

  async function listSites(values: { host: string; port: number; username: string; password?: string; insecure?: boolean; mode?: string; keyPassphrase?: string }) {
    const ssh = (values.mode || "ssh") !== "api";
    const body = connection(values);
    if (ssh && !values.password && !keyText && !body.useSavedKey && !body.useSavedPassword) {
      message.error("Enter a password or upload a PPK file");
      return;
    }
    if (!ssh && !values.password && !body.useSavedPassword) {
      message.error("Password is required");
      return;
    }
    setListing(true);
    try {
      const data = await api.post<{ sites: Site[] }>("/api/migrate/directadmin/list", body);
      const next = data.sites || [];
      setSites(next);
      setPicked(next.map((s) => s.domain));
      await refreshSaved();
      message.success(next.length === 1 ? "1 website" : `${next.length} websites`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setListing(false);
    }
  }

  function selectedSites() {
    return sites.filter((s) => picked.includes(s.domain));
  }

  async function measure() {
    const values = form.getFieldsValue();
    const selected = selectedSites();
    if (selected.length === 0) {
      message.error("Select at least one website");
      return;
    }
    if (!php) {
      message.error("Choose a PHP version");
      return;
    }
    if ((values.mode || "ssh") === "api") {
      message.error("Connect with SSH to measure the website and database size");
      return;
    }
    setMeasuring(true);
    try {
      const data = await api.post<SizeList>("/api/migrate/directadmin/size", {
        ...connection(values),
        sites: selected,
      });
      setSizes(data);
      await refreshSaved();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setMeasuring(false);
    }
  }

  async function start() {
    const values = form.getFieldsValue();
    const selected = selectedSites();
    if (selected.length === 0) {
      message.error("Select at least one website");
      return;
    }
    if (!php) {
      message.error("Choose a PHP version");
      return;
    }
    setStarting(true);
    try {
      const data = await api.post<Job>("/api/migrate/directadmin", {
        ...connection(values),
        sites: selected,
        phpVersion: php,
      });
      setJob(data);
      setSizes(null);
      await refreshSaved();
      message.success("Migration started");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setStarting(false);
    }
  }

  const rows = useMemo(() => sites.map((s) => ({ ...s, key: s.domain })), [sites]);
  const all = sites.length > 0 && picked.length === sites.length;

  return (
    <Space direction="vertical" size="large" style={{ width: "100%" }}>
      <Card>
        <Typography.Paragraph type="secondary">
          Copy websites from DirectAdmin over SSH as root, or as a user that can sudo. Log in with a password or a PuTTY PPK file. The host, username, and private key are saved on this server. The DirectAdmin API login still works. Files, databases, and the account are imported. Subdomains of a selected website come with it. Mail, DNS, and certificates are not. The DirectAdmin username becomes the account on this server.
        </Typography.Paragraph>
        <Form
          form={form}
          layout="vertical"
          initialValues={{ mode: "ssh", port: 22, insecure: true }}
          onFinish={listSites}
        >
          <Form.Item name="mode" label="Connection">
            <Select
              options={[
                { value: "ssh", label: "SSH (root or sudo)" },
                { value: "api", label: "DirectAdmin API" },
              ]}
              onChange={(value) => {
                form.setFieldValue("port", value === "ssh" ? 22 : 2222);
                setSites([]);
                setPicked([]);
              }}
            />
          </Form.Item>
          <Form.Item name="host" label="Host" rules={[{ required: true, message: "Host is required" }]}>
            <Input placeholder={mode === "ssh" ? "203.0.113.10" : "203.0.113.10 or https://panel.example:2222"} autoComplete="off" />
          </Form.Item>
          <Form.Item name="port" label="Port">
            <InputNumber min={1} max={65535} style={{ width: 160 }} />
          </Form.Item>
          <Form.Item name="username" label="Username" rules={[{ required: true, message: "Username is required" }]}>
            <Input placeholder={mode === "ssh" ? "root" : ""} autoComplete="off" />
          </Form.Item>
          <Form.Item name="password" label={mode === "ssh" ? "Password" : "Password or login key"} rules={mode === "ssh" || saved?.hasPassword ? [] : [{ required: true, message: "Password is required" }]}>
            <Input.Password autoComplete="new-password" placeholder={saved?.hasPassword ? "Saved — leave blank to keep it" : ""} />
          </Form.Item>
          {mode === "ssh" ? (
            <>
              <Form.Item label="Private key">
                <Space wrap>
                  <Upload
                    accept=".ppk,.pem"
                    maxCount={1}
                    showUploadList={false}
                    beforeUpload={(file) => {
                      if (file.size > 256 * 1024) {
                        message.error("Key file is too large");
                        return false;
                      }
                      const reader = new FileReader();
                      reader.onload = () => {
                        const text = String(reader.result || "");
                        if (!text.includes("PuTTY-User-Key-File-") && !text.includes("PRIVATE KEY")) {
                          message.error("Choose a PuTTY PPK or OpenSSH private key");
                          setKeyText("");
                          setKeyName("");
                          return;
                        }
                        setKeyText(text);
                        setKeyName(file.name);
                        setKeyRemoved(false);
                      };
                      reader.onerror = () => message.error("Could not read the key file");
                      reader.readAsText(file);
                      return false;
                    }}
                  >
                    <Button>Upload PPK</Button>
                  </Upload>
                  {keyName ? (
                    <Typography.Text>
                      {keyName}{" "}
                      <Button
                        type="link"
                        onClick={() => {
                          setKeyText("");
                          setKeyName("");
                          setKeyRemoved(true);
                          form.setFieldValue("keyPassphrase", "");
                        }}
                      >
                        Remove
                      </Button>
                    </Typography.Text>
                  ) : null}
                </Space>
              </Form.Item>
              {keyName ? (
                <Form.Item name="keyPassphrase" label="Key passphrase">
                  <Input.Password placeholder={saved?.hasPassphrase ? "Saved — leave blank to keep it" : "Leave empty if the PPK has no passphrase"} autoComplete="new-password" />
                </Form.Item>
              ) : null}
              <Typography.Paragraph type="secondary">
                Root runs the copy directly. Any other user uses sudo with the password. Leave the password empty when the key logs in as root. The panel accepts the SSH key of the host you enter.
              </Typography.Paragraph>
            </>
          ) : (
            <Form.Item name="insecure" label="Allow a self-signed certificate" valuePropName="checked">
              <Switch />
            </Form.Item>
          )}
          <Button type="primary" htmlType="submit" loading={listing}>
            List websites
          </Button>
        </Form>
      </Card>

      {sites.length > 0 ? (
        <Card title="Websites">
          <Space style={{ marginBottom: 12 }} wrap>
            <Checkbox
              checked={all}
              indeterminate={picked.length > 0 && !all}
              onChange={(e) => setPicked(e.target.checked ? sites.map((s) => s.domain) : [])}
            >
              Select all
            </Checkbox>
            <Select
              value={php || undefined}
              placeholder="PHP version"
              style={{ width: 140 }}
              options={phpList.map((v) => ({ value: v, label: "PHP " + v }))}
              onChange={setPhp}
            />
            <Button type="primary" loading={measuring || starting} disabled={picked.length === 0 || running} onClick={() => void measure()}>
              Migrate selected
            </Button>
          </Space>
          <Table
            size="small"
            pagination={false}
            rowKey="domain"
            dataSource={rows}
            rowSelection={{
              selectedRowKeys: picked,
              onChange: (keys) => setPicked(keys.map(String)),
            }}
            columns={[
              { title: "Website", dataIndex: "domain" },
              { title: "Account", dataIndex: "owner" },
            ]}
          />
        </Card>
      ) : null}

      <Modal
        title="Confirm size before migrating"
        open={!!sizes}
        onCancel={() => setSizes(null)}
        onOk={() => void start()}
        confirmLoading={starting}
        okText="Migrate"
        destroyOnHidden
      >
        <Typography.Paragraph type="secondary">
          These file and database sizes will be copied. Mail, DNS, and certificates are not.
        </Typography.Paragraph>
        <Table
          size="small"
          pagination={false}
          rowKey="domain"
          dataSource={sizes?.sites || []}
          columns={[
            { title: "Website", dataIndex: "domain" },
            { title: "Files", width: 120, render: (_, row) => formatBytes(row.webBytes || 0) },
            { title: "Databases", width: 140, render: (_, row) => (row.dbBytes < 0 ? "Unknown" : formatBytes(row.dbBytes || 0)) },
          ]}
        />
        <Typography.Paragraph style={{ marginTop: 12, marginBottom: 0 }}>
          Total files {formatBytes(sizes?.webBytes || 0)}
          {" · "}
          Total databases {(sizes?.dbBytes ?? 0) < 0 ? "Unknown" : formatBytes(sizes?.dbBytes || 0)}
        </Typography.Paragraph>
      </Modal>

      {job && job.items && job.items.length > 0 ? (
        <Card title="Progress">
          {job.message ? (
            <Alert
              type={running ? "info" : job.items.some((item) => item.status === "failed") ? "warning" : "success"}
              showIcon
              message={job.message}
              style={{ marginBottom: 12 }}
            />
          ) : null}
          {running ? <Typography.Paragraph type="secondary">You can leave this page. Each website keeps its own progress.</Typography.Paragraph> : null}
          <Table
            size="small"
            pagination={false}
            rowKey="domain"
            dataSource={job.items}
            columns={[
              { title: "Website", dataIndex: "domain" },
              { title: "Account", dataIndex: "owner" },
              {
                title: "Progress",
                dataIndex: "percent",
                width: 180,
                render: (pct: number) => <Progress percent={pct || 0} size="small" />,
              },
              {
                title: "Status",
                dataIndex: "status",
                width: 120,
                render: (status: string) => (
                  <Tag color={status === "done" ? "success" : status === "failed" ? "error" : status === "queued" ? "default" : "processing"}>
                    {statusText[status] || status}
                  </Tag>
                ),
              },
              { title: "Detail", dataIndex: "message" },
            ]}
          />
        </Card>
      ) : null}
    </Space>
  );
}
