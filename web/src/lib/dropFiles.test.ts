/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { filesFromDrop } from "./dropFiles";

function fileEntry(path: string, name: string): FileSystemEntry {
  return {
    isFile: true,
    isDirectory: false,
    name,
    fullPath: path,
    filesystem: {} as FileSystem,
    file: (ok: (file: File) => void) => {
      queueMicrotask(() => ok(new File(["x"], name)));
    },
  } as FileSystemFileEntry;
}

function onceTransfer(names: string[]): DataTransfer {
  const calls = new Map<string, number>();
  const items = names.map((name) => ({
    kind: "file" as const,
    webkitGetAsEntry: () => {
      const n = (calls.get(name) || 0) + 1;
      calls.set(name, n);
      if (n > 1) return null;
      return fileEntry("/" + name, name);
    },
  }));
  return {
    items,
    files: names.map((name) => new File(["x"], name)),
  } as unknown as DataTransfer;
}

describe("filesFromDrop", () => {
  it("keeps every dropped file when each entry can be read only once", async () => {
    const files = await filesFromDrop(onceTransfer(["a.txt", "b.txt", "c.txt"]));
    expect(files.map((f) => f.name)).toEqual(["a.txt", "b.txt", "c.txt"]);
    expect(files.map((f) => f.webkitRelativePath)).toEqual(["a.txt", "b.txt", "c.txt"]);
  });

  it("walks a dropped folder", async () => {
    const dir = {
      isFile: false,
      isDirectory: true,
      name: "site",
      fullPath: "/site",
      filesystem: {} as FileSystem,
      createReader: () => {
        let n = 0;
        return {
          readEntries: (ok: (entries: FileSystemEntry[]) => void) => {
            n++;
            ok(n === 1 ? [fileEntry("/site/index.php", "index.php"), fileEntry("/site/app.css", "app.css")] : []);
          },
        };
      },
    } as FileSystemDirectoryEntry;
    const dt = {
      items: [{ kind: "file", webkitGetAsEntry: () => dir }],
      files: [],
    } as unknown as DataTransfer;
    const files = await filesFromDrop(dt);
    expect(files.map((f) => f.webkitRelativePath)).toEqual(["site/index.php", "site/app.css"]);
  });
});
