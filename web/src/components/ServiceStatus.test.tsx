import { fireEvent, render, screen } from "@testing-library/react";
import { App as AntdApp } from "antd";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "@/lib/api";
import { ServiceStatus } from "./ServiceStatus";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
  },
}));

describe("Service Status", () => {
  beforeEach(() => {
    vi.mocked(api.post).mockResolvedValue({});
    vi.mocked(api.get).mockResolvedValue([
      { name: "nginx", title: "Nginx", version: "1.31.8", active: true, monitor: true, memory: 1048576, processes: 3 },
      { name: "redis", title: "Redis", version: "7", active: false, monitor: false },
    ]);
  });

  it("starts a stopped service and stops a running one", async () => {
    render(
      <MemoryRouter>
        <AntdApp>
          <ServiceStatus />
        </AntdApp>
      </MemoryRouter>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Stop" }));
    expect(vi.mocked(api.post)).toHaveBeenCalledWith("/api/software/service", { name: "nginx", action: "stop" });
    fireEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(vi.mocked(api.post)).toHaveBeenCalledWith("/api/software/service", { name: "redis", action: "start" });
  });
});
