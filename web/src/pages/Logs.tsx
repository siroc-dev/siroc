import { useEffect, useMemo, useState } from "react";
import { Link, useOutletContext, useSearchParams } from "react-router-dom";
import { Alert, App, Button, Card, Input, Select, Space, Tabs, Tag, Typography } from "antd";
import { ReloadOutlined } from "@ant-design/icons";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/usage";
import { LogTable } from "@/components/LogTable";
import { SiteLogs } from "@/components/SiteLogs";
import { ScanLogs } from "@/pages/ScanLogs";
import { filterEntries, logRows, type SiteLogEntry } from "@/components/SiteLogs.parse";

type LogFile = {
  id: string;
  group: string;
  title: string;
  path?: string;
  kind?: string;
  exists: boolean;
  size?: number;
  modTime?: string;
};
type LogResp = {
  files?: LogFile[];
  current?: LogFile;
  entries?: SiteLogEntry[];
  content?: string;
  truncated?: boolean;
  message?: string;
};
type SiteOpt = { id: number; domain: string; displayName?: string };

const TABS = [
  { key: "panel", label: "Panel" },
  { key: "website", label: "Website" },
  { key: "server", label: "Server" },
  { key: "ssh", label: "SSH" },
  { key: "cron", label: "Cron / syslog" },
  { key: "scan", label: "Scan" },
];

export function Logs() {
  const { message } = App.useApp();
  const { admin } = useOutletContext<{ admin?: boolean }>();
  const [search, setSearch] = useSearchParams();
  const want = search.get("tab") || (admin ? "panel" : "website");
  const tab = admin ? want : "website";
  const [files, setFiles] = useState<LogFile[]>([]);
  const [cur, setCur] = useState<LogFile | null>(null);
  const [entries, setEntries] = useState<SiteLogEntry[]>([]);
  const [content, setContent] = useState("");
  const [trunc, setTrunc] = useState(false);
  const [hint, setHint] = useState("");
  const [busy, setBusy] = useState(false);
  const [q, setQ] = useState("");
  const [raw, setRaw] = useState(false);
  const [sites, setSites] = useState<SiteOpt[]>([]);
  const [siteId, setSiteId] = useState(0);

  const shown = useMemo(() => files.filter((f) => f.group === tab), [files, tab]);

  async function load(id?: string) {
    if (!admin || tab === "website" || tab === "scan") return;
    setBusy(true);
    try {
      const qs = new URLSearchParams();
      if (id) qs.set("id", id);
      else qs.set("group", tab);
      const out = await api.get<LogResp>(`/api/system-logs?${qs.toString()}`);
      setFiles(out.files || []);
      setCur(out.current || null);
      setEntries(logRows(out.entries, out.content));
      setContent(out.content || "");
      setTrunc(!!out.truncated);
      setHint(out.message || "");
    } catch (e) {
      message.error(e instanceof Error ? e.message : "Failed to load logs");
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    if (tab === "website") {
      api
        .get<SiteOpt[]>("/api/sites")
        .then((rows) => {
          setSites(rows || []);
          setSiteId((cur) => cur || rows?.[0]?.id || 0);
        })
        .catch(() => setSites([]));
      return;
    }
    if (admin && tab !== "scan") void load();
  }, [tab, admin]);

  const filtered = useMemo(() => filterEntries(entries, q, []), [entries, q]);
  const rawFiltered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return content;
    return content
      .split("\n")
      .filter((line) => line.toLowerCase().includes(needle))
      .join("\n");
  }, [content, q]);

  const items = (admin ? TABS : TABS.filter((t) => t.key === "website")).map((t) => ({
    key: t.key,
    label: <Link to={`/logs?tab=${t.key}`}>{t.label}</Link>,
  }));

  const site = sites.find((s) => s.id === siteId);

  return (
    <div className="cp-page">
      <div>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Logs
        </Typography.Title>
        <Typography.Paragraph type="secondary">
          {admin ? "Panel, website, server, SSH, cron, and scan history." : "Access and error logs for your websites."}
        </Typography.Paragraph>
      </div>
      <Tabs
        activeKey={tab}
        items={items}
        onChange={(key) => {
          const next = new URLSearchParams(search);
          next.set("tab", key);
          setSearch(next, { replace: true });
          setQ("");
          setRaw(false);
        }}
      />
      {tab === "scan" ? (
        <ScanLogs embedded />
      ) : tab === "website" ? (
        <Card
          title="Website logs"
          extra={
            <Select
              style={{ minWidth: 260 }}
              placeholder="Select a website"
              value={siteId || undefined}
              options={sites.map((s) => ({ value: s.id, label: s.displayName || s.domain }))}
              onChange={setSiteId}
            />
          }
        >
          {site ? (
            <SiteLogs siteId={site.id} domain={site.displayName || site.domain} />
          ) : (
            <Typography.Text type="secondary">Create a website first.</Typography.Text>
          )}
        </Card>
      ) : (
        <div className="log-shell">
          <div className="log-list">
            {shown.map((f) => (
              <button
                key={f.id}
                type="button"
                className={cur?.id === f.id ? "is-on" : ""}
                onClick={() => void load(f.id)}
              >
                <span>{f.title}</span>
                <Tag color={f.exists ? "success" : "default"}>{f.exists ? "Ready" : "Missing"}</Tag>
              </button>
            ))}
            {!shown.length ? <Typography.Text type="secondary">No logs in this group.</Typography.Text> : null}
          </div>
          <Card
            className="log-view-card"
            title={cur?.title || "Log"}
            extra={
              <Space wrap>
                {cur?.size ? <Typography.Text type="secondary">{formatBytes(cur.size)}</Typography.Text> : null}
                <Input
                  allowClear
                  size="small"
                  placeholder="Filter lines"
                  style={{ width: 180 }}
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                />
                <Button size="small" onClick={() => setRaw((v) => !v)}>
                  {raw ? "Table" : "Raw"}
                </Button>
                <Button size="small" icon={<ReloadOutlined />} loading={busy} onClick={() => void load(cur?.id)}>
                  Refresh
                </Button>
              </Space>
            }
          >
            {hint ? <Alert type="info" showIcon message={hint} style={{ marginBottom: 12 }} /> : null}
            {trunc ? (
              <Typography.Text type="secondary" style={{ display: "block", marginBottom: 8 }}>
                Showing the last 256 KB, newest first.
              </Typography.Text>
            ) : null}
            {raw ? (
              <pre className="log-view">{rawFiltered || (busy ? "Loading…" : "Empty")}</pre>
            ) : (
              <LogTable rows={filtered} loading={busy && !entries.length} emptyText={busy ? "Loading…" : "Empty"} />
            )}
            {cur?.path ? (
              <Typography.Text type="secondary" style={{ display: "block", marginTop: 8 }}>
                {cur.path}
              </Typography.Text>
            ) : null}
          </Card>
        </div>
      )}
    </div>
  );
}
