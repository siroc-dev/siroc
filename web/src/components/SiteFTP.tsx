import { useEffect, useState } from "react";
import { App, Alert, Button, Form, Input, Popconfirm, Space, Table, Typography } from "antd";
import { api } from "@/lib/api";
import { PathPicker } from "@/components/PathPicker";

type FTPRow = { id: number; login: string; home: string; username: string };

function absPath(user: string, raw: string) {
  const path = raw.trim().replace(/\\/g, "/").replace(/\/+$/, "");
  if (!path) return `/home/${user}`;
  if (path.startsWith("/")) return path;
  return `/home/${user}/${path}`.replace(/\/+$/, "");
}

function sitePath(user: string, docRoot: string) {
  const root = docRoot.trim().replace(/\\/g, "/");
  const prefix = `/home/${user}/`;
  if (root.startsWith(prefix)) return root.slice(prefix.length).replace(/\/+$/, "");
  if (root === `/home/${user}`) return "";
  return root;
}

function forThisSite(row: FTPRow, user: string, docRoot: string) {
  const root = absPath(user, docRoot);
  const home = absPath(user, row.home);
  return home === root || home.startsWith(root + "/");
}

export function SiteFTP({
  username,
  domain,
  docRoot,
  admin,
}: {
  username: string;
  domain: string;
  docRoot: string;
  admin?: boolean;
}) {
  const { message, modal } = App.useApp();
  const [form] = Form.useForm();
  const [passForm] = Form.useForm();
  const homeWatch = Form.useWatch("home", form) as string | undefined;
  const [rows, setRows] = useState<FTPRow[]>([]);
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState("");
  const [passTarget, setPassTarget] = useState<FTPRow | null>(null);
  const [newPass, setNewPass] = useState("");
  const locked = sitePath(username, docRoot);

  async function load() {
    const list = await api.get<FTPRow[]>(`/api/ftp?user=${encodeURIComponent(username)}`);
    setRows(list.filter((row) => forThisSite(row, username, docRoot)));
  }

  useEffect(() => {
    form.setFieldValue("home", locked);
    load().catch((err) => message.error(err instanceof Error ? err.message : "Cannot load FTP users"));
  }, [username, docRoot]);

  async function randomPassword(target: "create" | "reset") {
    try {
      const data = await api.get<{ password: string }>("/api/password/suggest");
      (target === "create" ? form : passForm).setFieldValue("password", data.password);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Cannot generate password");
    }
  }

  async function add(values: { name: string; password: string; home?: string }) {
    setBusy(true);
    try {
      const home = (values.home || locked).trim() || locked;
      if (!forThisSite({ id: 0, login: "", home, username }, username, docRoot)) {
        message.error("Path must stay inside this website");
        return;
      }
      const out = await api.post<{ user: FTPRow; password: string }>("/api/ftp", {
        username,
        name: values.name.trim(),
        password: values.password,
        home,
      });
      setCreated(`${out.user.login}  ${out.password}`);
      form.resetFields();
      form.setFieldValue("home", locked);
      await load();
      message.success("FTP user created");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function showPassword(row: FTPRow) {
    try {
      const out = await api.get<{ password: string }>(`/api/ftp/${row.id}/password`);
      modal.info({
        title: `FTP password · ${row.login}`,
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

  async function resetPassword(values: { password: string }) {
    if (!passTarget) return;
    setBusy(true);
    try {
      const out = await api.put<{ password: string }>(`/api/ftp/${passTarget.id}/password`, { password: values.password });
      setNewPass(out.password || values.password);
      message.success(`FTP password reset for ${passTarget.login}`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function remove(row: FTPRow) {
    setBusy(true);
    try {
      await api.delete(`/api/ftp/${row.id}`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  const pathAbs = (homeWatch || "").startsWith("/");

  return (
    <>
      <Typography.Paragraph type="secondary">
        Logins here can only see {domain}. Connect to this server on port 21. The login is {username}_name.
      </Typography.Paragraph>
      <Form form={form} layout="vertical" onFinish={add} requiredMark={false} initialValues={{ home: locked }}>
        <Space align="start" wrap>
          <Form.Item name="name" label="Name" rules={[{ required: true, pattern: /^[a-z][a-z0-9]{1,15}$/, message: "2–16 letters or digits, starting with a letter" }]}>
            <Input placeholder="files" autoComplete="off" style={{ width: 140 }} />
          </Form.Item>
          <Form.Item label="Password" required>
            <Space.Compact>
              <Form.Item name="password" noStyle rules={[{ required: true, min: 8, message: "At least 8 characters" }]}>
                <Input.Password autoComplete="new-password" style={{ width: 180 }} />
              </Form.Item>
              <Button htmlType="button" onClick={() => void randomPassword("create")}>
                Random
              </Button>
            </Space.Compact>
          </Form.Item>
          <Form.Item name="home" label="Path">
            <PathPicker
              user={username || "root"}
              dirsOnly
              relativeTo={pathAbs ? undefined : `/home/${username}`}
              addonBefore={pathAbs ? undefined : `/home/${username}/`}
              placeholder={locked || "domains/site.test/public_html"}
              style={{ width: 420 }}
            />
          </Form.Item>
        </Space>
        <Button type="primary" htmlType="submit" loading={busy}>
          Add FTP user
        </Button>
      </Form>
      {created ? (
        <Alert style={{ marginTop: 12 }} type="success" showIcon message="Login and password" description={<Typography.Text copyable>{created}</Typography.Text>} />
      ) : null}
      <Table
        style={{ marginTop: 16 }}
        size="small"
        rowKey="id"
        pagination={false}
        dataSource={rows}
        locale={{ emptyText: "No FTP users for this website yet." }}
        columns={[
          { title: "Login", dataIndex: "login" },
          { title: "Path", dataIndex: "home", ellipsis: true },
          {
            title: "",
            width: 200,
            align: "right",
            render: (_, row) => (
              <Space size={0}>
                {admin ? (
                  <Button size="small" type="link" onClick={() => void showPassword(row)}>
                    Show
                  </Button>
                ) : null}
                <Button
                  size="small"
                  type="link"
                  onClick={() => {
                    setPassTarget(row);
                    setNewPass("");
                    passForm.resetFields();
                  }}
                >
                  Reset
                </Button>
                <Popconfirm title={`Delete ${row.login}?`} onConfirm={() => void remove(row)}>
                  <Button size="small" type="link" danger>
                    Delete
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />
      {passTarget ? (
        <div style={{ marginTop: 16 }}>
          <Typography.Text strong>Reset password · {passTarget.login}</Typography.Text>
          {newPass ? (
            <Typography.Paragraph copyable style={{ margin: "8px 0 0" }}>
              {newPass}
            </Typography.Paragraph>
          ) : (
            <Form form={passForm} layout="inline" onFinish={resetPassword} requiredMark={false} style={{ marginTop: 8 }}>
              <Form.Item name="password" rules={[{ required: true, min: 8 }]} style={{ marginBottom: 0 }}>
                <Input.Password autoComplete="new-password" placeholder="New password" style={{ width: 220 }} />
              </Form.Item>
              <Button htmlType="button" onClick={() => void randomPassword("reset")}>
                Random
              </Button>
              <Button type="primary" htmlType="submit" loading={busy}>
                Reset password
              </Button>
            </Form>
          )}
        </div>
      ) : null}
    </>
  );
}
