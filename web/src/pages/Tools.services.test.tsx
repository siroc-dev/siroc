import { fireEvent, render, screen } from "@testing-library/react";
import { App as AntdApp } from "antd";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "@/lib/api";
import { Tools } from "./Tools";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
  },
}));

describe("System tools services", () => {
  beforeEach(() => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (String(url).includes("/services")) {
        return [{ name: "nginx", title: "Nginx", version: "1.31.8", active: true, monitor: true, manage: "/nginx", memory: 1048576, processes: 3 }];
      }
      return { hostname: "box", time: "now", timezone: "UTC", ntp: true, dns: [], mounts: [], ips: [], routes: [], ifaces: ["eth0"], netplan: true };
    });
  });

  it("lists service status on the System tools page", async () => {
    render(
      <MemoryRouter>
        <AntdApp>
          <Tools />
        </AntdApp>
      </MemoryRouter>,
    );
    fireEvent.click(await screen.findByRole("tab", { name: "Service Status" }));
    expect(await screen.findByText("Nginx")).toBeInTheDocument();
    expect(screen.getByText("Memcached")).toBeInTheDocument();
    expect(screen.getByText("RAM")).toBeInTheDocument();
    expect(screen.getByText("Processes")).toBeInTheDocument();
    expect(screen.getByText("1.0 MB")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("saves an address and gateway on the IP form", async () => {
    render(
      <MemoryRouter>
        <AntdApp>
          <Tools />
        </AntdApp>
      </MemoryRouter>,
    );
    fireEvent.click(await screen.findByRole("tab", { name: "OS" }));
    expect(await screen.findByPlaceholderText("Gateway")).toBeInTheDocument();
    expect(screen.getByText(/written to netplan/)).toBeInTheDocument();
  });
});
