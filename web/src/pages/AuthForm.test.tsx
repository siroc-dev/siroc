import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthForm } from "./AuthForm";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
  },
  RequestError: class RequestError extends Error {},
}));

import { api } from "@/lib/api";

function renderSignIn() {
  return render(
    <AuthForm
      title="Sign in"
      subtitle="Siroc control panel"
      submitLabel="Sign in"
      endpoint="/api/login"
      onDone={() => undefined}
    />,
  );
}

describe("sign-in form", () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem("siroc.login", JSON.stringify({ username: "old", password: "old-password" }));
    window.history.replaceState({}, "", "/login");
    vi.mocked(api.get).mockResolvedValue({ required: false });
    vi.mocked(api.post).mockResolvedValue({ ok: true });
  });

  it("posts username and password so the browser can offer to save them", () => {
    renderSignIn();
    const form = document.getElementById("siroc-sign-in");
    const username = screen.getByLabelText("Username");
    const password = screen.getByLabelText("Password");
    expect(form?.getAttribute("method")).toBe("post");
    expect(form?.getAttribute("action")).toBe("/api/login");
    expect(form?.getAttribute("target")).toBeNull();
    expect(username.getAttribute("autocomplete")).toBe("username");
    expect(username.getAttribute("name")).toBe("username");
    expect(password.getAttribute("autocomplete")).toBe("current-password");
    expect(password.getAttribute("type")).toBe("password");
    expect(password.getAttribute("name")).toBe("password");
    expect(screen.getByRole("button", { name: "Sign in" }).getAttribute("type")).toBe("submit");
    expect(localStorage.getItem("siroc.login")).toBeNull();
  });

  it("shows the error sent back after a rejected sign-in", () => {
    window.history.replaceState({}, "", "/login?error=invalid%20credentials");
    renderSignIn();
    expect(screen.getByText("invalid credentials")).toBeTruthy();
  });
});
