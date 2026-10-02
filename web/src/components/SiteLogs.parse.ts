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
  hint?: string;
};

export const DEFAULT_PAGE_SIZE = 20;

export function linesToEntries(content: string): SiteLogEntry[] {
  const rows = content
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .split("\n")
    .map((line) => line.trimEnd())
    .filter((line) => line.trim() && !line.startsWith("-- "))
    .map((line) => {
      const iso = line.match(/^(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s+(.*)$/);
      if (iso) return { time: iso[1], level: inferLineLevel(iso[2]), message: iso[2] };
      const syslog = line.match(/^([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+(\S+?)(?:\[\d+\])?:\s*(.*)$/);
      if (syslog) return { time: syslog[1], env: syslog[2], level: inferLineLevel(syslog[3] + " " + syslog[2]), message: syslog[3] };
      return { level: inferLineLevel(line), message: line };
    });
  return rows.reverse();
}

function inferLineLevel(s: string) {
  const low = s.toLowerCase();
  if (low.includes("error") || low.includes("fail") || low.includes("denied") || low.includes("fatal")) return "error";
  if (low.includes("warn")) return "warning";
  if (low.includes("notice")) return "notice";
  if (low.includes("debug")) return "debug";
  return "info";
}

export function logRows(entries?: SiteLogEntry[], content?: string) {
  if (entries && entries.length) return entries;
  if (content && content.trim()) return linesToEntries(content);
  return [];
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
