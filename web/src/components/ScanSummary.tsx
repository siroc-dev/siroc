import type { ReactNode } from "react";
import { Alert, List, Space, Tag, Typography } from "antd";

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
        <List
          size="small"
          bordered
          dataSource={findings}
          renderItem={(f) => (
            <List.Item>
              <Space align="start" style={{ width: "100%" }}>
                <Tag color={sevColor[f.severity] || "default"}>{sevLabel[f.severity] || f.severity}</Tag>
                <div>
                  <div>
                    {f.title}
                    {f.count && f.count > 1 ? <Typography.Text type="secondary"> × {f.count}</Typography.Text> : null}
                  </div>
                  {f.detail ? (
                    <Typography.Paragraph type="secondary" style={{ margin: "4px 0 0", fontSize: 12 }}>
                      {f.detail}
                    </Typography.Paragraph>
                  ) : null}
                </div>
              </Space>
            </List.Item>
          )}
        />
      ) : null}
    </Space>
  );
}
