import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Checkbox, Popconfirm, Select, Space, Table, Typography } from "antd";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";

type RAIDArray = { device: string; level?: string; state?: string; size?: number; devices?: string[] };
type RAIDDisk = { path: string; name: string; type: string; size?: number; model?: string; transport?: string };
type RAIDStatus = { installed: boolean; arrays?: RAIDArray[]; disks?: RAIDDisk[]; message?: string };

const levels = [
  { value: "1", label: "RAID 1 (mirror)", extra: "Two or more disks. Data is copied on each disk. One disk can fail." },
  { value: "0", label: "RAID 0 (stripe)", extra: "Two or more disks. Faster, and one failed disk loses everything." },
  { value: "5", label: "RAID 5 (parity)", extra: "Three or more disks. One disk can fail." },
  { value: "10", label: "RAID 10 (mirror + stripe)", extra: "Four or more disks, even count. One disk in each pair can fail." },
];

export function RAIDManager() {
  const { message } = App.useApp();
  const [st, setSt] = useState<RAIDStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [level, setLevel] = useState("1");
  const [picked, setPicked] = useState<string[]>([]);

  async function load() {
    setSt(await api.get<RAIDStatus>("/api/sysops/raid"));
  }

  useEffect(() => {
    load().catch((e) => message.error(e instanceof Error ? e.message : "Failed"));
  }, []);

  async function run(fn: () => Promise<RAIDStatus>, ok?: string) {
    setBusy(true);
    try {
      const next = await fn();
      setSt(next);
      setPicked([]);
      if (ok) message.success(ok);
      else if (next.message) message.success(next.message);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  const spec = levels.find((l) => l.value === level) || levels[0];
  const disks = st?.disks || [];
  const arrays = st?.arrays || [];

  return (
    <Card title="RAID">
      {!st ? (
        <Typography.Text type="secondary">Reading disks…</Typography.Text>
      ) : !st.installed ? (
        <Space direction="vertical">
          <Alert type="info" showIcon message={st.message || "Install mdadm to create a software RAID array."} />
          <Button type="primary" loading={busy} onClick={() => void run(() => api.post<RAIDStatus>("/api/sysops/raid/install"), "mdadm installed")}>
            Install mdadm
          </Button>
        </Space>
      ) : (
        <Space direction="vertical" size={16} style={{ width: "100%" }}>
          {st.message && arrays.length ? <Alert type="success" showIcon message={st.message} /> : null}
          <Table
            size="small"
            rowKey="device"
            pagination={false}
            dataSource={arrays}
            locale={{ emptyText: "No RAID arrays yet." }}
            columns={[
              { title: "Device", dataIndex: "device" },
              { title: "Level", dataIndex: "level", width: 110 },
              { title: "State", dataIndex: "state", width: 110 },
              { title: "Size", width: 120, render: (_, row) => (row.size ? formatBytes(row.size) : "") },
              { title: "Disks", render: (_, row) => (row.devices || []).join(", ") },
            ]}
          />
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            Only unused disks are listed. The system disk is hidden. Creating an array erases those disks. After it is created, format it if needed and mount it below.
          </Typography.Paragraph>
          <Table
            size="small"
            rowKey="path"
            pagination={false}
            dataSource={disks}
            locale={{ emptyText: "No unused disks." }}
            columns={[
              {
                title: "",
                width: 48,
                render: (_, row) => (
                  <Checkbox
                    checked={picked.includes(row.path)}
                    onChange={(e) => setPicked(e.target.checked ? [...picked, row.path] : picked.filter((p) => p !== row.path))}
                  />
                ),
              },
              { title: "Disk", dataIndex: "path" },
              { title: "Type", dataIndex: "type", width: 80 },
              { title: "Size", width: 120, render: (_, row) => (row.size ? formatBytes(row.size) : "") },
              { title: "Model", render: (_, row) => [row.model, row.transport].filter(Boolean).join(" · ") },
            ]}
          />
          <Space wrap>
            <Select value={level} style={{ minWidth: 260 }} options={levels} onChange={setLevel} />
            <Popconfirm
              title={`Create ${spec.label}?`}
              description={`This erases ${picked.join(", ") || "the selected disks"}.`}
              okText="Create"
              disabled={!picked.length}
              onConfirm={() => void run(() => api.post<RAIDStatus>("/api/sysops/raid", { level, devices: picked }))}
            >
              <Button type="primary" loading={busy} disabled={!picked.length}>
                Create array
              </Button>
            </Popconfirm>
          </Space>
          <Typography.Text type="secondary">{spec.extra}</Typography.Text>
        </Space>
      )}
    </Card>
  );
}
