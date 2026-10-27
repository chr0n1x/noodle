import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewList } from "./ReviewList";
import { buildOrder, buildReview, buildSnapshot, buildStage } from "~/test-utils";

let mockSnapshot = buildSnapshot();
const mockMutate = vi.fn();
type MockControl = { mutate: ReturnType<typeof vi.fn>; isPending: boolean; variables?: unknown; [key: string]: unknown };
let mockControl: MockControl = { mutate: mockMutate, isPending: false };

vi.mock("~/client", async () => {
  const actual = await vi.importActual("~/client");
  return {
    ...(actual as Record<string, unknown>),
    useSuspenseSnapshot: () => ({ data: mockSnapshot }),
    useSendControl: () => mockControl,
    useReviewDiff: () => ({ data: { diff: "d", stat: "" }, isLoading: false, error: undefined }),
  };
});

describe("ReviewList", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    mockControl = { mutate: mockMutate, isPending: false };
    mockSnapshot = buildSnapshot();
  });

  it.each([
    ["merge", 0],
    ["request-changes", 2],
    ["reject", 3],
  ])("marks only the clicked %s button busy and disables all actions", (action, busyIndex) => {
    const review = buildReview({ order_id: "o-1" });
    mockSnapshot = buildSnapshot({ pending_reviews: [review], pending_review_count: 1 });
    mockControl = { mutate: mockMutate, isPending: true, variables: { action, order_id: "o-1" } };

    render(<ReviewList />);

    const footer = document.querySelector(".review-action-footer") as HTMLElement;
    const buttons = Array.from(footer.querySelectorAll("button"));
    expect(buttons).toHaveLength(4);
    buttons.forEach((button, i) => {
      if (i === 1) {
        return; // ApproveButton owns its own mutation
      }
      expect(button).toBeDisabled();
      expect(button.getAttribute("aria-busy")).toBe(i === busyIndex ? "true" : null);
    });
  });

  it("keeps a valid selection when reviews shrink", () => {
    const first = buildReview({ order_id: "o-1", title: "First" });
    const second = buildReview({ order_id: "o-2", title: "Second" });
    mockSnapshot = buildSnapshot({
      pending_reviews: [first, second],
      pending_review_count: 2,
    });

    const view = render(<ReviewList />);
    fireEvent.click(screen.getByRole("tab", { name: "Second" }));

    mockSnapshot = buildSnapshot({
      pending_reviews: [first],
      pending_review_count: 1,
    });
    view.rerender(<ReviewList />);

    expect(screen.getByText("Review Actions")).toBeInTheDocument();
    expect(screen.getByText(/Review:/)).toBeInTheDocument();
    expect(screen.getByText("o-1")).toBeInTheDocument();
  });

  it("sends merge from Approve & Merge and offers no draft PR action", () => {
    mockSnapshot = buildSnapshot({
      pending_reviews: [buildReview({ order_id: "o-1" })],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.queryByRole("button", { name: /Draft PR/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Approve & Merge" }));
    expect(mockMutate.mock.calls[0]?.[0]).toMatchObject({ action: "merge", order_id: "o-1" });
  });

  it("hides Approve & Merge in a gated order and makes Continue the primary action", () => {
    const refusal = "this order publishes at the end; approve the stage, then approve-and-publish";
    mockSnapshot = buildSnapshot({
      orders: [buildOrder({ id: "o-1", stages: [buildStage({ task_key: "execute" }), buildStage({ task_key: "quality" })] })],
      pending_reviews: [buildReview({ order_id: "o-1", publish_gated: true, blocked_actions: ["merge"], blocked_reason: refusal })],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.queryByRole("button", { name: /Approve & Merge/ })).toBeNull();
    const cont = screen.getByRole("button", { name: "Continue to quality" });
    expect(cont).toBeEnabled();
    expect(cont).toHaveClass("review-action-primary");
    expect(screen.getByTestId("review-blocked-reason")).toHaveTextContent("Publishes when the last stage completes");
    expect(screen.queryByText(/approve-and-publish/)).toBeNull();
    fireEvent.click(cont);
    expect(mockMutate.mock.calls[0]?.[0]).toMatchObject({ action: "approve", order_id: "o-1" });
  });

  it("labels the last stage Approve in a gated order", () => {
    mockSnapshot = buildSnapshot({
      orders: [buildOrder({ id: "o-1", stages: [buildStage({ task_key: "execute" }), buildStage({ task_key: "quality" })] })],
      pending_reviews: [buildReview({ order_id: "o-1", stage_index: 1, publish_gated: true, blocked_actions: ["merge"] })],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.getByRole("button", { name: "Approve" })).toHaveClass("review-action-primary");
  });

  it("keeps Approve & Merge primary in an ungated order", () => {
    mockSnapshot = buildSnapshot({
      orders: [buildOrder({ id: "o-1", stages: [buildStage({ task_key: "execute" }), buildStage({ task_key: "reflect" })] })],
      pending_reviews: [buildReview({ order_id: "o-1" })],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.getByRole("button", { name: "Approve & Merge" })).toHaveClass("review-action-primary");
    expect(screen.getByRole("button", { name: "Continue to reflect" })).toHaveClass("review-action-outline");
    expect(screen.queryByTestId("review-blocked-reason")).toBeNull();
  });

  it("keeps the agent message out of the header and asks for a decision", () => {
    const reason = `agent ended its turn without changes: ${"word ".repeat(200)}`;
    mockSnapshot = buildSnapshot({
      pending_reviews: [buildReview({ order_id: "o-1", reason })],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.getByRole("banner")).not.toHaveTextContent("agent ended its turn");
    expect(screen.getByTestId("review-reason")).toHaveClass("line-clamp-3");
    expect(screen.getByText(/Decision needed/)).toBeInTheDocument();
  });

  it("explains the re-run when publishing is gated", () => {
    mockSnapshot = buildSnapshot({
      pending_reviews: [buildReview({ order_id: "o-1", publish_gated: true })],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.getByRole("button", { name: "Request Changes" })).toHaveAttribute(
      "title",
      "Re-runs this stage in the same worktree with your feedback. Fails the order after the rework limit.",
    );
  });

  it("labels tabs and header with the order title and stage", () => {
    mockSnapshot = buildSnapshot({
      pending_reviews: [
        buildReview({ order_id: "1", title: "PROJ-123: fix login bug", task_key: "execute" }),
        buildReview({ order_id: "5", title: "PROJ-125: retry upload", task_key: "quality" }),
      ],
      pending_review_count: 2,
    });

    render(<ReviewList />);

    expect(screen.getByRole("tab", { name: "PROJ-125: retry upload · quality" })).toBeInTheDocument();
    expect(screen.getByRole("banner")).toHaveTextContent("Execute Review:");
    expect(screen.getByRole("banner")).toHaveTextContent("PROJ-123: fix login bug");
  });

  it("disables approve on an external hold and shows the loop's reason", () => {
    const reason = "order o-1 is held at stage quality for external review; complete it with noodle order o-1 complete quality (action stage-complete)";
    mockSnapshot = buildSnapshot({
      pending_reviews: [buildReview({ order_id: "o-1", blocked_actions: ["approve", "merge"], blocked_reason: reason })],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.getByRole("button", { name: "Approve & Merge" })).toBeDisabled();
    const approve = screen.getByRole("button", { name: "Approve" });
    expect(approve).toBeDisabled();
    expect(screen.getByRole("button", { name: "Request Changes" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Request Changes" })).toHaveAttribute(
      "title",
      "Fails the order. The worktree and its uncommitted work are kept.",
    );
    expect(screen.getByRole("button", { name: "Dismiss Order" })).toBeEnabled();
    expect(screen.getByTestId("review-blocked-reason")).toHaveTextContent(reason);
    fireEvent.click(approve);
    expect(mockMutate).not.toHaveBeenCalled();
  });
  it("shows the ack message and error for the clicked action", () => {
    mockSnapshot = buildSnapshot({ pending_reviews: [buildReview({ order_id: "o-1" })], pending_review_count: 1 });
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: true, isError: false, data: { id: "x", action: "merge", status: "ok", message: "merged", at: "" }, variables: { action: "merge", order_id: "o-1" }, reset: vi.fn() };
    const view = render(<ReviewList />);
    expect(screen.getAllByRole("status").map((e) => e.textContent)).toContain("Approve & Merge: mergedDismiss");
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: false, isError: true, error: new Error("order not found"), variables: { action: "reject", order_id: "o-1" }, reset: vi.fn() };
    view.rerender(<ReviewList />);
    expect(screen.getByRole("alert")).toHaveTextContent("Dismiss Order: order not found");
  });
});
