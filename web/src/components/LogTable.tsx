import { Table, Tag, Typography } from "antd";
import { DEFAULT_PAGE_SIZE, type SiteLogEntry } from "./SiteLogs.parse";

export const LEVEL_COLOR: Record<string, string> = {
  emergency: "magenta",
  alert: "magenta",
  critical: "red",
  error: "red",
  warning: "orange",
  notice: "gold",
  info: "blue",
  debug: "default",
  access: "cyan",
};

export function LogTable({
  rows,
  loading,
  emptyText,
}: {
  rows: SiteLogEntry[];
  loading?: boolean;
  emptyText?: string;
}) {
  return (
    <Table<SiteLogEntry & { key: string }>
      size="small"
      rowKey="key"
      dataSource={rows.map((r, i) => ({ ...r, key: String(i) }))}
      loading={loading}
      locale={{ emptyText: emptyText || "No log entries." }}
      pagination={{
        defaultPageSize: DEFAULT_PAGE_SIZE,
        showSizeChanger: true,
        pageSizeOptions: [10, 20, 50, 100],
        showTotal: (t, range) => `${range[0]}-${range[1]} of ${t}`,
      }}
      expandable={{
        rowExpandable: (r) => !!r.context,
        expandedRowRender: (r) => (
          <pre style={{ margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-word", fontSize: 12 }}>{r.context}</pre>
        ),
      }}
      columns={[
        {
          title: "Time",
          dataIndex: "time",
          width: 200,
          render: (v?: string) => v || "—",
        },
        {
          title: "Level",
          dataIndex: "level",
          width: 110,
          render: (v: string) => <Tag color={LEVEL_COLOR[v] || "default"}>{(v || "info").toUpperCase()}</Tag>,
        },
        {
          title: "Message",
          dataIndex: "message",
          render: (v: string, r) => (
            <div style={{ wordBreak: "break-word", whiteSpace: "pre-wrap" }}>
              {v}
              {r.env ? (
                <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
                  {r.env}
                </Typography.Text>
              ) : null}
              {r.status ? (
                <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
                  {r.status}
                </Typography.Text>
              ) : null}
            </div>
          ),
        },
      ]}
    />
  );
}
