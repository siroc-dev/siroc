import { useEffect, useState } from "react";
import { App, Button, Card, Form, Input, Modal, Space, Table, Typography } from "antd";
import { api } from "@/lib/api";
import { formatPanelTime } from "@/lib/panelTime";

type HostRow = { id: number; ip: string; names: string[]; created?: string };

function hostWhen(raw?: string) {
  return formatPanelTime(raw) || "—";
}

function splitNames(raw: string) {
  return raw.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean);
}

export function HostsEditor() {
  const { message, modal } = App.useApp();
  const [rows, setRows] = useState<HostRow[]>([]);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [draft, setDraft] = useState<{ id: number; ip: string; names: string } | null>(null);

  async function load() {
    const data = await api.get<{ entries?: HostRow[] }>("/api/sysops/hosts");
    setRows(data.entries || []);
  }

  useEffect(() => {
    load()
      .catch((e) => message.error(e instanceof Error ? e.message : "Failed"))
      .finally(() => setReady(true));
  }, []);

  async function save() {
    if (!draft) return;
    const names = splitNames(draft.names);
    if (!draft.ip.trim() || !names.length) {
      message.error("Enter an IP address and at least one hostname");
      return;
    }
    setBusy(true);
    try {
      const body = { ip: draft.ip.trim(), names };
      const data =
        draft.id > 0
          ? await api.put<{ entries?: HostRow[] }>(`/api/sysops/hosts/${draft.id}`, body)
          : await api.post<{ entries?: HostRow[] }>("/api/sysops/hosts", body);
      setRows(data.entries || []);
      setDraft(null);
      message.success(draft.id > 0 ? "Host updated" : "Host added");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  function remove(row: HostRow) {
    modal.confirm({
      title: `Delete ${row.names.join(" ")}?`,
      content: `${row.ip} will be removed from /etc/hosts.`,
      okText: "Delete",
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          const data = await api.delete<{ entries?: HostRow[] }>(`/api/sysops/hosts/${row.id}`);
          setRows(data.entries || []);
          message.success("Host deleted");
        } catch (err) {
          message.error(err instanceof Error ? err.message : "Failed");
          throw err;
        }
      },
    });
  }

  return (
    <Card
      title="Hosts"
      extra={
        <Button type="primary" onClick={() => setDraft({ id: 0, ip: "", names: "" })} disabled={!ready || busy}>
          Add
        </Button>
      }
    >
      <Typography.Paragraph type="secondary">
        Address lines in /etc/hosts. A row added here records when it was created. Names written for local scans stay in their own block and are not listed.
      </Typography.Paragraph>
      <Table
        size="small"
        rowKey="id"
        loading={!ready}
        dataSource={rows}
        pagination={{ pageSize: 20, showSizeChanger: true, pageSizeOptions: ["10", "20", "50", "100"], showTotal: (n) => `Total ${n}` }}
        locale={{ emptyText: "No host entries" }}
        columns={[
          { title: "IP address", dataIndex: "ip", width: 180 },
          { title: "Hostnames", render: (_, row) => row.names.join(" ") },
          { title: "Created", dataIndex: "created", width: 200, render: (v: string) => hostWhen(v) },
          {
            title: "",
            width: 160,
            render: (_, row) => (
              <Space>
                <Button size="small" disabled={busy} onClick={() => setDraft({ id: row.id, ip: row.ip, names: row.names.join(" ") })}>
                  Edit
                </Button>
                <Button size="small" danger disabled={busy} onClick={() => remove(row)}>
                  Delete
                </Button>
              </Space>
            ),
          },
        ]}
      />
      <Modal
        title={draft && draft.id > 0 ? "Edit host" : "Add host"}
        open={!!draft}
        onCancel={() => !busy && setDraft(null)}
        onOk={() => void save()}
        confirmLoading={busy}
        okText="Save"
        destroyOnHidden
      >
        <Form layout="vertical">
          <Form.Item label="IP address" required>
            <Input value={draft?.ip || ""} placeholder="10.0.0.5" onChange={(e) => setDraft((d) => (d ? { ...d, ip: e.target.value } : d))} />
          </Form.Item>
          <Form.Item label="Hostnames" extra="Separate names with a space or comma" required>
            <Input value={draft?.names || ""} placeholder="db.internal cache.internal" onChange={(e) => setDraft((d) => (d ? { ...d, names: e.target.value } : d))} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
