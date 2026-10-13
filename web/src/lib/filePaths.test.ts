/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { accountAbs, fileCrumbs, isSystemPath, joinPath, moveDestinations, normalizeFileJump, parentFilePath } from "./filePaths";

describe("moveDestinations", () => {
  it("keeps folder + name for one item", () => {
    expect(moveDestinations([{ path: "/composer/assets", name: "assets" }], "/composer/vendor/assets")).toEqual([
      { path: "/composer/assets", dest: "/composer/vendor/assets" },
    ]);
  });

  it("moves every checked name into the destination folder", () => {
    const items = [
      { path: "/composer/assets", name: "assets" },
      { path: "/composer/templates", name: "templates" },
      { path: "/composer/composer.json", name: "composer.json" },
    ];
    expect(moveDestinations(items, "/domains/shop/public_html")).toEqual([
      { path: "/composer/assets", dest: "/domains/shop/public_html/assets" },
      { path: "/composer/templates", dest: "/domains/shop/public_html/templates" },
      { path: "/composer/composer.json", dest: "/domains/shop/public_html/composer.json" },
    ]);
  });
});

describe("normalizeFileJump", () => {
  it("keeps an absolute path", () => {
    expect(normalizeFileJump(" /etc/ssh/ ")).toBe("/etc/ssh");
    expect(normalizeFileJump("etc/ssh")).toBe("/etc/ssh");
    expect(normalizeFileJump("/etc/../etc/ssh/")).toBe("/etc/ssh");
    expect(normalizeFileJump("/")).toBe("/");
  });
});

describe("isSystemPath", () => {
  it("matches filesystem roots and not an account folder", () => {
    expect(isSystemPath("/etc/ssh")).toBe(true);
    expect(isSystemPath("/home/user")).toBe(true);
    expect(isSystemPath("/domains/example/public_html")).toBe(false);
  });
});

describe("joinPath", () => {
  it("joins under home", () => {
    expect(joinPath("/composer", "assets")).toBe("/composer/assets");
    expect(joinPath("/", "assets")).toBe("/assets");
  });
});

describe("external document root", () => {
  it("does not prefix the account home onto a mounted document root", () => {
    expect(accountAbs("/home/abraventure", "/mnt/data/plyxxx", "/mnt/data/plyxxx")).toBe("/mnt/data/plyxxx");
    expect(accountAbs("/home/abraventure", "/mnt/data/plyxxx/index.php", "/mnt/data/plyxxx")).toBe("/mnt/data/plyxxx/index.php");
    expect(accountAbs("/home/abraventure", "/domains/abraventure.com/public_html")).toBe(
      "/home/abraventure/domains/abraventure.com/public_html",
    );
  });

  it("goes up to the account home from the document root", () => {
    expect(parentFilePath("/mnt/data/plyxxx", "/mnt/data/plyxxx")).toBe("/");
    expect(parentFilePath("/mnt/data/plyxxx/css", "/mnt/data/plyxxx")).toBe("/mnt/data/plyxxx");
    expect(parentFilePath("/domains/a", "")).toBe("/domains");
  });

  it("does not offer /mnt or /mnt/data as folders inside the home", () => {
    expect(fileCrumbs("/mnt/data/plyxxx", "/mnt/data/plyxxx").map((c) => c.path)).toEqual(["/", "/mnt/data/plyxxx"]);
    expect(fileCrumbs("/mnt/data/plyxxx/css", "/mnt/data/plyxxx").map((c) => c.path)).toEqual([
      "/",
      "/mnt/data/plyxxx",
      "/mnt/data/plyxxx/css",
    ]);
  });
});
