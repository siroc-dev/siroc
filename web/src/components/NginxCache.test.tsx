import { describe, expect, it } from "vitest";
import { applyCachePreset, cacheFromSaved, cachePayload, cacheSizeBytes, cacheSizeToken, type NginxCacheValue } from "./NginxCache";

const blank: NginxCacheValue = {
  on: true,
  preset: "public",
  valid: "1h",
  path: "/var/cache/nginx/siroc/test.com",
  size: "1g",
  idle: "1d",
  stale: false,
  refresh: false,
  honor: false,
  cookies: "",
  skip: "",
};

describe("Nginx cache presets", () => {
  it("fills the WordPress recipe", () => {
    const next = applyCachePreset(blank, "wordpress");
    expect(next.preset).toBe("wordpress");
    expect(next.cookies).toContain("wordpress_logged_in");
    expect(next.skip).toContain("/wp-admin");
    expect(next.stale).toBe(true);
    expect(next.refresh).toBe(true);
    const saved = cachePayload(next);
    expect(saved.preset).toBe("wordpress");
    expect(saved.valid).toBe("");
    expect(saved.idle).toBe("7d");
  });

  it("stores a typed disk limit and treats 1 GB as the default", () => {
    expect(cacheSizeToken("500 MB")).toBe("500m");
    expect(cacheSizeToken("1024m")).toBe("");
    expect(cacheSizeToken("1.5g")).toBeNull();
    expect(cacheSizeBytes("9g")).toBe(9 * 1024 ** 3);
    const saved = cachePayload({ ...blank, size: "2 GB" });
    expect(saved.size).toBe("2g");
    expect(cachePayload({ ...blank, size: "1g" }).size).toBe("");
  });

  it("recognizes a saved public cache", () => {
    const loaded = cacheFromSaved({ enabled: true, path: "/var/cache/nginx/siroc/test.com" }, "test.com");
    expect(loaded.preset).toBe("public");
    expect(loaded.valid).toBe("1h");
  });
});
