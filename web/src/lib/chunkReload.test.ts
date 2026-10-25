import { describe, expect, it } from "vitest";
import { shouldReloadMissingChunk } from "./chunkReload";

describe("shouldReloadMissingChunk", () => {
  it("reloads when a chunk from the previous build is gone", () => {
    expect(shouldReloadMissingChunk(0, 1_000)).toBe(true);
  });

  it("waits before trying the same reload again", () => {
    expect(shouldReloadMissingChunk(5_000, 12_000)).toBe(false);
    expect(shouldReloadMissingChunk(5_000, 15_000)).toBe(true);
  });
});
