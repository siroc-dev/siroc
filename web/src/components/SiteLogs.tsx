import { useEffect, useMemo, useState } from "react";
import { Alert, Button, Input, Pagination, Space, Tag, Typography } from "antd";
import { api } from "@/lib/api";
import { asList } from "@/lib/lists";

export type SiteLogFile = {
  id: string;
  label: string;
  group: string;
  kind: string;
  exists: boolean;
  size?: number;
  modTime?: string;
};

export type SiteLogEntry = {
  time?: string;
  env?: string;
  level: string;
  message: string;
  context?: string;
  status?: number;
};

export type SiteLogsData = {
  ok: boolean;
  domain: string;
  files: SiteLogFile[];
  current?: SiteLogFile;
  entries?: SiteLogEntry[];
  counts?: Record<string, number>;
  content?: string;
  truncated?: boolean;
};

const FILE_GROUPS = [
  { key: "laravel", title: "Laravel" },
  { key: "nginx", title: "Nginx" },
  { key: "apache", title: "Apache" },
  { key: "waf", title: "ModSecurity WAF" },
];

export const DEFAULT_PAGE_SIZE = 10;

const LEVEL_ORDER = ["emergency", "alert", "critical", "error", "warning", "notice", "info", "debug", "access"];

const LEVEL_COLOR: Record<string, string> = {
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

export function fmtSize(n?: number) {
  if (!n) return "empty";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function pageEntries<T>(rows: T[], page: number, pageSize = DEFAULT_PAGE_SIZE): T[] {
  const size = pageSize > 0 ? pageSize : DEFAULT_PAGE_SIZE;
  const p = page > 0 ? page : 1;
  const start = (p - 1) * size;
  return rows.slice(start, start + size);
}

export function filterEntries(entries: SiteLogEntry[], query: string, levels: string[]) {
  const q = query.trim().toLowerCase();
  return entries.filter((e) => {
    if (levels.length && !levels.includes(e.level)) return false;
    if (!q) return true;
    return (
      e.message.toLowerCase().includes(q) ||
      (e.context || "").toLowerCase().includes(q) ||
      (e.env || "").toLowerCase().includes(q) ||
      (e.time || "").toLowerCase().includes(q)
    );
  });
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
  const [data, setData] = useState<SiteLogsData | null>(null);
  const [id, setId] = useState("");
  const [query, setQuery] = useState("");
  const [levels, setLevels] = useState<string[]>([]);
  const [raw, setRaw] = useState(false);
  const [open, setOpen] = useState<number | null>(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function load(next = id) {
    setBusy(true);
    setErr("");
    setOpen(null);
    setPage(1);
    try {
      const q = next ? `?id=${encodeURIComponent(next)}` : "";
      const out = await api.get<SiteLogsData>(`/api/sites/${siteId}/logs${q}`);
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
  const paged = useMemo(() => pageEntries(visible, page, pageSize), [visible, page, pageSize]);

  useEffect(() => {
    setPage(1);
    setOpen(null);
  }, [query, levels]);
  const counts = data?.counts || {};
  const presentLevels = LEVEL_ORDER.filter((l) => counts[l]);

  function toggleLevel(l: string) {
    setLevels((cur) => (cur.includes(l) ? cur.filter((x) => x !== l) : [...cur, l]));
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
    <div style={{ display: "flex", minHeight: "62vh", border: "1px solid #1e293b", borderRadius: 10, overflow: "hidden", background: "#0b1220", color: "#e2e8f0" }}>
      <aside style={{ width: 240, flexShrink: 0, borderRight: "1px solid #1e293b", background: "#0f172a", overflow: "auto" }}>
        <div style={{ padding: "12px 14px 8px", fontSize: 12, color: "#94a3b8" }}>Files · {domain}</div>
        {FILE_GROUPS.map((g) => {
          const rows = files.filter((f) => f.group === g.key);
          if (!rows.length) return null;
          return (
            <div key={g.key} style={{ marginBottom: 10 }}>
              <div style={{ padding: "6px 14px", fontSize: 11, letterSpacing: 0.4, textTransform: "uppercase", color: "#64748b" }}>{g.title}</div>
              {rows.map((f) => {
                const active = f.id === id;
                return (
                  <button
                    key={f.id}
                    type="button"
                    onClick={() => {
                      setId(f.id);
                      void load(f.id);
                    }}
                    style={{
                      display: "block",
                      width: "100%",
                      textAlign: "left",
                      border: 0,
                      background: active ? "#1d4ed8" : "transparent",
                      color: f.exists ? "#e2e8f0" : "#64748b",
                      padding: "7px 14px",
                      cursor: "pointer",
                      fontSize: 13,
                    }}
                  >
                    <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{fileName(f)}</div>
                    <div style={{ fontSize: 11, opacity: 0.75 }}>{f.exists ? fmtSize(f.size) : "none"}</div>
                  </button>
                );
              })}
            </div>
          );
        })}
      </aside>
      <section style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
        <div style={{ padding: 12, borderBottom: "1px solid #1e293b", display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
          <Input
            allowClear
            placeholder="Search logs"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            style={{ width: 260 }}
          />
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
              {raw ? "Entries" : "Raw"}
            </Button>
            <Button size="small" disabled={!data?.content} onClick={download}>
              Download
            </Button>
            <Button size="small" loading={busy} onClick={() => void load(id)}>
              Refresh
            </Button>
          </Space>
        </div>
        {data?.truncated ? (
          <Typography.Text style={{ padding: "6px 12px", color: "#94a3b8", fontSize: 12 }}>Showing the last 256 KB of this file.</Typography.Text>
        ) : null}
        {err ? <Alert type="error" showIcon message={err} style={{ margin: 12 }} /> : null}
        <div style={{ flex: 1, overflow: "auto" }}>
          {busy && !data ? (
            <div style={{ padding: 16, color: "#94a3b8" }}>Loading…</div>
          ) : raw ? (
            <pre style={{ margin: 0, padding: 14, fontSize: 12, lineHeight: 1.45, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
              {data?.content || (data?.current?.exists ? "File is empty." : "No log yet.")}
            </pre>
          ) : !visible.length ? (
            <div style={{ padding: 16, color: "#94a3b8" }}>
              {data?.current?.exists ? "No matching entries." : "No log yet. Traffic or Laravel errors will appear here."}
            </div>
          ) : (
            paged.map((e, i) => {
              const expanded = open === i;
              return (
                <button
                  key={`${e.time}-${i}`}
                  type="button"
                  onClick={() => setOpen(expanded ? null : i)}
                  style={{
                    display: "block",
                    width: "100%",
                    textAlign: "left",
                    border: 0,
                    borderBottom: "1px solid #1e293b",
                    background: expanded ? "#111827" : "transparent",
                    color: "inherit",
                    padding: "10px 14px",
                    cursor: "pointer",
                  }}
                >
                  <div style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 4 }}>
                    <Tag color={LEVEL_COLOR[e.level] || "default"} style={{ marginInlineEnd: 0 }}>
                      {e.level.toUpperCase()}
                    </Tag>
                    {e.env ? <span style={{ color: "#94a3b8", fontSize: 12 }}>{e.env}</span> : null}
                    {e.time ? <span style={{ color: "#64748b", fontSize: 12 }}>{e.time}</span> : null}
                    {e.status ? <span style={{ color: "#94a3b8", fontSize: 12 }}>{e.status}</span> : null}
                  </div>
                  <div style={{ fontSize: 13, lineHeight: 1.45, wordBreak: "break-word" }}>{e.message}</div>
                  {expanded && e.context ? (
                    <pre style={{ margin: "10px 0 0", padding: 10, background: "#020617", borderRadius: 6, fontSize: 11, lineHeight: 1.4, whiteSpace: "pre-wrap", wordBreak: "break-word", color: "#cbd5e1" }}>
                      {e.context}
                    </pre>
                  ) : e.context && !expanded ? (
                    <div style={{ marginTop: 4, fontSize: 11, color: "#64748b" }}>Stack / context — click to expand</div>
                  ) : null}
                </button>
              );
            })
          )}
        </div>
        {!raw && visible.length > 0 ? (
          <div style={{ padding: "8px 12px", borderTop: "1px solid #1e293b", background: "#0f172a" }}>
            <Pagination
              size="small"
              current={page}
              pageSize={pageSize}
              total={visible.length}
              showSizeChanger
              pageSizeOptions={[10, 20, 50, 100]}
              showTotal={(t, range) => `${range[0]}-${range[1]} of ${t}`}
              onChange={(p, ps) => {
                setPage(p);
                setPageSize(ps);
                setOpen(null);
              }}
            />
          </div>
        ) : null}
      </section>
    </div>
  );
}
