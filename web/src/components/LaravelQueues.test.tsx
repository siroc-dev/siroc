import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LaravelQueues, type LaravelQueueState } from "./LaravelQueues";

const laravel: LaravelQueueState = {
  queue: true,
  workers: 1,
  queues: [{ name: "default", workers: 1 }],
  scheduler: false,
};

describe("queue name input", () => {
  it("keeps focus across keystrokes", () => {
    render(
      <LaravelQueues laravel={laravel} busy={false} onToggle={() => undefined} onApply={() => undefined} onScheduler={() => undefined} />,
    );
    const input = screen.getByDisplayValue("default");
    input.focus();
    fireEvent.change(input, { target: { value: "d" } });
    const next = screen.getByDisplayValue("d");
    expect(next).toBe(document.activeElement);
    fireEvent.change(next, { target: { value: "de" } });
    expect(screen.getByDisplayValue("de")).toBe(document.activeElement);
  });
});
