import { useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Card, Checkbox, Form, Input, InputNumber, Popconfirm, Progress, Select, Space, Switch, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";

type Site = { domain: string; owner: string };
type Item = {
  domain: string;
  owner: string;
  status: string;
  step?: string;
  percent: number;
  message?: string;
};
type Job = { status: string; message?: string; phpVersion?: string; items: Item[] };

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
  const [starting, setStarting] = useState(false);
  const [job, setJob] = useState<Job | null>(null);

  const running = job?.status === "running";

  useEffect(() => {
    api.get<string[]>("/api/software/php-versions").then((list) => {
      const versions = list || [];
      setPhpList(versions);
      setPhp(newestPHP(versions));
    }).catch(() => undefined);
  }, []);

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

  async function listSites(values: { host: string; port: number; username: string; password: string; insecure: boolean }) {
    setListing(true);
    try {
      const data = await api.post<{ sites: Site[] }>("/api/migrate/directadmin/list", values);
      const next = data.sites || [];
      setSites(next);
      setPicked(next.map((s) => s.domain));
      message.success(next.length === 1 ? "1 website" : `${next.length} websites`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setListing(false);
    }
  }

  async function start() {
    const values = form.getFieldsValue();
    const selected = sites.filter((s) => picked.includes(s.domain));
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
      const data = await api.post<Job>("/api/migrate/directadmin", { ...values, sites: selected, phpVersion: php });
      setJob(data);
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
          Connect to DirectAdmin, choose the websites, and leave this running. Files, databases, and the account are imported. Subdomains of a selected website come with it. Mail, DNS, and certificates are not. The DirectAdmin username becomes the account on this server.
        </Typography.Paragraph>
        <Form
          form={form}
          layout="vertical"
          initialValues={{ port: 2222, insecure: true }}
          onFinish={listSites}
        >
          <Form.Item name="host" label="Host" rules={[{ required: true, message: "Host is required" }]}>
            <Input placeholder="203.0.113.10 or https://panel.example:2222" autoComplete="off" />
          </Form.Item>
          <Form.Item name="port" label="Port">
            <InputNumber min={1} max={65535} style={{ width: 160 }} />
          </Form.Item>
          <Form.Item name="username" label="Username" rules={[{ required: true, message: "Username is required" }]}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item name="password" label="Password or login key" rules={[{ required: true, message: "Password is required" }]}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="insecure" label="Allow a self-signed certificate" valuePropName="checked">
            <Switch />
          </Form.Item>
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
            <Popconfirm
              title={`Migrate ${picked.length} website${picked.length === 1 ? "" : "s"}?`}
              description="This runs in the background."
              okText="Migrate"
              disabled={picked.length === 0 || running}
              onConfirm={start}
            >
              <Button type="primary" loading={starting} disabled={picked.length === 0 || running}>
                Migrate selected
              </Button>
            </Popconfirm>
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
