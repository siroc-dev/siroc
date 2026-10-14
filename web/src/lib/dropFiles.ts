// webkitGetAsEntry() can be called once per item, and only while the drop
// handler is still running. Snapshot every entry before the first await.

export async function filesFromDrop(dt: DataTransfer): Promise<File[]> {
  const plain = [...dt.files];
  const entries: FileSystemEntry[] = [];
  for (const item of [...dt.items]) {
    if (item.kind !== "file") continue;
    const entry = item.webkitGetAsEntry?.();
    if (entry) entries.push(entry);
  }
  if (!entries.length) return plain;
  const out: File[] = [];
  for (const entry of entries) await walkEntry(entry, out);
  return out;
}

async function walkEntry(entry: FileSystemEntry, out: File[]) {
  if (entry.isFile) {
    const file = await new Promise<File>((resolve, reject) => (entry as FileSystemFileEntry).file(resolve, reject));
    const rel = entry.fullPath.replace(/^\/+/, "");
    if (rel && !file.webkitRelativePath) {
      try {
        Object.defineProperty(file, "webkitRelativePath", { value: rel });
      } catch {
        (file as File & { webkitRelativePath?: string }).webkitRelativePath = rel;
      }
    }
    out.push(file);
    return;
  }
  if (!entry.isDirectory) return;
  const reader = (entry as FileSystemDirectoryEntry).createReader();
  const readBatch = () => new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject));
  let batch = await readBatch();
  while (batch.length) {
    for (const child of batch) await walkEntry(child, out);
    batch = await readBatch();
  }
}
