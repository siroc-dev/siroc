import { useEffect, useState } from "react";
import { useNavigate, useOutletContext } from "react-router-dom";
import { MoreOutlined } from "@ant-design/icons";
import { App, Alert, Button, Card, Dropdown, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes, type UserUsage } from "@/lib/usage";

type Account = {
  id: number;
  username: string;
  linuxUid: number;
  linuxGid: number;
  suspended: boolean;
  phpCli?: string;
  pythonCli?: string;
  nodeCli?: string;
  diskQuotaMB?: number;
  wafEnabled?: boolean;
};

type Pkg = { name: string; installedVersions?: string[] };

type UserRedis = {
  username: string;
  enabled: boolean;
  installed?: boolean;
  host?: string;
  port?: number;
  redisUser?: string;
  password?: string;
  prefix?: string;
  database?: number;
  file?: string;
  message?: string;
};

export function Accounts() {
  const nav = useNavigate();
  const { admin } = useOutletContext<{ admin?: boolean }>();
  const { message, modal } = App.useApp();
  const [list, setList] = useState<Account[]>([]);
  const [phpVers, setPhpVers] = useState<string[]>([]);
  const [pyVers, setPyVers] = useState<string[]>([]);
  const [nodeVers, setNodeVers] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [cliTarget, setCliTarget] = useState<Account | null>(null);
  const [quotaTarget, setQuotaTarget] = useState<Account | null>(null);
  const [redisTarget, setRedisTarget] = useState<Account | null>(null);
  const [redisInfo, setRedisInfo] = useState<UserRedis | null>(null);
  const [redisMap, setRedisMap] = useState<Record<string, boolean>>({});
  const [form] = Form.useForm();
  const [cliForm] = Form.useForm();
  const [quotaForm] = Form.useForm();
  const [passTarget, setPassTarget] = useState<Account | null>(null);
  const [passForm] = Form.useForm();
  const [newPass, setNewPass] = useState("");
  const [usage, setUsage] = useState<Record<string, UserUsage>>({});
  const [wafBusy, setWafBusy] = useState("");

  async function load() {
    const [accounts, pkgs, redisRows] = await Promise.all([
      api.get<Account[]>("/api/accounts"),
      api.get<Pkg[]>("/api/software"),
      api.get<UserRedis[]>("/api/accounts/redis").catch(() => [] as UserRedis[]),
    ]);
    setList(accounts);
    const find = (name: string) => pkgs.find((p) => p.name === name)?.installedVersions || [];
    setPhpVers(find("php"));
    setPyVers(find("python"));
    setNodeVers(find("nodejs"));
    const next: Record<string, boolean> = {};
    for (const r of redisRows || []) next[r.username] = !!r.enabled;
    setRedisMap(next);
  }

  async function setAccountWAF(a: Account, enabled: boolean) {
    setWafBusy(a.username);
    try {
      await api.put(`/api/accounts/${encodeURIComponent(a.username)}/waf`, { enabled });
      message.success(enabled ? `ModSecurity on for ${a.username}` : `ModSecurity off for ${a.username}`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setWafBusy("");
    }
  }

  useEffect(() => {
    load().catch((e) => message.error(e.message));
  }, []);

  useEffect(() => {
    let stop = false;
    async function poll() {
      try {
        const rows = await api.get<UserUsage[]>("/api/usage");
        if (stop) return;
        const next: Record<string, UserUsage> = {};
        for (const u of rows) next[u.user] = u;
        setUsage(next);
      } catch {
        if (!stop) setUsage({});
      }
    }
    poll();
    const t = setInterval(poll, 4000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, []);

  async function create(values: { username: string; password: string }) {
    setBusy(true);
    try {
      await api.post("/api/accounts", values);
      form.resetFields();
      setCreateOpen(false);
      message.success("Account created");
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function act(name: string, path: string, method: "POST" | "DELETE") {
    try {
      if (method === "DELETE") await api.delete(`/api/accounts/${name}`);
      else await api.post(`/api/accounts/${name}/${path}`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    }
  }

  function openCLI(a: Account) {
    setCliTarget(a);
    cliForm.setFieldsValue({ php: a.phpCli || "", python: a.pythonCli || "", nodejs: a.nodeCli || "" });
  }

  async function saveCLI(values: { php: string; python: string; nodejs: string }) {
    if (!cliTarget) return;
    setBusy(true);
    try {
      await api.send(`/api/accounts/${cliTarget.username}/cli`, "PUT", values);
      message.success(`CLI saved for ${cliTarget.username}`);
      setCliTarget(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function openRedis(a: Account) {
    setRedisTarget(a);
    setBusy(true);
    try {
      const data = await api.get<UserRedis>(`/api/accounts/${a.username}/redis`);
      setRedisInfo(data);
    } catch (err) {
      setRedisInfo(null);
      message.error(err instanceof Error ? err.message : "Cannot load Redis");
    } finally {
      setBusy(false);
    }
  }

  async function saveRedis(body: { enabled?: boolean; rotatePassword?: boolean }) {
    if (!redisTarget) return;
    setBusy(true);
    try {
      const data = await api.put<UserRedis>(`/api/accounts/${redisTarget.username}/redis`, body);
      setRedisInfo(data);
      await load();
      if (data.message) message.success(data.message);
      else message.success("Redis updated");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function randomPassword() {
    try {
      const data = await api.get<{ password: string }>("/api/password/suggest");
      form.setFieldValue("password", data.password);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Cannot generate password");
    }
  }

  function openReset(a: Account) {
    setPassTarget(a);
    setNewPass("");
    passForm.resetFields();
  }

  async function randomResetPassword() {
    try {
      const data = await api.get<{ password: string }>("/api/password/suggest");
      passForm.setFieldValue("password", data.password);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Cannot generate password");
    }
  }

  async function resetPassword(values: { password: string }) {
    if (!passTarget) return;
    setBusy(true);
    try {
      const out = await api.put<{ password: string }>(`/api/accounts/${passTarget.username}/password`, {
        password: values.password,
      });
      setNewPass(out.password || values.password);
      message.success(`Password reset for ${passTarget.username}`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function showPassword(a: Account) {
    try {
      const out = await api.get<{ password: string }>(`/api/accounts/${a.username}/password`);
      modal.info({
        title: `Password · ${a.username}`,
        content: (
          <Typography.Paragraph copyable style={{ marginBottom: 0 }}>
            {out.password}
          </Typography.Paragraph>
        ),
      });
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Password is not stored");
    }
  }

  function cliLabel(v?: string) {
    return v || "system";
  }

  return (
    <div className="cp-page">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Linux accounts
        </Typography.Title>
        <Button type="primary" onClick={() => setCreateOpen(true)}>
          Create account
        </Button>
      </div>
      <Card>
        <Table
          rowKey="id"
          dataSource={list}
          pagination={false}
          locale={{ emptyText: "No hosting accounts yet." }}
          columns={[
            { title: "User", dataIndex: "username" },
            { title: "UID", dataIndex: "linuxUid", width: 90 },
            {
              title: "Status",
              width: 120,
              render: (_, a) => <Tag color={a.suspended ? "warning" : "success"}>{a.suspended ? "Suspended" : "Active"}</Tag>,
            },
            {
              title: "PHP",
              width: 80,
              render: (_, a) => usage[a.username]?.phpProcesses ?? 0,
            },
            {
              title: "RAM",
              width: 110,
              render: (_, a) => formatBytes(usage[a.username]?.memory || 0),
            },
            {
              title: "Disk",
              width: 110,
              render: (_, a) => formatBytes(usage[a.username]?.diskUsed || 0),
            },
            {
              title: "Quota",
              width: 110,
              render: (_, a) => (a.diskQuotaMB ? `${a.diskQuotaMB} MB` : "∞"),
            },
            {
              title: "Redis",
              width: 90,
              render: (_, a) =>
                redisMap[a.username] ? <Tag color="success">On</Tag> : <Tag>Off</Tag>,
            },
            {
              title: "WAF",
              width: 88,
              render: (_, a) => (
                <Switch
                  size="small"
                  checked={a.wafEnabled !== false}
                  loading={wafBusy === a.username}
                  onChange={(v) => void setAccountWAF(a, v)}
                />
              ),
            },
            {
              title: "Default CLI",
              render: (_, a) => (
                <Space size={4} wrap>
                  <Tag>PHP {cliLabel(a.phpCli)}</Tag>
                  <Tag>Python {cliLabel(a.pythonCli)}</Tag>
                  <Tag>Node {cliLabel(a.nodeCli)}</Tag>
                </Space>
              ),
            },
            {
              title: "",
              width: 56,
              align: "right",
              render: (_, a) => (
                <Dropdown
                  trigger={["click"]}
                  menu={{
                    items: [
                      { key: "reset", label: "Reset password" },
                      ...(admin ? [{ key: "showpass", label: "Show password" }] : []),
                      { key: "cli", label: "Edit CLI" },
                      { key: "php", label: "PHP-FPM" },
                      { key: "quota", label: "Quota" },
                      { key: "redis", label: "Redis" },
                      { key: "suspend", label: a.suspended ? "Unsuspend" : "Suspend" },
                      { type: "divider" },
                      { key: "delete", label: "Delete", danger: true },
                    ],
                    onClick: ({ key }) => {
                      if (key === "reset") openReset(a);
                      if (key === "showpass") void showPassword(a);
                      if (key === "cli") openCLI(a);
                      if (key === "php") nav(`/php?user=${a.username}`);
                      if (key === "quota") {
                        setQuotaTarget(a);
                        quotaForm.setFieldsValue({ limitMB: a.diskQuotaMB || 0 });
                      }
                      if (key === "redis") void openRedis(a);
                      if (key === "suspend") void act(a.username, a.suspended ? "unsuspend" : "suspend", "POST");
                      if (key === "delete") {
                        modal.confirm({
                          title: `Delete ${a.username}?`,
                          okText: "Delete",
                          okButtonProps: { danger: true },
                          onOk: () => act(a.username, "", "DELETE"),
                        });
                      }
                    },
                  }}
                >
                  <Button size="small" icon={<MoreOutlined />} />
                </Dropdown>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        title="Create Linux account"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={busy}
        destroyOnHidden
        okText="Create"
      >
        <Form form={form} layout="vertical" onFinish={create} requiredMark={false} style={{ marginTop: 8 }}>
          <Form.Item name="username" label="Username" rules={[{ required: true, min: 3 }]}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item name="password" label="Password" rules={[{ required: true, min: 8 }]}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Button onClick={randomPassword}>Random password</Button>
        </Form>
      </Modal>

      <Modal
        title={passTarget ? `Reset password · ${passTarget.username}` : "Reset password"}
        open={!!passTarget}
        onCancel={() => {
          setPassTarget(null);
          setNewPass("");
        }}
        onOk={() => (newPass ? setPassTarget(null) : passForm.submit())}
        confirmLoading={busy}
        destroyOnHidden
        okText={newPass ? "Done" : "Reset password"}
      >
        <Form form={passForm} layout="vertical" onFinish={resetPassword} requiredMark={false} style={{ marginTop: 8 }}>
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 12 }}
            message="This changes the panel login, SSH, and FTP password for the Linux user."
          />
          {newPass ? (
            <>
              <Typography.Text type="secondary">New password</Typography.Text>
              <Typography.Paragraph copyable style={{ marginBottom: 0 }}>
                {newPass}
              </Typography.Paragraph>
            </>
          ) : (
            <>
              <Form.Item name="password" label="New password" rules={[{ required: true, min: 8 }]}>
                <Input.Password autoComplete="new-password" />
              </Form.Item>
              <Button onClick={() => void randomResetPassword()}>Random password</Button>
            </>
          )}
        </Form>
      </Modal>

      <Modal
        title={cliTarget ? `Default CLI · ${cliTarget.username}` : "Default CLI"}
        open={!!cliTarget}
        onCancel={() => setCliTarget(null)}
        onOk={() => cliForm.submit()}
        confirmLoading={busy}
        destroyOnHidden
        okText="Save CLI"
      >
        <Form form={cliForm} layout="vertical" onFinish={saveCLI} requiredMark={false} style={{ marginTop: 8 }}>
          <Form.Item name="php" label="PHP">
            <Select options={[{ value: "", label: "system" }, ...phpVers.map((v) => ({ value: v, label: v }))]} />
          </Form.Item>
          <Form.Item name="python" label="Python">
            <Select options={[{ value: "", label: "system" }, ...pyVers.map((v) => ({ value: v, label: v }))]} />
          </Form.Item>
          <Form.Item name="nodejs" label="Node">
            <Select options={[{ value: "", label: "system" }, ...nodeVers.map((v) => ({ value: v, label: v }))]} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={quotaTarget ? `Disk quota · ${quotaTarget.username}` : "Quota"}
        open={!!quotaTarget}
        onCancel={() => setQuotaTarget(null)}
        onOk={() => quotaForm.submit()}
        confirmLoading={busy}
        destroyOnHidden
        okText="Save quota"
      >
        <Form
          form={quotaForm}
          layout="vertical"
          onFinish={async (v: { limitMB: number }) => {
            if (!quotaTarget) return;
            setBusy(true);
            try {
              await api.send(`/api/accounts/${quotaTarget.username}/quota`, "PUT", { limitMB: v.limitMB || 0 });
              message.success("Quota saved");
              setQuotaTarget(null);
              await load();
            } catch (err) {
              message.error(err instanceof Error ? err.message : "Failed");
            } finally {
              setBusy(false);
            }
          }}
        >
          <Form.Item name="limitMB" label="Limit (MB)" extra="0 means unlimited. Requires filesystem quota support.">
            <InputNumber min={0} style={{ width: "100%" }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={redisTarget ? `Redis · ${redisTarget.username}` : "Redis"}
        open={!!redisTarget}
        onCancel={() => {
          setRedisTarget(null);
          setRedisInfo(null);
        }}
        footer={<Button onClick={() => { setRedisTarget(null); setRedisInfo(null); }}>Close</Button>}
        destroyOnHidden
      >
        {redisInfo?.installed === false ? (
          <Alert type="warning" showIcon message={redisInfo.message || "Install Redis from Software first."} style={{ marginBottom: 12 }} />
        ) : null}
        <Space direction="vertical" size={12} style={{ width: "100%", marginTop: 8 }}>
          <Space wrap align="center">
            <Switch
              checked={!!redisInfo?.enabled}
              loading={busy}
              disabled={redisInfo?.installed === false}
              onChange={(on) => void saveRedis({ enabled: on })}
            />
            <Typography.Text>Enable Redis for this account</Typography.Text>
          </Space>
          <Typography.Text type="secondary">
            Uses the system Redis with an ACL user, a random password, and isolated keys under the prefix{" "}
            {redisTarget ? `${redisTarget.username}:` : "user:"}.
          </Typography.Text>
          {redisInfo?.enabled ? (
            <>
              <div>
                <Typography.Text type="secondary">Host</Typography.Text>
                <Typography.Paragraph copyable style={{ marginBottom: 8 }}>
                  {redisInfo.host || "127.0.0.1"}
                </Typography.Paragraph>
                <Typography.Text type="secondary">Port</Typography.Text>
                <Typography.Paragraph copyable style={{ marginBottom: 8 }}>
                  {String(redisInfo.port || 6379)}
                </Typography.Paragraph>
                <Typography.Text type="secondary">Username</Typography.Text>
                <Typography.Paragraph copyable style={{ marginBottom: 8 }}>
                  {redisInfo.redisUser || redisTarget?.username}
                </Typography.Paragraph>
                <Typography.Text type="secondary">Password</Typography.Text>
                <Typography.Paragraph copyable={{ text: redisInfo.password || "" }} style={{ marginBottom: 8 }}>
                  {redisInfo.password || "—"}
                </Typography.Paragraph>
                <Typography.Text type="secondary">Prefix</Typography.Text>
                <Typography.Paragraph copyable style={{ marginBottom: 8 }}>
                  {redisInfo.prefix || ""}
                </Typography.Paragraph>
                <Typography.Text type="secondary">Credentials file</Typography.Text>
                <Typography.Paragraph copyable style={{ marginBottom: 8 }}>
                  {redisInfo.file || ""}
                </Typography.Paragraph>
              </div>
              <Button loading={busy} onClick={() => void saveRedis({ rotatePassword: true })}>
                Randomize password
              </Button>
            </>
          ) : null}
        </Space>
      </Modal>
    </div>
  );
}
