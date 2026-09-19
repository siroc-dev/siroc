import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { App, Button, Card, Collapse, Select, Space, Table, Tag, Typography } from "antd";
import { ScanSummary, toolLabel, type ScanResult } from "@/components/ScanSummary";
import { api } from "@/lib/api";

export function ScanLogs() {
  const { message } = App.useApp();
  const nav = useNavigate();
  const [params, setParams] = useSearchParams();
  const [logs, setLogs] = useState<ScanResult[]>([]);
  const [detail, setDetail] = useState<ScanResult | null>(null);
  const [tool, setTool] = useState<string>("");

  async function load() {
    const list = await api.get<ScanResult[]>("/api/security/logs");
    setLogs(list || []);
    const id = params.get("id");
    if (id) {
      const found = (list || []).find((l) => l.id === id);
      if (found) setDetail(found);
      else {
        try {
          setDetail(await api.get<ScanResult>(`/api/security/logs/${encodeURIComponent(id)}`));
        } catch {
          setDetail(null);
        }
      }
    }
  }

  useEffect(() => {
    load().catch((e) => message.error(e.message));
  }, []);

  const rows = useMemo(() => (tool ? logs.filter((l) => l.tool === tool) : logs), [logs, tool]);

  return (
    <div className="cp-page">
      <div>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Scan logs
        </Typography.Title>
        <Typography.Paragraph type="secondary">
          History of Nikto, OWASP ZAP, and OpenVAS runs. New scans from Security are saved here automatically.
        </Typography.Paragraph>
      </div>
      <Card
        title="Past scans"
        extra={
          <Space>
            <Select
              allowClear
              placeholder="All tools"
              style={{ width: 200 }}
              value={tool || undefined}
              onChange={(v) => setTool(v || "")}
              options={[
                { value: "nikto", label: "Nikto" },
                { value: "zap", label: "OWASP ZAP" },
                { value: "openvas", label: "OpenVAS / Greenbone" },
              ]}
            />
            <Button onClick={() => nav("/security")}>Run a scan</Button>
          </Space>
        }
      >
        <Table
          size="small"
          rowKey="id"
          dataSource={rows}
          pagination={{ pageSize: 20 }}
          onRow={(row) => ({
            onClick: () => {
              setDetail(row);
              setParams({ id: row.id });
            },
            style: { cursor: "pointer" },
          })}
          columns={[
            {
              title: "When",
              dataIndex: "createdAt",
              width: 190,
              render: (v: string, r) => (v ? new Date(v).toLocaleString() : r.id),
            },
            { title: "Tool", dataIndex: "tool", width: 170, render: (v: string) => toolLabel(v) },
            { title: "Target", dataIndex: "target", ellipsis: true },
            {
              title: "Result",
              width: 260,
              render: (_, r) => (
                <Space wrap size={4}>
                  <Tag color={r.ok ? (r.high ? "warning" : "success") : "error"}>{r.ok ? "Done" : "Failed"}</Tag>
                  <Tag color="red">{r.high || 0} high</Tag>
                  <Tag color="orange">{r.medium || 0} med</Tag>
                  <Tag>{(r.low || 0) + (r.info || 0)} other</Tag>
                </Space>
              ),
            },
          ]}
        />
      </Card>
      {detail ? (
        <Card title={detail.title || toolLabel(detail.tool)} extra={<Typography.Text type="secondary">{detail.id}</Typography.Text>}>
          <ScanSummary
            scan={detail}
            extra={
              detail.report ? (
                <Typography.Link href={detail.report} target="_blank" rel="noreferrer">
                  Open original report
                </Typography.Link>
              ) : null
            }
          />
          {detail.output ? (
            <Collapse
              style={{ marginTop: 16 }}
              items={[
                {
                  key: "raw",
                  label: "Technical log",
                  children: (
                    <pre style={{ margin: 0, whiteSpace: "pre-wrap", maxHeight: 360, overflow: "auto" }}>{detail.output}</pre>
                  ),
                },
              ]}
            />
          ) : null}
        </Card>
      ) : null}
    </div>
  );
}
