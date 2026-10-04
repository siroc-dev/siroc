import { useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Input, Popconfirm, Space, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { asList } from "@/lib/lists";
import { LogTable, LEVEL_COLOR } from "./LogTable";
import { filterEntries, type SiteLogFile, type SiteLogsData } from "./SiteLogs.parse";

export type { SiteLogEntry, SiteLogFile, SiteLogsData } from "./SiteLogs.parse";
export { DEFAULT_PAGE_SIZE, filterEntries, pageEntries } from "./SiteLogs.parse";

const FILE_GROUPS = [
  { key: "laravel", title: "Laravel" },
  { key: "nginx", title: "Nginx" },
  { key: "apache", title: "Apache" },
  { key: "waf", title: "ModSecurity WAF" },
];

const LEVEL_ORDER = ["emergency", "alert", "critical", "error", "warning", "notice", "info", "debug", "access"];

export function fmtSize(n?: number) {
  if (!n) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function fileName(f: SiteLogFile) {
  if (f.group === "waf") return "audit.log";
  if (f.group === "laravel") {
    const cut = f.label.split("·").pop()?.trim();
    return cut || f.label;
  }
  return f.kind === "error" ? "error.log" : "access.log";
}

export function SiteLogs({ siteId, domain }: { siteId: number; domain: string }) {
  const { message } = App.useApp();
  const [data, setData] = useState<SiteLogsData | null>(null);
  const [id, setId] = useState("");
  const [query, setQuery] = useState("");
  const [levels, setLevels] = useState<string[]>([]);
  const [raw, setRaw] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function load(next = id, probe = false) {
    setBusy(true);
    setErr("");
    try {
      const q = new URLSearchParams();
      if (next) q.set("id", next);
      if (probe) q.set("probe", "1");
      const qs = q.toString() ? `?${q.toString()}` : "";
      const out = await api.get<SiteLogsData>(`/api/sites/${siteId}/logs${qs}`);
      setData(out);
      if (out.current?.id) setId(out.current.id);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to load logs");
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void load("");
  }, [siteId]);

  const files = asList(data?.files);
  const entries = asList(data?.entries);
  const visible = useMemo(() => filterEntries(entries, query, levels), [entries, query, levels]);
  const counts = data?.counts || {};
  const presentLevels = LEVEL_ORDER.filter((l) => counts[l]);

  function toggleLevel(l: string) {
    setLevels((cur) => (cur.includes(l) ? cur.filter((x) => x !== l) : [...cur, l]));
  }

  async function clearLogs(all: boolean) {
    setBusy(true);
    setErr("");
    try {
      const out = await api.post<SiteLogsData>(`/api/sites/${siteId}/logs/clear`, { id: all ? "" : id });
      setData(out);
      if (out.current?.id) setId(out.current.id);
      message.success(all ? "Cleared logs for this website" : "Cleared this log");
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to clear logs");
    } finally {
      setBusy(false);
    }
  }

  function download() {
    const name = data?.current ? fileName(data.current) : "site.log";
    const blob = new Blob([data?.content || ""], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${domain}-${name}`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="log-shell">
      <div className="log-list">
        <div style={{ padding: "4px 2px 8px", fontSize: 12, color: "inherit", opacity: 0.65 }}>Files · {domain}</div>
        {FILE_GROUPS.map((g) => {
          const rows = files.filter((f) => f.group === g.key);
          if (!rows.length) return null;
          return (
            <div key={g.key} style={{ marginBottom: 10 }}>
              <div style={{ padding: "4px 2px", fontSize: 11, letterSpacing: 0.4, textTransform: "uppercase", opacity: 0.55 }}>
                {g.title}
              </div>
              {rows.map((f) => (
                <button
                  key={f.id}
                  type="button"
                  className={f.id === id ? "is-on" : ""}
                  onClick={() => {
                    setId(f.id);
                    void load(f.id);
                  }}
                >
                  <span>{fileName(f)}</span>
                  <span style={{ fontSize: 11, opacity: 0.7 }}>{f.exists ? (f.size ? fmtSize(f.size) : "empty") : "none"}</span>
                </button>
              ))}
            </div>
          );
        })}
      </div>
      <div>
        <div style={{ marginBottom: 12, display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
          <Input allowClear placeholder="Search logs" value={query} onChange={(e) => setQuery(e.target.value)} style={{ width: 260 }} />
          {presentLevels.map((l) => (
            <Tag
              key={l}
              color={levels.includes(l) || !levels.length ? LEVEL_COLOR[l] : undefined}
              onClick={() => toggleLevel(l)}
              style={{ cursor: "pointer", marginInlineEnd: 0, opacity: !levels.length || levels.includes(l) ? 1 : 0.45 }}
            >
              {l} {counts[l]}
            </Tag>
          ))}
          <Space size={6} style={{ marginLeft: "auto" }}>
            <Button size="small" onClick={() => setRaw((v) => !v)}>
              {raw ? "Table" : "Raw"}
            </Button>
            <Button size="small" disabled={!data?.content} onClick={download}>
              Download
            </Button>
            <Button size="small" loading={busy} onClick={() => void load(id, true)}>
              Test request
            </Button>
            <Button size="small" loading={busy} onClick={() => void load(id)}>
              Refresh
            </Button>
            <Popconfirm title="Empty this log file?" description="The file stays in place so the web server can keep writing." onConfirm={() => void clearLogs(false)}>
              <Button size="small" danger disabled={!id} loading={busy}>
                Clear
              </Button>
            </Popconfirm>
            <Popconfirm title="Empty every log for this website?" description="Rotated copies are deleted. Live files are emptied." onConfirm={() => void clearLogs(true)}>
              <Button size="small" danger loading={busy}>
                Clear all
              </Button>
            </Popconfirm>
          </Space>
        </div>
        {data?.truncated ? (
          <Typography.Text type="secondary" style={{ display: "block", marginBottom: 8 }}>
            Showing the last 256 KB, newest first.
          </Typography.Text>
        ) : null}
        {err ? <Alert type="error" showIcon message={err} style={{ marginBottom: 12 }} /> : null}
        {data?.hint ? <Alert type="info" showIcon message={data.hint} style={{ marginBottom: 12 }} /> : null}
        {raw ? (
          <pre className="log-view">
            {data?.content || (data?.current?.exists ? "File is empty — no traffic has hit this vhost yet." : "No log yet.")}
          </pre>
        ) : (
          <LogTable
            rows={visible}
            loading={busy && !data}
            emptyText={
              data?.current?.exists
                ? data.current.size
                  ? "No matching entries."
                  : "No traffic yet. Visit the site or click Test request."
                : "No log yet. Traffic or Laravel errors will appear here."
            }
          />
        )}
      </div>
    </div>
  );
}
