import { describe, expect, it } from "vitest";
import { ADMIN_ONLY_PATHS, canUsePath } from "./nav";

describe("nav", () => {
  it("lets hosting users keep the pages they can use", () => {
    for (const path of ["/", "/sites", "/php", "/files", "/terminal", "/databases", "/backup"]) {
      expect(canUsePath(false, path)).toBe(true);
    }
  });

  it("hides admin-only pages from hosting users", () => {
    for (const path of ADMIN_ONLY_PATHS) {
      expect(canUsePath(false, path)).toBe(false);
      expect(canUsePath(true, path)).toBe(true);
    }
  });
});
