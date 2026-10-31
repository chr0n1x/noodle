import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LoopControlButton } from "./LoopControlButton";

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

describe("LoopControlButton", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    mockControl = { mutate: mockMutate, isPending: false };
  });

  it.each(["running", "idle"] as const)("sends pause while the loop is %s", (state) => {
    render(<LoopControlButton loopState={state} />);

    const button = screen.getByRole("button", { name: "Pause loop" });
    fireEvent.click(button);

    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ action: "pause" }),
      expect.anything(),
    );
  });

  it.each(["paused", "draining"] as const)("sends resume while the loop is %s", (state) => {
    render(<LoopControlButton loopState={state} />);

    const button = screen.getByRole("button", { name: "Resume loop" });
    expect(button).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(button);

    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ action: "resume" }),
      expect.anything(),
    );
  });

  it("shows the paused state", () => {
    render(<LoopControlButton loopState="paused" />);
    expect(screen.getByText("PAUSED")).toBeInTheDocument();
  });

  it("is busy and disabled while a pause is pending", () => {
    mockControl = { mutate: mockMutate, isPending: true, variables: { action: "pause" } };
    render(<LoopControlButton loopState="running" />);

    const button = screen.getByRole("button", { name: "Pause loop" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
  });
});
