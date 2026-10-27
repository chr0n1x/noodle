import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApproveButton } from "./ApproveButton";

const mockMutate = vi.fn();
type MockControl = { mutate: ReturnType<typeof vi.fn>; isPending: boolean; variables?: unknown; [key: string]: unknown };
let mockControl: MockControl = { mutate: mockMutate, isPending: false };

vi.mock("~/client", async () => {
  const actual = await vi.importActual("~/client");
  return {
    ...(actual as Record<string, unknown>),
    useSendControl: () => mockControl,
  };
});

describe("ApproveButton", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    mockControl = { mutate: mockMutate, isPending: false };
  });

  it("is busy and disabled while its approve is pending", () => {
    mockControl = { mutate: mockMutate, isPending: true, variables: { action: "approve", order_id: "o-1" } };
    render(<ApproveButton orderId="o-1" />);

    const button = screen.getByRole("button", { name: "Approve" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
  });

  it("sends approve for the order", () => {
    render(<ApproveButton orderId="o-1" />);

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ action: "approve", order_id: "o-1" }),
      expect.anything(),
    );
  });

  it("is disabled and explains why when the loop blocks approve", () => {
    render(<ApproveButton orderId="o-1" blockedReason="order o-1 is held at stage quality; complete it with noodle order o-1 complete quality" />);

    const button = screen.getByRole("button", { name: "Approve" });
    expect(button).toBeDisabled();
    expect(button).toHaveAccessibleDescription(/noodle order o-1 complete quality/);
    fireEvent.click(button);
    expect(mockMutate).not.toHaveBeenCalled();
  });
  it("shows the ack message after approve succeeds", () => {
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: true, isError: false, data: { id: "x", action: "approve", status: "ok", message: "approved, not published; unpublished work kept at 7-0-execute", at: "" }, variables: { action: "approve", order_id: "o-1" }, reset: vi.fn() };
    render(<ApproveButton orderId="o-1" />);
    expect(screen.getByRole("status")).toHaveTextContent("Approve: approved, not published; unpublished work kept at 7-0-execute");
  });

  it("shows the error after approve fails", () => {
    mockControl = { mutate: vi.fn(), isPending: false, isSuccess: false, isError: true, error: new Error("noodle server unreachable"), variables: { action: "approve", order_id: "o-1" }, reset: vi.fn() };
    render(<ApproveButton orderId="o-1" />);
    expect(screen.getByRole("alert")).toHaveTextContent("Approve: noodle server unreachable");
  });

  it("explains continuing in a gated order", () => {
    render(<ApproveButton orderId="o-1" label="Continue to quality" gated />);
    expect(screen.getByRole("button", { name: "Continue to quality" })).toHaveAccessibleDescription(
      "Approves this stage and starts the next one. The branch is kept. The order publishes, or closes with nothing to publish, after its last stage.",
    );
  });
});
