import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewBanner } from "./ReviewBanner";
import { buildOrder, buildReview, buildSnapshot, buildStage } from "~/test-utils";

let mockSnapshot = buildSnapshot();
type MockControl = { mutate: ReturnType<typeof vi.fn>; isPending: boolean; variables?: unknown; [key: string]: unknown };
let mockControl: MockControl = { mutate: vi.fn(), isPending: false };

vi.mock("~/client", async () => {
  const actual = await vi.importActual("~/client");
  return {
    ...(actual as Record<string, unknown>),
    useSuspenseSnapshot: () => ({ data: mockSnapshot }),
    useSendControl: () => mockControl,
    useReviewDiff: () => ({ data: { diff: "d", stat: "" }, isLoading: false }),
  };
});

const longReason = `agent ended its turn without changes: ${"word ".repeat(200)}`;

describe("ReviewBanner", () => {
  beforeEach(() => {
    mockSnapshot = buildSnapshot();
    mockControl = { mutate: vi.fn(), isPending: false };
  });

  it.each([
    ["merge", 0],
    ["reject", 2],
    ["request-changes", 3],
  ])("marks only the clicked %s button busy and disables all actions", (action, busyIndex) => {
    mockControl = { mutate: vi.fn(), isPending: true, variables: { action, order_id: "o-1" } };
    const { container } = render(<ReviewBanner review={buildReview({ order_id: "o-1" })} />);

    const buttons = Array.from(container.querySelectorAll(".flex.gap-2 button"));
    expect(buttons).toHaveLength(4);
    buttons.forEach((button, i) => {
      if (i === 1) {
        return; // ApproveButton owns its own mutation
      }
      expect(button).toBeDisabled();
      expect(button.getAttribute("aria-busy")).toBe(i === busyIndex ? "true" : null);
    });
  });

  it("clamps a long agent message behind a toggle", () => {
    render(<ReviewBanner review={buildReview({ order_id: "o-1", reason: longReason })} />);
    expect(screen.getByTestId("review-reason")).toHaveClass("line-clamp-3");
    fireEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(screen.getByTestId("review-reason")).not.toHaveClass("line-clamp-3");
  });

  it("explains the re-run when publishing is gated", () => {
    render(<ReviewBanner review={buildReview({ order_id: "o-1", publish_gated: true })} />);
    expect(screen.getByRole("button", { name: "REQUEST CHANGES" })).toHaveAttribute(
      "title",
      "Re-runs this stage in the same worktree with your feedback. Fails the order after the rework limit.",
    );
  });

  it("disables approve on an external review hold", () => {
    const reason = "order o-1 is held at stage quality for external review; complete it with noodle order o-1 complete quality (action stage-complete)";
    render(<ReviewBanner review={buildReview({ order_id: "o-1", blocked_actions: ["approve", "merge"], blocked_reason: reason })} />);
    expect(screen.getByRole("button", { name: "Approve" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "MERGE" })).toBeDisabled();
    expect(screen.queryByRole("button", { name: /Draft PR/ })).toBeNull();
    expect(screen.getByRole("button", { name: "REJECT" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "REQUEST CHANGES" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "REQUEST CHANGES" })).toHaveAttribute(
      "title",
      "Fails the order. The worktree and its uncommitted work are kept.",
    );
    expect(screen.getByTestId("review-blocked-reason")).toHaveTextContent(reason);
  });

  it("hides MERGE in a gated order and leads with Continue", () => {
    const refusal = "this order publishes at the end; approve the stage, then approve-and-publish";
    mockSnapshot = buildSnapshot({
      orders: [buildOrder({ id: "o-1", stages: [buildStage({ task_key: "execute" }), buildStage({ task_key: "quality" })] })],
    });
    render(<ReviewBanner review={buildReview({ order_id: "o-1", publish_gated: true, blocked_actions: ["merge"], blocked_reason: refusal })} />);
    expect(screen.queryByRole("button", { name: /MERGE/ })).toBeNull();
    const cont = screen.getByRole("button", { name: "Continue to quality" });
    expect(cont).toBeEnabled();
    expect(cont).toHaveClass("bg-green");
    expect(screen.getByTestId("review-blocked-reason")).toHaveTextContent("Publishes when the last stage completes");
  });

  it("shows no blocked reason when nothing is blocked", () => {
    render(<ReviewBanner review={buildReview({ order_id: "o-1" })} />);
    expect(screen.getByRole("button", { name: "MERGE" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Approve" })).toBeEnabled();
    expect(screen.queryByTestId("review-blocked-reason")).toBeNull();
  });
  it("shows the ack message and error for the clicked action", () => {
    const review = buildReview({ order_id: "o-1" });
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: true, isError: false, data: { id: "x", action: "merge", status: "ok", message: "merged", at: "" }, variables: { action: "merge", order_id: "o-1" }, reset: vi.fn() };
    const view = render(<ReviewBanner review={review} />);
    expect(screen.getAllByRole("status").map((e) => e.textContent)).toContain("MERGE: mergedDismiss");
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: false, isError: true, error: new Error("order not found"), variables: { action: "reject", order_id: "o-1" }, reset: vi.fn() };
    view.rerender(<ReviewBanner review={review} />);
    expect(screen.getByRole("alert")).toHaveTextContent("REJECT: order not found");
  });
});
