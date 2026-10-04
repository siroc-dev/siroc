import { useState } from "react";
import { Button, Descriptions, Modal, Popconfirm, Table, Typography } from "antd";

export type Fail2banBan = {
  ip: string;
  jail: string;
  asn?: string;
  asOrg?: string;
  country?: string;
  city?: string;
  location?: string;
};

export function Fail2banBans({
  bans,
  busy,
  onUnban,
}: {
  bans: Fail2banBan[];
  busy?: boolean;
  onUnban: (jail: string, ip: string) => void;
}) {
  const [open, setOpen] = useState<Fail2banBan | null>(null);
  return (
    <>
      <Table
        size="small"
        rowKey={(r) => r.jail + ":" + r.ip}
        dataSource={bans}
        pagination={{ pageSize: 20, showSizeChanger: true, pageSizeOptions: [10, 20, 50, 100] }}
        locale={{ emptyText: "No banned addresses." }}
        onRow={(row) => ({ onClick: () => setOpen(row), style: { cursor: "pointer" } })}
        columns={[
          { title: "IP", dataIndex: "ip" },
          { title: "Jail", dataIndex: "jail", width: 160 },
          {
            title: "Location",
            render: (_, row) => row.location || "—",
          },
          {
            title: "ASN",
            width: 120,
            render: (_, row) => row.asn || "—",
          },
          {
            title: "",
            width: 100,
            align: "right" as const,
            render: (_, row) => (
              <Popconfirm
                title={`Unban ${row.ip}?`}
                onConfirm={(e) => {
                  e?.stopPropagation();
                  onUnban(row.jail, row.ip);
                }}
                onCancel={(e) => e?.stopPropagation()}
              >
                <Button size="small" danger onClick={(e) => e.stopPropagation()} loading={busy}>
                  Delete
                </Button>
              </Popconfirm>
            ),
          },
        ]}
      />
      <Modal
        title={open ? open.ip : "Banned address"}
        open={!!open}
        onCancel={() => setOpen(null)}
        footer={
          open ? (
            <Popconfirm
              title={`Unban ${open.ip}?`}
              onConfirm={() => {
                onUnban(open.jail, open.ip);
                setOpen(null);
              }}
            >
              <Button danger>Delete</Button>
            </Popconfirm>
          ) : null
        }
        destroyOnHidden
      >
        {open ? (
          <Descriptions column={1} size="small" style={{ marginTop: 8 }}>
            <Descriptions.Item label="IP">
              <Typography.Text copyable>{open.ip}</Typography.Text>
            </Descriptions.Item>
            <Descriptions.Item label="Jail">{open.jail}</Descriptions.Item>
            <Descriptions.Item label="ASN">{open.asn || "—"}</Descriptions.Item>
            <Descriptions.Item label="Organization">{open.asOrg || "—"}</Descriptions.Item>
            <Descriptions.Item label="Location">{open.location || "—"}</Descriptions.Item>
            <Descriptions.Item label="City">{open.city || "—"}</Descriptions.Item>
            <Descriptions.Item label="Country">{open.country || "—"}</Descriptions.Item>
          </Descriptions>
        ) : null}
      </Modal>
    </>
  );
}
