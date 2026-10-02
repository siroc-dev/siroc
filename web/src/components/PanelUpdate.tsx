import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Space, Tag, Typography } from "antd";
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

const DEFAULT_CHANNEL = "https://get.siroc.dev";

export function PanelUpdate() {
  const { message } = App.useApp();
  const [st, setSt] = useState<UpdateStatus | null>(null);
  const [busy, setBusy] = useState("");

  async function load() {
    const data = await api.get<UpdateStatus>("/api/panel/update");
    setSt(data);
  }

  useEffect(() => {
    load().catch((e) => message.error(e.message));
  }, []);

  async function run(action: string) {
    setBusy(action);
    try {
      const data = await api.post<UpdateStatus>("/api/panel/update", { action });
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

  const channel = st?.channel || DEFAULT_CHANNEL;

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
          The panel checks the public channel every day at 04:00 (server timezone). On the server you can also run{" "}
          <code>sudo siroc update</code> then <code>sudo siroc upgrade</code>.
        </Typography.Paragraph>
        <Typography.Text>
          Channel <code>{channel}</code>
        </Typography.Text>
        <Space wrap>
          <Button loading={busy === "check"} onClick={() => void run("check")}>
            Check for updates
          </Button>
          <Button type="primary" loading={busy === "apply"} onClick={() => void run("apply")}>
            Apply update
          </Button>
        </Space>
      </Space>
    </Card>
  );
}
