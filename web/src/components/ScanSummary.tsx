import type { ReactNode } from "react";
import { Alert, Space, Table, Tag, Typography } from "antd";

export type ScanFinding = { severity: string; title: string; count?: number; detail?: string };
export type ScanResult = {
  ok: boolean;
  id: string;
  tool: string;
  target: string;
  report?: string;
  output?: string;
  title?: string;
  summary?: string;
  findings?: ScanFinding[];
  high?: number;
  medium?: number;
  low?: number;
  info?: number;
  createdAt?: string;
};

const sevColor: Record<string, string> = { high: "red", medium: "orange", low: "gold", info: "blue" };
const sevLabel: Record<string, string> = { high: "High", medium: "Medium", low: "Low", info: "Info" };

export function toolLabel(id: string) {
  if (id === "nikto") return "Nikto";
  if (id === "zap") return "OWASP ZAP";
  if (id === "wpscan") return "WPScan";
  if (id === "amwscan") return "PHP Antimalware Scanner";
  if (id === "openvas") return "OpenVAS / Greenbone";
  return id;
}

export function ScanSummary({ scan, extra }: { scan: ScanResult; extra?: ReactNode }) {
  const findings = scan.findings || [];
  return (
    <Space direction="vertical" style={{ width: "100%" }} size="middle">
      <Alert
        type={scan.ok ? (scan.high ? "warning" : "success") : "error"}
        message={scan.title || `${toolLabel(scan.tool)} · ${scan.target}`}
        description={
          <Space direction="vertical" size={8} style={{ width: "100%" }}>
            <Typography.Paragraph style={{ margin: 0 }}>{scan.summary || "Scan finished."}</Typography.Paragraph>
            <Space wrap>
              {scan.target ? <Typography.Text type="secondary">{scan.target}</Typography.Text> : null}
              <Tag color="red">{scan.high || 0} high</Tag>
              <Tag color="orange">{scan.medium || 0} medium</Tag>
              <Tag color="gold">{scan.low || 0} low</Tag>
              <Tag color="blue">{scan.info || 0} info</Tag>
            </Space>
            {extra}
          </Space>
        }
      />
      {findings.length ? (
        <Table
          size="small"
          rowKey={(_, i) => String(i)}
          pagination={findings.length > 12 ? { pageSize: 12 } : false}
          dataSource={findings}
          columns={[
            {
              title: "Severity",
              width: 110,
              render: (_, f) => <Tag color={sevColor[f.severity] || "default"}>{sevLabel[f.severity] || f.severity}</Tag>,
            },
            { title: "Finding", dataIndex: "title" },
            {
              title: "Count",
              width: 80,
              align: "right" as const,
              render: (_, f) => (f.count && f.count > 1 ? f.count : "—"),
            },
            { title: "Detail", dataIndex: "detail", ellipsis: true, render: (v: string) => v || "—" },
          ]}
        />
      ) : null}
    </Space>
  );
}
