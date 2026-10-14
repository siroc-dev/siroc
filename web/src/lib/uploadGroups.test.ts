/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { uploadGroups, uploadPercent } from "./uploadGroups";

function file(name: string, rel = "", body = "x") {
  const f = new File([body], name);
  if (rel) Object.defineProperty(f, "webkitRelativePath", { value: rel });
  return f;
}

describe("uploadGroups", () => {
  it("keeps each loose file on its own row", () => {
    const groups = uploadGroups([file("a.txt"), file("b.txt"), file("a.txt")]);
    expect(groups.map((g) => ({ name: g.name, folder: g.folder, n: g.files.length }))).toEqual([
      { name: "a.txt", folder: false, n: 1 },
      { name: "b.txt", folder: false, n: 1 },
      { name: "a.txt", folder: false, n: 1 },
    ]);
  });

  it("rolls a folder into one row and leaves sibling files alone", () => {
    const groups = uploadGroups([
      file("readme.txt"),
      file("index.php", "photos/index.php"),
      file("a.jpg", "photos/nested/a.jpg", "aaaa"),
      file("notes.txt", "docs/notes.txt"),
    ]);
    expect(groups.map((g) => ({ name: g.name, folder: g.folder, n: g.files.length }))).toEqual([
      { name: "readme.txt", folder: false, n: 1 },
      { name: "photos", folder: true, n: 2 },
      { name: "docs", folder: true, n: 1 },
    ]);
  });
});

describe("uploadPercent", () => {
  it("uses bytes for a folder and holds 100 until the request finishes", () => {
    expect(uploadPercent(50, 200, 1, 4, false)).toBe(25);
    expect(uploadPercent(200, 200, 3, 4, false)).toBe(99);
    expect(uploadPercent(200, 200, 4, 4, true)).toBe(100);
  });

  it("counts empty files", () => {
    expect(uploadPercent(0, 0, 1, 2, true)).toBe(50);
  });
});
