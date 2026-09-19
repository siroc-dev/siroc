export type UserUsage = {
  user: string;
  processes: number;
  phpProcesses: number;
  memory: number;
  memPct: number;
  cpu: number;
  diskUsed: number;
};

export function formatBytes(n: number) {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 10 || i === 0 ? v.toFixed(0) : v.toFixed(1)} ${u[i]}`;
}

export function editorWorkspace(absPath: string, listingAbs?: string) {
  if (!absPath) return listingAbs || "";
  const pub = absPath.match(/^(.*\/(?:public_html|private_html))/);
  if (pub) return pub[1];
  const domain = absPath.match(/^(.*\/domains\/[^/]+)/);
  if (domain) return domain[1];
  const dir = absPath.replace(/\/[^/]+$/, "");
  return dir || "/";
}
