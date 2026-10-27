import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SchedulerFeed } from "./SchedulerFeed";
import { buildSession, buildSnapshot } from "../test-utils";
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
  };
});

vi.mock("./Toaster", () => ({ toast: vi.fn() }));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children }: { to: string; children: React.ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}));

const BUTTON_NAMES: Record<string, RegExp> = {
  "Stop All": /^Stop All$/,
  Stop: /^Stop(ping…)?$/,
  "Mark Complete": /^(Mark Complete|Completing…)$/,
  Send: /^Send$/,
};

beforeEach(() => {
  mockSnapshot = buildSnapshot();
  mockSend.mockReset();
  mockControl = { mutate: mockSend, isPending: false };
  mockEvents = [];
});

describe("SchedulerFeed", () => {
  it("renders scheduler header", () => {
    render(<SchedulerFeed />);
    expect(screen.getByText("Scheduler")).toBeInTheDocument();
  });

  it("renders empty state when no scheduler session", () => {
    render(<SchedulerFeed />);
    expect(screen.getByText(/The scheduler starts when you run/)).toBeInTheDocument();
    expect(screen.getByText("Learn how the loop works")).toBeInTheDocument();
  });

  it("renders input area", () => {
    render(<SchedulerFeed />);
    expect(screen.getByPlaceholderText("Enter instructions or critique...")).toBeInTheDocument();
    expect(screen.getByText("Send")).toBeInTheDocument();
  });

  it("shows bootstrap status message when schedule bootstrap session is running", () => {
    const bootstrapSession = buildSession({
      id: "bootstrap-schedule-123",
      task_key: "schedule",
      status: "running",
      current_action: "",
    });
    mockSnapshot = buildSnapshot({
      loop_state: "running",
      sessions: [bootstrapSession],
      active: [bootstrapSession],
    });

    render(<SchedulerFeed />);
    expect(screen.getByText("Bootstrapping schedule skill...")).toBeInTheDocument();
    expect(
      screen.getByText("Bootstrapping schedule skill. This usually takes a few seconds."),
    ).toBeInTheDocument();
  });

  it("shows bootstrap status message when schedule bootstrap is pending with no session", () => {
    mockSnapshot = buildSnapshot({
      loop_state: "running",
      sessions: [],
      orders: [
        {
          id: "schedule",
          status: "active",
          stages: [{ provider: "claude", model: "opus", status: "pending", task_key: "schedule" }],
        },
      ],
    });

    render(<SchedulerFeed />);
    expect(screen.getByText("Bootstrapping schedule skill...")).toBeInTheDocument();
    expect(
      screen.getByText("Bootstrapping schedule skill. This usually takes a few seconds."),
    ).toBeInTheDocument();
  });

  it("sends steer command on submit", async () => {
    render(<SchedulerFeed />);
    const user = userEvent.setup();
    const textarea = screen.getByPlaceholderText("Enter instructions or critique...");
    await user.type(textarea, "deploy to staging");
    await user.click(screen.getByText("Send"));
    expect(mockSend).toHaveBeenCalledWith({
      action: "steer",
      target: "schedule",
      prompt: "deploy to staging",
    }, expect.anything());
  });

  it("sends steer command on Enter", async () => {
    render(<SchedulerFeed />);
    const user = userEvent.setup();
    const textarea = screen.getByPlaceholderText("Enter instructions or critique...");
    await user.type(textarea, "run tests{Enter}");
    expect(mockSend).toHaveBeenCalledWith({
      action: "steer",
      target: "schedule",
      prompt: "run tests",
    }, expect.anything());
  });

  it("clears input after submit", async () => {
    render(<SchedulerFeed />);
    const user = userEvent.setup();
    const textarea = screen.getByPlaceholderText(
      "Enter instructions or critique...",
    ) as HTMLTextAreaElement;
    await user.type(textarea, "do something");
    await user.click(screen.getByText("Send"));
    expect(textarea.value).toBe("");
  });

  it("does not send empty input", async () => {
    render(<SchedulerFeed />);
    const user = userEvent.setup();
    await user.click(screen.getByText("Send"));
    expect(mockSend).not.toHaveBeenCalled();
  });

  it("shows idle prompt when loop is idle even if active list still includes schedule", () => {
    const session = buildSession({
      id: "scheduler-1",
      loop_state: "idle",
      task_key: "schedule",
    });
    mockSnapshot = buildSnapshot({
      loop_state: "idle",
      sessions: [session],
      active: [session],
    });

    render(<SchedulerFeed />);
    expect(screen.getByText("Talk to the scheduler...")).toBeInTheDocument();
    expect(screen.queryByText("Thinking…")).not.toBeInTheDocument();
  });

  it("shows idle prompt when scheduler session is active but has no current action", () => {
    const session = buildSession({
      id: "scheduler-2",
      loop_state: "running",
      task_key: "schedule",
      current_action: "",
    });
    mockSnapshot = buildSnapshot({
      loop_state: "running",
      sessions: [session],
      active: [session],
    });

    render(<SchedulerFeed />);
    expect(screen.getByText("Talk to the scheduler...")).toBeInTheDocument();
    expect(screen.queryByText("Thinking…")).not.toBeInTheDocument();
  });

  it.each([
    ["stop-all", "Stop All"],
    ["stop", "Stop"],
    ["steer", "Send"],
  ])("marks only the clicked %s button busy and disables all control buttons", (action, label) => {
    const session = buildSession({ id: "scheduler-3", task_key: "schedule", current_action: "Think" });
    mockSnapshot = buildSnapshot({ loop_state: "running", sessions: [session], active: [session] });
    mockControl = { mutate: mockSend, isPending: true, variables: { action } };

    render(<SchedulerFeed />);

    for (const name of ["Stop All", "Stop", "Send"]) {
      const button = screen.getByRole("button", { name: BUTTON_NAMES[name] });
      expect(button).toBeDisabled();
      expect(button.getAttribute("aria-busy")).toBe(name === label ? "true" : null);
    }
  });

  it("sends Stop All through the reported path", async () => {
    render(<SchedulerFeed />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Stop All" }));
    expect(mockSend).toHaveBeenCalledWith({ action: "stop-all" }, expect.anything());
  });

  it("shows a sent steer as pending until delivered and its event arrives", async () => {
    const session = buildSession({ id: "scheduler-4", task_key: "schedule", current_action: "" });
    mockSnapshot = buildSnapshot({ loop_state: "running", sessions: [session] });
    const view = render(<SchedulerFeed />);
    const user = userEvent.setup();
    await user.type(screen.getByPlaceholderText("Enter instructions or critique..."), "go faster{Enter}");

    const steer = screen.getByTestId("pending-steer");
    expect(steer).toHaveTextContent("go faster");
    expect(steer).toHaveAttribute("data-state", "pending");
    expect(steer).toHaveAttribute("aria-busy", "true");

    const [, opts] = mockSend.mock.calls[0];
    act(() => opts.onSuccess({ id: "c1", action: "steer", status: "ok", at: "" }));
    expect(screen.getByTestId("pending-steer")).toHaveAttribute("data-state", "delivered");

    mockEvents = [{ at: new Date().toISOString(), label: "User", body: "go faster", category: "all" }];
    view.rerender(<SchedulerFeed />);
    expect(screen.queryByTestId("pending-steer")).toBeNull();
  });

  it("keeps a failed steer visible with the error message", async () => {
    render(<SchedulerFeed />);
    await userEvent.setup().type(
      screen.getByPlaceholderText("Enter instructions or critique..."),
      "go faster{Enter}",
    );

    const [, opts] = mockSend.mock.calls[0];
    act(() => opts.onError(new Error("noodle server unreachable")));

    const steer = screen.getByTestId("pending-steer");
    expect(steer).toHaveAttribute("data-state", "error");
    expect(steer).not.toHaveAttribute("aria-busy", "true");
    expect(steer).toHaveTextContent("noodle server unreachable");
  });
  it("shows the ack message and error for the clicked action", () => {
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: true, isError: false, data: { id: "x", action: "stop-all", status: "ok", message: "stopped 2 sessions", at: "" }, variables: { action: "stop-all" }, reset: vi.fn() } as never;
    const view = render(<SchedulerFeed />);
    expect(screen.getByRole("status")).toHaveTextContent("Stop All: stopped 2 sessions");
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: false, isError: true, error: new Error("noodle server unreachable"), variables: { action: "steer" }, reset: vi.fn() } as never;
    view.rerender(<SchedulerFeed />);
    expect(screen.getAllByRole("alert").map((e) => e.textContent)).toContain("Send: noodle server unreachableDismiss");
  });
});
