export type InstallJob = {
  id: number;
  name: string;
  version: string;
  status: "queued" | "running" | "done" | "error" | "canceled";
  message: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
};

export type InstallQueue = {
  jobs: InstallJob[];
  active: number;
  current: InstallJob | null;
  queue: InstallJob[];
};

export function jobLabel(job: Pick<InstallJob, "name" | "version">) {
  if (job.name === "php-ext") {
    const [ver, ext, src] = (job.version || "").split(":");
    const bits = [ver && `PHP ${ver}`, ext, src && src !== "auto" ? src : ""].filter(Boolean);
    return bits.length ? bits.join(" ") : "PHP extension";
  }
  if (job.version === "upgrade") {
    return `Update ${job.name}`;
  }
  if ((job.version || "").startsWith("upgrade:")) {
    return `Update ${job.name} ${job.version.slice(8)}`.trim();
  }
  return [job.name, job.version].filter(Boolean).join(" ");
}

export function elapsed(startedAt?: string) {
  if (!startedAt) return "";
  const ms = Date.now() - Date.parse(startedAt);
  if (!Number.isFinite(ms) || ms < 0) return "";
  const s = Math.floor(ms / 1000);
  const m = Math.floor(s / 60);
  if (m >= 60) return `${Math.floor(m / 60)}h ${m % 60}m`;
  if (m > 0) return `${m}m ${s % 60}s`;
  return `${s}s`;
}
