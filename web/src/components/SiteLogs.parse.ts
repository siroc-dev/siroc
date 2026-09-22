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

export const DEFAULT_PAGE_SIZE = 10;

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
