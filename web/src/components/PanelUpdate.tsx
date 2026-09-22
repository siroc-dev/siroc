import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Form, Input, Space, Tag, Typography } from "antd";
import { api } from "@/lib/api";

type UpdateStatus = {
  ok: boolean;
  version: string;
  latest?: string;
  available?: boolean;
  channel?: string;
  packageUrl?: string;
  checkedAt?: string;
  restarting?: boolean;
  message?: string;
};

export function PanelUpdate() {
  const { message } = App.useApp();
  const [st, setSt] = useState<UpdateStatus | null>(null);
  const [busy, setBusy] = useState("");
  const [form] = Form.useForm();

  async function load() {
    const data = await api.get<UpdateStatus>("/api/panel/update");
    setSt(data);
    form.setFieldsValue({ channel: data.channel || "https://get.siroc.dev", url: data.packageUrl || "", path: "" });
  }

  useEffect(() => {
    load().catch((e) => message.error(e.message));
  }, []);

  async function run(action: string, extra?: Record<string, string>) {
    setBusy(action);
    try {
      const values = form.getFieldsValue();
      const data = await api.post<UpdateStatus>("/api/panel/update", {
        action,
        channel: values.channel || "",
        url: values.url || "",
        path: values.path || "",
        ...extra,
      });
      setSt(data);
      if (data.restarting) {
        message.success("Update applied. The panel will reconnect in a few seconds.");
        setTimeout(() => {
          load().catch(() => undefined);
        }, 4000);
      } else {
        message.success(data.message || "Done");
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy("");
    }
  }

  return (
    <Card title="Siroc updates">
      <Space direction="vertical" size={12} style={{ width: "100%" }}>
        <Space wrap>
          <Tag color="blue">Installed {st?.version || "…"}</Tag>
          {st?.latest ? <Tag color={st.available ? "gold" : "success"}>Latest {st.latest}</Tag> : null}
          {st?.available ? <Tag color="warning">Update available</Tag> : null}
          {st?.checkedAt ? <Typography.Text type="secondary">Checked {st.checkedAt}</Typography.Text> : null}
        </Space>
        {st?.message ? <Alert type={st.available ? "info" : "success"} showIcon message={st.message} /> : null}
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          From the server: <code>sudo siroc update</code> to check, <code>sudo siroc upgrade</code> to apply.
          Or build a package with <code>./scripts/package.sh</code> and publish <code>latest.json</code>:
        </Typography.Paragraph>
        <pre style={{ margin: 0, padding: 12, background: "#f8fafc", borderRadius: 8, fontSize: 12 }}>
{`{
  "version": "0.2.4",
  "url": "https://get.siroc.dev/siroc-linux-amd64.tar.gz",
  "sha256": "optional"
}`}
        </pre>
        <Form form={form} layout="vertical">
          <Form.Item name="channel" label="Update channel" extra="Folder URL that contains latest.json, or a direct latest.json URL.">
            <Input placeholder="https://get.siroc.dev" />
          </Form.Item>
          <Form.Item name="url" label="Package URL" extra="Direct .tar.gz if you are not using a channel.">
            <Input placeholder="https://get.siroc.dev/siroc-linux-amd64.tar.gz" />
          </Form.Item>
          <Form.Item
            name="path"
            label="Local package path"
            extra="Absolute path on this server. Allowed: /var/lib/siroc, /opt/siroc, /root, /tmp."
          >
            <Input placeholder="/var/lib/siroc/updates/siroc-linux-amd64.tar.gz" />
          </Form.Item>
          <Space wrap>
            <Button loading={busy === "check"} onClick={() => void run("check")}>
              Check for updates
            </Button>
            <Button type="primary" loading={busy === "apply"} onClick={() => void run("apply")}>
              Apply update
            </Button>
          </Space>
        </Form>
      </Space>
    </Card>
  );
}
