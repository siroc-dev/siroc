import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthForm, clearSavedLogin, readSavedLogin } from "./AuthForm";

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

describe("sign-in memory", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.mocked(api.get).mockResolvedValue({ required: false });
    vi.mocked(api.post).mockResolvedValue({ ok: true });
  });

  it("exposes username and password fields for browser autocomplete", () => {
    renderSignIn();
    const username = screen.getByLabelText("Username");
    const password = screen.getByLabelText("Password");
    expect(username.getAttribute("autocomplete")).toBe("username");
    expect(username.getAttribute("name")).toBe("username");
    expect(password.getAttribute("autocomplete")).toBe("current-password");
    expect(password.getAttribute("type")).toBe("password");
    expect(password.getAttribute("name")).toBe("password");
  });

  it("saves the username and password after a successful sign-in", async () => {
    renderSignIn();
    fireEvent.change(screen.getByLabelText("Username"), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "secretpass" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => {
      expect(readSavedLogin()).toEqual({ username: "admin", password: "secretpass" });
    });
  });

  it("fills the saved username and password on the next visit", async () => {
    localStorage.setItem("siroc.login", JSON.stringify({ username: "admin", password: "secretpass" }));
    renderSignIn();
    await waitFor(() => {
      expect((screen.getByLabelText("Username") as HTMLInputElement).value).toBe("admin");
      expect((screen.getByLabelText("Password") as HTMLInputElement).value).toBe("secretpass");
    });
  });

  it("clears the saved login when remember is off", async () => {
    localStorage.setItem("siroc.login", JSON.stringify({ username: "admin", password: "secretpass" }));
    renderSignIn();
    await waitFor(() => {
      expect((screen.getByLabelText("Username") as HTMLInputElement).value).toBe("admin");
    });
    fireEvent.click(screen.getByRole("checkbox", { name: "Remember username and password" }));
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => {
      expect(api.post).toHaveBeenCalled();
    });
    expect(readSavedLogin()).toBeNull();
    clearSavedLogin();
  });
});
