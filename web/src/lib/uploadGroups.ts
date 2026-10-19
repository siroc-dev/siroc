export type UploadGroup = { key: string; name: string; folder: boolean; files: File[] };

export function uploadGroups(files: File[]): UploadGroup[] {
  const groups: UploadGroup[] = [];
  const byFolder = new Map<string, UploadGroup>();
  files.forEach((file, index) => {
    const rel = (file.webkitRelativePath || "").replace(/^\/+/, "");
    const slash = rel.indexOf("/");
    if (slash > 0) {
      const name = rel.slice(0, slash);
      let group = byFolder.get(name);
      if (!group) {
        group = { key: `dir:${name}`, name, folder: true, files: [] };
        byFolder.set(name, group);
        groups.push(group);
      }
      group.files.push(file);
      return;
    }
    groups.push({ key: `file:${index}:${file.name}`, name: file.name, folder: false, files: [file] });
  });
  return groups;
}

// uploadSpeed is the average bytes per second since the upload started.
// The first fraction of a second is ignored so a single burst does not show a spike.
export function uploadSpeed(loaded: number, startedAt: number, now: number): number {
  const dt = (now - startedAt) / 1000;
  if (loaded <= 0 || dt < 0.2) return 0;
  return loaded / dt;
}

export function uploadPercent(doneBytes: number, totalBytes: number, doneCount: number, totalCount: number, settled: boolean): number {
  if (totalCount <= 0) return 0;
  const raw = totalBytes > 0 ? (doneBytes / totalBytes) * 100 : (doneCount / totalCount) * 100;
  const pct = Math.min(100, Math.round(raw));
  if (!settled && pct >= 100) return 99;
  return pct;
}
