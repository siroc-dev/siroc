/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { ADMIN_ONLY_PATHS, canUsePath, spaClick } from "./nav";

describe("nav", () => {
  it("lets hosting users keep the pages they can use", () => {
    for (const path of ["/", "/sites", "/sites/1", "/php", "/files", "/terminal", "/databases", "/backup", "/logs"]) {
      expect(canUsePath(false, path)).toBe(true);
    }
  });

  it("hides admin-only pages from hosting users", () => {
    for (const path of ADMIN_ONLY_PATHS) {
      expect(canUsePath(false, path)).toBe(false);
      expect(canUsePath(true, path)).toBe(true);
    }
    expect(ADMIN_ONLY_PATHS.has("/redis")).toBe(true);
  });

  it("keeps modifier clicks for a new tab", () => {
    const go = (path: string) => calls.push(path);
    const calls: string[] = [];
    const click = spaClick("/sites", go);
    const ev = (extra: Partial<{ metaKey: boolean; ctrlKey: boolean; shiftKey: boolean; altKey: boolean; button: number }>) => {
      let prevented = false;
      click({
        metaKey: false,
        ctrlKey: false,
        shiftKey: false,
        altKey: false,
        button: 0,
        preventDefault: () => {
          prevented = true;
        },
        ...extra,
      });
      return prevented;
    };
    expect(ev({ ctrlKey: true })).toBe(false);
    expect(calls).toEqual([]);
    expect(ev({})).toBe(true);
    expect(calls).toEqual(["/sites"]);
  });
});
