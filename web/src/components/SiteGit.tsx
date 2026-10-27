import { useEffect, useMemo, useRef, useState } from "react";
import { Alert, Button, Form, Input, Select, Space, Typography } from "antd";
import { api } from "@/lib/api";
import { formatPanelTime } from "@/lib/panelTime";
import { PathPicker } from "@/components/PathPicker";

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
  needsKey?: boolean;
  lastLog?: string;
  lastAt?: string;
  commit?: { hash?: string; subject?: string; author?: string; at?: string };
  laravelHint?: string;
  npmHint?: string;
};

type GitBranchList = {
  ok?: boolean;
  branches?: string[];
  head?: string;
  branch?: string;
  repo?: string;
  message?: string;
  needsKey?: boolean;
};

export function SiteGit({ siteId, domain, username, docRoot }: { siteId: number; domain: string; username: string; docRoot: string }) {
  const [form] = Form.useForm();
  const [info, setInfo] = useState<SiteGitInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [branches, setBranches] = useState<string[]>([]);
  const [head, setHead] = useState("");
  const [branchBusy, setBranchBusy] = useState(false);
  const [branchErr, setBranchErr] = useState("");
  const [branchNeedsKey, setBranchNeedsKey] = useState(false);
  const loadedRepo = useRef("");
  const branchSeq = useRef(0);
  const siteGen = useRef(0);
  const branchValue = Form.useWatch("branch", form);

  async function load(gen: number) {
    setErr("");
    try {
      const out = await api.get<SiteGitInfo>(`/api/sites/${siteId}/git`);
      if (gen !== siteGen.current) return;
      setInfo(out);
      form.setFieldsValue({
        repo: out.repo || "",
        branch: out.branch || "main",
        path: out.path || "",
        command: out.command || "",
      });
      if (out.repo) void loadBranches(out.repo);
    } catch (e) {
      if (gen !== siteGen.current) return;
      setErr(e instanceof Error ? e.message : "Failed to load git deploy");
    }
  }

  useEffect(() => {
    const gen = ++siteGen.current;
    loadedRepo.current = "";
    branchSeq.current += 1;
    setBranches([]);
    setHead("");
    setBranchErr("");
    setBranchNeedsKey(false);
    void load(gen);
  }, [siteId]);

  async function save(values: { repo: string; branch: string; path: string; command: string }) {
    setBusy(true);
    setErr("");
    try {
      const out = await api.put<SiteGitInfo>(`/api/sites/${siteId}/git`, values);
      setInfo(out);
      form.setFieldsValue({
        repo: out.repo || values.repo,
        branch: out.branch || values.branch,
        path: out.path || values.path,
        command: out.command || values.command,
      });
      if (out.lastOk === false && !out.needsKey) setErr("Could not pull the branch");
      if (!out.needsKey) setBranchNeedsKey(false);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function loadBranches(repoRaw?: string) {
    const gen = siteGen.current;
    const repo = (repoRaw ?? String(form.getFieldValue("repo") || "")).trim();
    if (!repo || repo === loadedRepo.current) return;
    const seq = ++branchSeq.current;
    setBranchBusy(true);
    setBranchErr("");
    try {
      const out = await api.post<GitBranchList>(`/api/sites/${siteId}/git/branches`, { repo });
      if (gen !== siteGen.current || seq !== branchSeq.current) return;
      if (out.branch) {
        form.setFieldValue("branch", out.branch);
        if (out.repo) form.setFieldValue("repo", out.repo);
      }
      if (!out.ok) {
        setBranchNeedsKey(!!out.needsKey);
        setBranchErr(out.needsKey ? "Add the SSH public key as a deploy key, then open the list again." : out.message || "Could not list branches");
        return;
      }
      const names = out.branches || [];
      setBranches(names);
      setHead(out.head || "");
      setBranchNeedsKey(false);
      loadedRepo.current = out.branch && out.repo ? out.repo : repo;
      if (!out.branch && out.head) {
        const cur = String(form.getFieldValue("branch") || "");
        if (!cur || (cur === "main" && !names.includes("main"))) form.setFieldValue("branch", out.head);
      }
    } catch (e) {
      if (gen !== siteGen.current || seq !== branchSeq.current) return;
      setBranchErr(e instanceof Error ? e.message : "Could not list branches");
    } finally {
      if (gen === siteGen.current && seq === branchSeq.current) setBranchBusy(false);
    }
  }

  async function deploy() {
    try {
      const values = await form.validateFields();
      await save(values);
    } catch {
      // The form shows its own validation message.
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

  const showKey = Boolean(info?.needsKey || branchNeedsKey);
  const branchOptions = useMemo(() => {
    const names = [...branches];
    const current = typeof branchValue === "string" ? branchValue : "";
    if (current && !names.includes(current)) names.unshift(current);
    return names.map((name) => ({
      value: name,
      label: head && name === head ? `${name} (default)` : name,
    }));
  }, [branches, branchValue, head]);

  return (
    <Space direction="vertical" size={12} style={{ width: "100%" }}>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
        Add the account SSH public key as a deploy key on GitHub, GitLab, or Gitea. The webhook always force-pulls{" "}
        <code>{domain}</code> as <code>{username}</code>, then runs your command.
      </Typography.Paragraph>
      {err ? <Alert type="error" showIcon message={err} /> : null}
      {showKey ? (
        <Alert
          type="warning"
          showIcon
          message="Private repository"
          description="The server could not read this repository. Add the SSH public key below as a deploy key, then pull again."
        />
      ) : null}
      {info?.publicKey ? (
        <Alert
          type={showKey ? "warning" : "info"}
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
        <Form.Item label="Repository" extra="HTTPS, or git@github.com:org/repo.git. Pull fetches the branch. A private repository needs the SSH key below.">
          <Space.Compact style={{ width: "100%" }}>
            <Form.Item name="repo" noStyle rules={[{ required: true, message: "Repo required" }]}>
              <Input placeholder="git@github.com:org/repo.git" onBlur={() => void loadBranches()} />
            </Form.Item>
            <Button type="primary" loading={busy} onClick={() => void deploy()}>
              Pull
            </Button>
          </Space.Compact>
        </Form.Item>
        <Form.Item name="branch" label="Branch" extra={branchErr || "Webhook and Deploy now fetch this branch."}>
          <Select
            showSearch
            loading={branchBusy}
            options={branchOptions}
            placeholder="main"
            optionFilterProp="label"
            onOpenChange={(open) => {
              if (open) void loadBranches();
            }}
            notFoundContent={branchBusy ? "Reading branches…" : branchErr || "No branches yet"}
          />
        </Form.Item>
        <Form.Item
          name="path"
          label="Deploy path"
          extra={`Inside /home/${username}/. For Laravel, use the app root (parent of public/), not only ${docRoot.split("/").slice(-1)[0] || "public_html"}.`}
        >
          <PathPicker user={username || "root"} dirsOnly relativeTo={username ? `/home/${username}` : undefined} placeholder="domains/example.com" />
        </Form.Item>
        <Form.Item name="command" label="After pull" extra="Runs as the site user in the deploy path. Leave empty to only force-pull.">
          <Input.TextArea rows={8} spellCheck={false} placeholder={info?.laravelHint || "composer install --no-dev"} style={{ fontFamily: "ui-monospace, Menlo, Consolas, monospace" }} />
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
      {info?.commit?.subject ? (
        <Alert
          type="info"
          showIcon
          message="Latest deploy"
          description={
            <Space direction="vertical" size={2}>
              <Typography.Text>
                <Typography.Text code>{info.commit.hash}</Typography.Text> {info.commit.subject}
              </Typography.Text>
              {info.commit.author || info.commit.at ? (
                <Typography.Text type="secondary">
                  {[info.commit.author, formatPanelTime(info.commit.at) || info.commit.at].filter(Boolean).join(" · ")}
                </Typography.Text>
              ) : null}
            </Space>
          }
        />
      ) : null}
      {info?.lastLog || info?.lastAt ? (
        <div>
          <Typography.Text type="secondary">
            Last deploy{info.lastAt ? ` · ${formatPanelTime(info.lastAt) || info.lastAt}` : ""} {info.lastOk ? "ok" : "failed"}
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
