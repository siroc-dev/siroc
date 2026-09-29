import { useEffect, useState } from "react";
import { Alert, Button, Form, Input, Space, Typography } from "antd";
import { api } from "@/lib/api";

export type SiteGitInfo = {
  repo?: string;
  branch?: string;
  path?: string;
  command?: string;
  webhookUrl?: string;
  publicKey?: string;
  keyPath?: string;
  fingerprint?: string;
  lastOk?: boolean;
  lastLog?: string;
  lastAt?: string;
  laravelHint?: string;
  npmHint?: string;
};

export function SiteGit({ siteId, domain, username, docRoot }: { siteId: number; domain: string; username: string; docRoot: string }) {
  const [form] = Form.useForm();
  const [info, setInfo] = useState<SiteGitInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function load() {
    setErr("");
    try {
      const out = await api.get<SiteGitInfo>(`/api/sites/${siteId}/git`);
      setInfo(out);
      form.setFieldsValue({
        repo: out.repo || "",
        branch: out.branch || "main",
        path: out.path || "",
        command: out.command || "",
      });
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to load git deploy");
    }
  }

  useEffect(() => {
    void load();
  }, [siteId]);

  async function save(values: { repo: string; branch: string; path: string; command: string }) {
    setBusy(true);
    setErr("");
    try {
      const out = await api.put<SiteGitInfo>(`/api/sites/${siteId}/git`, values);
      setInfo(out);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function deploy() {
    setBusy(true);
    setErr("");
    try {
      await form.validateFields();
      await save(form.getFieldsValue());
      const out = await api.post<{ log?: string; message?: string }>(`/api/sites/${siteId}/git/deploy`, {});
      setInfo((cur) => ({ ...(cur || {}), lastOk: true, lastLog: out.log || out.message || "Deployed" }));
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Deploy failed");
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function rotate() {
    setBusy(true);
    try {
      const out = await api.post<SiteGitInfo>(`/api/sites/${siteId}/git/token`, {});
      setInfo(out);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function copy(text?: string) {
    if (!text) return;
    await navigator.clipboard.writeText(text);
  }

  return (
    <Space direction="vertical" size={12} style={{ width: "100%" }}>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
        Add the account SSH public key as a deploy key on GitHub, GitLab, or Gitea. The webhook always force-pulls{" "}
        <code>{domain}</code> as <code>{username}</code>, then runs your command.
      </Typography.Paragraph>
      {err ? <Alert type="error" showIcon message={err} /> : null}
      {info?.publicKey ? (
        <Alert
          type="info"
          showIcon
          message="SSH public key"
          description={
            <Space direction="vertical" size={6} style={{ width: "100%" }}>
              {info.fingerprint ? <Typography.Text type="secondary">{info.fingerprint}</Typography.Text> : null}
              <Input.TextArea value={info.publicKey} readOnly autoSize style={{ fontFamily: "ui-monospace, Menlo, Consolas, monospace", fontSize: 12 }} />
              <Button size="small" onClick={() => void copy(info.publicKey)}>
                Copy public key
              </Button>
            </Space>
          }
        />
      ) : (
        <Alert type="warning" showIcon message="Could not read the Linux user SSH key yet. Save the repo after the agent is online." />
      )}
      <Form form={form} layout="vertical" onFinish={save} requiredMark={false}>
        <Form.Item name="repo" label="SSH repo" rules={[{ required: true, message: "Repo required" }]}>
          <Input placeholder="git@github.com:org/repo.git" />
        </Form.Item>
        <Form.Item name="branch" label="Branch" extra="Webhook and Deploy now always fetch and reset --hard to this branch.">
          <Input placeholder="main" />
        </Form.Item>
        <Form.Item
          name="path"
          label="Deploy path"
          extra={`Inside /home/${username}/. For Laravel, use the app root (parent of public/), not only ${docRoot.split("/").slice(-1)[0] || "public_html"}.`}
        >
          <Input placeholder="domains/example.com" />
        </Form.Item>
        <Form.Item name="command" label="After pull" extra="Runs as the site user in the deploy path. Leave empty to only force-pull.">
          <Input.TextArea rows={6} spellCheck={false} placeholder={info?.laravelHint || "composer install --no-dev"} style={{ fontFamily: "ui-monospace, Menlo, Consolas, monospace" }} />
        </Form.Item>
        <Space wrap>
          <Button type="primary" htmlType="submit" loading={busy}>
            Save
          </Button>
          <Button loading={busy} onClick={() => form.setFieldValue("command", info?.laravelHint || "")}>
            Laravel command
          </Button>
          <Button loading={busy} onClick={() => form.setFieldValue("command", info?.npmHint || "npm install\nnpm run build")}>
            npm install and build
          </Button>
          <Button type="primary" loading={busy} onClick={() => void deploy()}>
            Deploy now
          </Button>
        </Space>
      </Form>
      {info?.webhookUrl ? (
        <Alert
          type="success"
          showIcon
          message="Webhook"
          description={
            <Space direction="vertical" size={6} style={{ width: "100%" }}>
              <Typography.Text type="secondary">POST this URL from GitHub/GitLab. It answers immediately, then force-pulls and runs the command. Refresh this page for the last log.</Typography.Text>
              <Input value={info.webhookUrl} readOnly />
              <Space>
                <Button size="small" onClick={() => void copy(info.webhookUrl)}>
                  Copy webhook
                </Button>
                <Button size="small" onClick={() => void rotate()} loading={busy}>
                  New token
                </Button>
              </Space>
            </Space>
          }
        />
      ) : null}
      {info?.lastLog || info?.lastAt ? (
        <div>
          <Typography.Text type="secondary">
            Last deploy{info.lastAt ? ` · ${info.lastAt}` : ""} {info.lastOk ? "ok" : "failed"}
          </Typography.Text>
          <pre
            style={{
              margin: "8px 0 0",
              maxHeight: 280,
              overflow: "auto",
              padding: 12,
              background: "#0f172a",
              color: "#e2e8f0",
              borderRadius: 8,
              fontSize: 12,
            }}
          >
            {info.lastLog || "No log yet"}
          </pre>
        </div>
      ) : null}
    </Space>
  );
}
