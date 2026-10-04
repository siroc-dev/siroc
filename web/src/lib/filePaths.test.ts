/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { joinPath, moveDestinations } from "./filePaths";

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

describe("joinPath", () => {
  it("joins under home", () => {
    expect(joinPath("/composer", "assets")).toBe("/composer/assets");
    expect(joinPath("/", "assets")).toBe("/assets");
  });
});
