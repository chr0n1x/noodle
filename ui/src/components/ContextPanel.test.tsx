import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ContextPanel } from "./ContextPanel";
import { buildOrder, buildSession, buildSnapshot, buildStage } from "../test-utils";

let mockSnapshot = buildSnapshot();
let mockActiveChannel: { type: "scheduler" } | { type: "agent"; sessionId: string } = {
  type: "scheduler",
};
const mockNavigate = vi.fn();

vi.mock("~/client", async () => {
  const actual = await vi.importActual("~/client");
  return {
    ...(actual as Record<string, unknown>),
    useSuspenseSnapshot: () => ({ data: mockSnapshot }),
    useActiveChannel: () => ({
      activeChannel: mockActiveChannel,
      setActiveChannel: vi.fn(),
    }),
    useReviewDiff: () => ({ data: undefined, isLoading: false, error: undefined }),
  };
});

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

describe("ContextPanel", () => {
  beforeEach(() => {
    const session = buildSession({ id: "actor-1", worktree_name: "wt-actor-1" });
    mockSnapshot = buildSnapshot({ sessions: [session] });
    mockActiveChannel = { type: "agent", sessionId: session.id };
    mockNavigate.mockReset();
  });

  it("renders the session worktree in the agent stats area", () => {
    render(<ContextPanel />);

    expect(screen.getByText("Worktree")).toBeInTheDocument();
    expect(screen.getByText("wt-actor-1")).toBeInTheDocument();
  });

  it("opens a pipeline stage's transcript when it is clicked", async () => {
    const user = userEvent.setup();
    const session = buildSession({ id: "16-0-execute-a" });
    mockSnapshot = buildSnapshot({
      sessions: [session],
      orders: [
        buildOrder({
          id: "16",
          stages: [
            buildStage({ task_key: "execute", status: "completed", session_id: "16-0-execute-a" }),
            buildStage({
              task_key: "adversarial-review",
              status: "completed",
              session_id: "16-1-review-b",
            }),
          ],
        }),
      ],
    });
    mockActiveChannel = { type: "agent", sessionId: session.id };

    render(<ContextPanel />);
    await user.click(screen.getByRole("button", { name: "adversarial-review" }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: "/actor/$id",
      params: { id: "16-1-review-b" },
    });
  });
});
