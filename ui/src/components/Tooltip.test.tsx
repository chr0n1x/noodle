import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Tooltip } from "./Tooltip";

describe("Tooltip right placement", () => {
  afterEach(() => vi.useRealTimers());

  it("opens to the right of the trigger and is capped to the viewport width", () => {
    vi.useFakeTimers();
    render(
      <Tooltip content="Not ready" multiline placement="right">
        <button type="button">go</button>
      </Tooltip>,
    );
    const trigger = screen.getByRole("button", { name: "go" }).parentElement as HTMLElement;
    trigger.getBoundingClientRect = () =>
      ({ top: 100, left: 40, right: 200, bottom: 120, width: 160, height: 20, x: 40, y: 100, toJSON: () => ({}) }) as DOMRect;
    fireEvent.mouseEnter(trigger);
    act(() => {
      vi.advanceTimersByTime(500);
    });
    const tip = screen.getByRole("tooltip");
    expect(tip.style.left).toBe("208px");
    expect(tip.style.top).toBe("110px");
    expect(tip.className).toContain("-translate-y-1/2");
    expect(tip.className).not.toContain("-translate-x");
    expect(tip.className).toContain("max-w-[calc(100vw-16px)]");
  });
});
