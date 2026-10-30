import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StageRail } from "./StageRail";
import { buildStage } from "../test-utils";

describe("StageRail", () => {
  it("draws cancelled (skipped) stages with their own dot, not as pending", () => {
    const { container } = render(
      <StageRail
        stages={[
          buildStage({ status: "pending", task_key: "execute" }),
          buildStage({ status: "cancelled", task_key: "quality" }),
        ]}
      />,
    );
    const dots = container.querySelectorAll(".stage-dot");
    expect(dots[0]).toHaveClass("pending");
    expect(dots[1]).toHaveClass("skipped");
    expect(dots[1]).not.toHaveClass("pending");
  });

  it("opens the session of a stage that has run", async () => {
    const user = userEvent.setup();
    const onOpenSession = vi.fn();
    render(
      <StageRail
        stages={[
          buildStage({ status: "completed", task_key: "execute", session_id: "16-0-execute-a" }),
          buildStage({
            status: "active",
            task_key: "adversarial-review",
            session_id: "16-1-review-b",
          }),
        ]}
        onOpenSession={onOpenSession}
      />,
    );
    await user.click(screen.getByRole("button", { name: "execute" }));
    await user.click(screen.getByRole("button", { name: "adversarial-review" }));
    expect(onOpenSession.mock.calls).toEqual([["16-0-execute-a"], ["16-1-review-b"]]);
  });

  it("leaves a stage that has not run as a plain row", () => {
    render(
      <StageRail
        stages={[buildStage({ status: "pending", task_key: "reflect", session_id: undefined })]}
        onOpenSession={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("reflect")).toBeInTheDocument();
  });
});
