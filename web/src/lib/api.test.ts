import { describe, expect, it } from "vitest";
import { shouldGoToLogin, UploadCancelled, uploadWasCancelled } from "./api";

describe("session expiry", () => {
  it("sends an expired panel session to the login page", () => {
    expect(shouldGoToLogin(401, "/api/sites", "/sites")).toBe(true);
    expect(shouldGoToLogin(401, "/api/me", "/backup")).toBe(true);
  });

  it("leaves a wrong password on the sign-in form", () => {
    expect(shouldGoToLogin(401, "/api/login", "/login")).toBe(false);
    expect(shouldGoToLogin(401, "/api/setup", "/setup")).toBe(false);
    expect(shouldGoToLogin(401, "/api/login/captcha", "/login")).toBe(false);
  });

  it("does not treat other errors as an expired session", () => {
    expect(shouldGoToLogin(403, "/api/software", "/software")).toBe(false);
    expect(shouldGoToLogin(400, "/api/sites", "/sites")).toBe(false);
  });

  it("recognises a cancelled upload", () => {
    expect(uploadWasCancelled(new UploadCancelled())).toBe(true);
    expect(uploadWasCancelled(new Error("Upload failed"))).toBe(false);
  });
});
