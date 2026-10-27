import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentFeed } from "./AgentFeed";
import { buildOrder, buildSession, buildSnapshot, buildStage } from "../test-utils";
import type { EventLine, Snapshot } from "~/client";

const mockSend = vi.fn();
let mockSnapshot: Snapshot = buildSnapshot();
let mockControl: { mutate: typeof mockSend; isPending: boolean; variables?: unknown } = {
  mutate: mockSend,
  isPending: false,
};
let mockEvents: EventLine[] = [];

vi.mock("~/client", async () => {
  const actual = await vi.importActual("~/client");
  return {
    ...(actual as Record<string, unknown>),
    useSuspenseSnapshot: () => ({ data: mockSnapshot }),
    useSendControl: () => mockControl,
    useSessionEvents: () => ({ data: mockEvents }),
    subscribeDelta: () => () => {},
  };
});

vi.mock("./Toaster", () => ({ toast: vi.fn() }));

function thinkingSnapshot(): Snapshot {
  const session = buildSession({ id: "s-1", current_action: "Edit a.ts" });
  return buildSnapshot({
    loop_state: "running",
    sessions: [session],
    active: [session],
    orders: [buildOrder({ id: "o-1", stages: [buildStage({ session_id: "s-1" })] })],
  });
}

const BUTTON_NAMES: Record<string, RegExp> = {
  "Stop All": /^Stop All$/,
  Stop: /^Stop(ping…)?$/,
  "Mark Complete": /^(Mark Complete|Completing…)$/,
  Send: /^Send$/,
};

beforeEach(() => {
  mockSnapshot = thinkingSnapshot();
  mockSend.mockReset();
  mockControl = { mutate: mockSend, isPending: false };
  mockEvents = [];
});

describe("AgentFeed", () => {
  it.each([
    ["stop-all", "Stop All"],
    ["stop", "Stop"],
    ["advance", "Mark Complete"],
    ["steer", "Send"],
  ])("marks only the clicked %s button busy and disables all control buttons", (action, label) => {
    mockControl = { mutate: mockSend, isPending: true, variables: { action } };
    render(<AgentFeed sessionId="s-1" />);

    for (const name of ["Stop All", "Stop", "Mark Complete", "Send"]) {
      const button = screen.getByRole("button", { name: BUTTON_NAMES[name] });
      expect(button).toBeDisabled();
      expect(button.getAttribute("aria-busy")).toBe(name === label ? "true" : null);
    }
  });

  it("sends force complete through the reported path", async () => {
    render(<AgentFeed sessionId="s-1" />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Mark Complete" }));
    expect(mockSend).toHaveBeenCalledWith({ action: "advance", order_id: "o-1" }, expect.anything());
  });

  it("shows a steer as pending, then removes it when the loop echoes it", async () => {
    const view = render(<AgentFeed sessionId="s-1" />);
    const user = userEvent.setup();
    const textarea = screen.getByRole("textbox", { name: "Agent instructions" }) as HTMLTextAreaElement;
    await user.type(textarea, "use the cache{Enter}");

    expect(textarea.value).toBe("");
    expect(mockSend).toHaveBeenCalledWith(
      { action: "steer", target: "s-1", prompt: "use the cache" },
      expect.anything(),
    );
    expect(screen.getByTestId("pending-steer")).toHaveAttribute("data-state", "pending");

    const [, opts] = mockSend.mock.calls[0];
    act(() => opts.onSuccess({ id: "c1", action: "steer", status: "ok", at: "" }));
    expect(screen.getByTestId("pending-steer")).toHaveAttribute("data-state", "delivered");

    mockEvents = [{ at: new Date().toISOString(), label: "User", body: "use the cache", category: "all" }];
    view.rerender(<AgentFeed sessionId="s-1" />);
    expect(screen.queryByTestId("pending-steer")).toBeNull();
  });

  it("keeps a steer with the loop's error message when the ack fails", async () => {
    render(<AgentFeed sessionId="s-1" />);
    await userEvent.setup().type(
      screen.getByRole("textbox", { name: "Agent instructions" }),
      "use the cache{Enter}",
    );

    const [, opts] = mockSend.mock.calls[0];
    act(() => opts.onError(new Error("session not found")));

    const steer = screen.getByTestId("pending-steer");
    expect(steer).toHaveAttribute("data-state", "error");
    expect(steer).toHaveTextContent("session not found");
  });
  it("shows the ack message and error for the clicked action", () => {
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: true, isError: false, data: { id: "x", action: "advance", status: "ok", message: "advanced o-1", at: "" }, variables: { action: "advance" }, reset: vi.fn() } as never;
    const view = render(<AgentFeed sessionId="s-1" />);
    expect(screen.getAllByRole("status").map((e) => e.textContent)).toContain("Mark Complete: advanced o-1Dismiss");
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: false, isError: true, error: new Error("noodle server unreachable"), variables: { action: "stop-all" }, reset: vi.fn() } as never;
    view.rerender(<AgentFeed sessionId="s-1" />);
    expect(screen.getByRole("alert")).toHaveTextContent("Stop All: noodle server unreachable");
  });
});
