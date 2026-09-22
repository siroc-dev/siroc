/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { asList } from "./lists";

describe("asList", () => {
  it("turns null into an empty array", () => {
    expect(asList(null)).toEqual([]);
  });

  it("keeps a real array", () => {
    expect(asList(["8.3"])).toEqual(["8.3"]);
  });
});
