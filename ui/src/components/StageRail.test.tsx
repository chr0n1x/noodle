import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
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
});
