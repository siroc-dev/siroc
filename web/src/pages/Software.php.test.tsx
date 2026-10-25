import { fireEvent, render, screen } from "@testing-library/react";
import { App as AntdApp } from "antd";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "@/lib/api";
import { Software } from "./Software";

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

const php = {
  name: "php",
  title: "PHP-FPM",
  installed: true,
  version: "8.4",
  service: "",
  active: true,
  versions: ["7.4", "8.1", "8.2", "8.3", "8.4"],
  installedVersions: ["8.4"],
  cliVersion: "8.4",
};

describe("PHP multi version install", () => {
  beforeEach(() => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (String(url).includes("/jobs")) return { jobs: [], queue: [], active: 0 };
      return [php];
    });
  });

  it("offers another PHP version, including 7.4, after one is installed", async () => {
    render(
      <MemoryRouter>
        <AntdApp>
          <Software />
        </AntdApp>
      </MemoryRouter>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Install" }));
    expect(await screen.findByText("8.3")).toBeInTheDocument();
    const boxes = screen.getAllByRole("combobox");
    fireEvent.mouseDown(boxes[boxes.length - 1]);
    expect(await screen.findByText("7.4")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Update" })).toBeInTheDocument();
  });
});
