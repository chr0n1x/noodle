import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PublishButton } from "./PublishButton";
import { buildOrder } from "../test-utils";

const mockMutate = vi.fn();
type MockControl = {
  mutate: ReturnType<typeof vi.fn>;
  isPending: boolean;
  variables?: unknown;
  [key: string]: unknown;
};
let mockControl: MockControl = { mutate: mockMutate, isPending: false };

vi.mock("~/client", async () => {
  const actual = await vi.importActual("~/client");
  return {
    ...(actual as Record<string, unknown>),
    useSendControl: () => mockControl,
  };
});

describe("PublishButton", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    mockControl = { mutate: mockMutate, isPending: false };
  });

  it("is busy and disabled while approve-and-publish is pending", () => {
    mockControl = {
      mutate: mockMutate,
      isPending: true,
      variables: { action: "approve-and-publish", order_id: "o-1" },
    };
    render(
      <PublishButton
        order={buildOrder({ id: "o-1", status: "awaiting-publish", publish_gated: true })}
      />,
    );

    const button = screen.getByRole("button", { name: "Approve & Publish" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
  });

  it("renders nothing for an ungated order", () => {
    const { container } = render(
      <PublishButton order={buildOrder({ id: "o-1", status: "awaiting-publish" })} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("sends approve-and-publish for the order", () => {
    render(
      <PublishButton
        order={buildOrder({ id: "o-1", status: "awaiting-publish", publish_gated: true })}
      />,
    );

    const button = screen.getByRole("button", { name: "Approve & Publish" });
    expect(button).toBeEnabled();
    fireEvent.click(button);

    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ action: "approve-and-publish", order_id: "o-1" }),
      expect.anything(),
    );
  });

  it("renders nothing while the order still has stages to run", () => {
    const { container } = render(
      <PublishButton
        order={buildOrder({
          id: "o-1",
          status: "active",
          publish_gated: true,
          publish_blocked_reason:
            "approve-and-publish refused: order o-1 is not awaiting publish (status active)",
          stages: [
            { task_key: "execute", status: "active", provider: "claude", model: "m" },
            { task_key: "quality", status: "pending", provider: "claude", model: "m" },
          ],
        })}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing for a failed or completed order", () => {
    for (const status of ["failed", "failing", "completed"] as const) {
      const { container, unmount } = render(
        <PublishButton order={buildOrder({ id: "o-1", status, publish_gated: true })} />,
      );
      expect(container).toBeEmptyDOMElement();
      unmount();
    }
  });

  it("is disabled with the loop's reason when an awaiting order is still blocked", () => {
    render(
      <PublishButton
        order={buildOrder({
          id: "o-1",
          status: "awaiting-publish",
          publish_gated: true,
          publish_blocked_reason: "raw",
        })}
      />,
    );
    const button = screen.getByRole("button", { name: "Approve & Publish" });
    expect(button).toBeDisabled();
    expect(button.closest("[data-loop-reason]")).toHaveAttribute("data-loop-reason", "raw");
    fireEvent.click(button);
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("shows the last publish error", () => {
    render(
      <PublishButton
        order={buildOrder({
          id: "o-1",
          status: "awaiting-publish",
          publish_gated: true,
          publish_error: "draft-pr: push rejected",
        })}
      />,
    );
    expect(screen.getByText("draft-pr: push rejected")).toBeInTheDocument();
  });
  it("shows the ack message and error for approve-and-publish", () => {
    const order = buildOrder({ id: "o-1", status: "awaiting-publish", publish_gated: true });
    mockControl = {
      mutate: vi.fn(),
      isPending: false,
      isSuccess: true,
      isError: false,
      data: { id: "x", action: "approve-and-publish", status: "ok", message: "published", at: "" },
      variables: { action: "approve-and-publish", order_id: "o-1" },
      reset: vi.fn(),
    };
    const view = render(<PublishButton order={order} />);
    expect(screen.getByRole("status")).toHaveTextContent("Approve & Publish: published");
    mockControl = {
      mutate: vi.fn(),
      isPending: false,
      isSuccess: false,
      isError: true,
      error: new Error("publish refused"),
      variables: { action: "approve-and-publish", order_id: "o-1" },
      reset: vi.fn(),
    };
    view.rerender(<PublishButton order={order} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Approve & Publish: publish refused");
  });
});
