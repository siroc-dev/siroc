import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Popconfirm, Space, Table, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { PageSkeleton } from "@/components/PageSkeleton";

type OSPackage = {
  name: string;
  current: string;
  available: string;
  source?: string;
};

type OSUpdateStatus = {
  ok: boolean;
  os?: string;
  checkedAt?: string;
  count: number;
  packages?: OSPackage[];
  message?: string;
  log?: string;
};

export function OSUpdates() {
  const { message } = App.useApp();
  const [st, setSt] = useState<OSUpdateStatus | null>(null);
  const [busy, setBusy] = useState("");
  const [ready, setReady] = useState(false);

  async function load() {
    setSt(await api.get<OSUpdateStatus>("/api/os/updates"));
  }

  useEffect(() => {
    load()
      .catch((e) => message.error(e.message))
      .finally(() => setReady(true));
  }, []);

  async function run(action: "update" | "upgrade") {
    setBusy(action);
    try {
      const data = await api.post<OSUpdateStatus>("/api/os/updates", { action });
      setSt(data);
      message.success(data.message || "Done");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
      load().catch(() => undefined);
    } finally {
      setBusy("");
    }
  }

  const packages = st?.packages || [];

  if (!ready) return <PageSkeleton />;

  return (
    <div className="cp-page">
      <Typography.Title level={3} style={{ margin: 0 }}>
        OS updates
      </Typography.Title>
      <Card>
        <Space direction="vertical" size={12} style={{ width: "100%" }}>
          <Space wrap>
            {st?.os ? <Tag>{st.os}</Tag> : null}
            <Tag color={st?.count ? "gold" : "success"}>{st?.count || 0} waiting</Tag>
            {st?.checkedAt ? <Typography.Text type="secondary">Checked {st.checkedAt}</Typography.Text> : null}
          </Space>
          {st?.message ? <Alert type={st.count ? "info" : st.ok === false ? "warning" : "success"} showIcon message={st.message} /> : null}
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            Update runs <code>apt-get update</code> and refreshes this list. Upgrade runs <code>apt-get upgrade</code> and keeps existing config files. Services that need a restart are restarted. The list is checked every day at 04:15 (server timezone) and is not installed until you choose Upgrade.
          </Typography.Paragraph>
          <Space wrap>
            <Button loading={busy === "update"} onClick={() => void run("update")}>
              Update
            </Button>
            <Popconfirm
              title="Upgrade the operating system packages?"
              description="This installs every package in the list. Running services may restart."
              okText="Upgrade"
              onConfirm={() => void run("upgrade")}
            >
              <Button type="primary" loading={busy === "upgrade"} disabled={!st?.count && busy !== "upgrade"}>
                Upgrade
              </Button>
            </Popconfirm>
          </Space>
        </Space>
      </Card>
      <Card title="Packages">
        <Table
          rowKey="name"
          size="small"
          pagination={{ pageSize: 50, hideOnSinglePage: true }}
          dataSource={packages}
          locale={{ emptyText: "No packages waiting" }}
          columns={[
            { title: "Package", dataIndex: "name" },
            { title: "Installed", dataIndex: "current" },
            { title: "Available", dataIndex: "available" },
            { title: "Source", dataIndex: "source" },
          ]}
        />
      </Card>
      {st?.log ? (
        <Card title="Upgrade log">
          <pre className="os-update-log">{st.log}</pre>
        </Card>
      ) : null}
    </div>
  );
}
