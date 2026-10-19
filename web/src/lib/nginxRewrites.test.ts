/** @vitest-environment node */
import { describe, expect, it } from "vitest";
import { matchNginxRewrite, nginxRewriteBody } from "./nginxRewrites";

describe("nginx rewrite templates", () => {
  it("fills the vod location for the nginx vod module", () => {
    const body = nginxRewriteBody("vod");
    expect(body).toContain("location /vod/ {");
    expect(body).toContain("vod hls;");
    expect(body).toContain("vod_mode local;");
    expect(body).toContain("alias /home/user/videos/;");
    expect(body).not.toContain("aio ");
    expect(matchNginxRewrite(body)).toBe("vod");
  });
});
