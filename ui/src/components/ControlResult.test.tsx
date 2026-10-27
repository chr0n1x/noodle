import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ControlResult } from "./ControlResult";

function succeeded(action: string, message?: string) {
  return {
    isSuccess: true,
    isError: false,
    data: { id: "x", action, status: "ok", message, at: "" },
    variables: { action, order_id: "o-1" },
    reset: vi.fn(),
  } as never;
}

function failed(action: string, message: string) {
  return {
    isSuccess: false,
    isError: true,
    error: new Error(message),
    variables: { action, order_id: "o-1" },
    reset: vi.fn(),
  } as never;
}

describe("ControlResult", () => {
  it("shows the ack message with role status on success", () => {
    render(
      <ControlResult
        mutation={succeeded("approve", "approved, not published; unpublished work kept at 7-0-execute")}
        labels={{ approve: "Approve" }}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Approve: approved, not published; unpublished work kept at 7-0-execute",
    );
  });

  it("shows ok when the ack has no message", () => {
    render(<ControlResult mutation={succeeded("approve")} labels={{ approve: "Approve" }} />);
    expect(screen.getByRole("status")).toHaveTextContent("Approve: ok");
  });

  it("shows the error with role alert on failure", () => {
    render(
      <ControlResult mutation={failed("merge", "noodle server unreachable")} labels={{ merge: "Merge" }} />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Merge: noodle server unreachable");
  });

  it("renders nothing for another action", () => {
    const { container } = render(
      <ControlResult mutation={succeeded("reject", "done")} labels={{ approve: "Approve" }} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("resets the mutation on dismiss", () => {
    const mutation = succeeded("approve", "done") as unknown as { reset: () => void };
    render(<ControlResult mutation={mutation as never} labels={{ approve: "Approve" }} />);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss result" }));
    expect(mutation.reset).toHaveBeenCalled();
  });

  it("resets the mutation when the subject changes", () => {
    const mutation = succeeded("approve", "done") as unknown as { reset: () => void };
    const view = render(
      <ControlResult mutation={mutation as never} labels={{ approve: "Approve" }} subject="o-1" />,
    );
    expect(mutation.reset).not.toHaveBeenCalled();
    view.rerender(
      <ControlResult mutation={mutation as never} labels={{ approve: "Approve" }} subject="o-2" />,
    );
    expect(mutation.reset).toHaveBeenCalled();
  });
});
