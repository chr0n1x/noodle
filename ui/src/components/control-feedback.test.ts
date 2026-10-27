import { beforeEach, describe, expect, it, vi } from "vitest";
import { isBusy, sendReported } from "./control-feedback";
import { toast } from "./Toaster";

vi.mock("./Toaster", () => ({ toast: vi.fn() }));

describe("sendReported", () => {
  beforeEach(() => {
    vi.mocked(toast).mockReset();
  });

  it("toasts the label and error message when the command fails", () => {
    const send = vi.fn((_cmd, opts) => opts.onError(new Error("at max concurrency")));

    sendReported(send, { action: "request-changes", order_id: "o-1" }, "Request changes");

    expect(send).toHaveBeenCalledWith(
      { action: "request-changes", order_id: "o-1" },
      expect.anything(),
    );
    expect(toast).toHaveBeenCalledWith("error", "Request changes failed: at max concurrency", 0);
  });

  it("toasts an unreachable server", () => {
    const send = vi.fn((_cmd, opts) => opts.onError(new Error("noodle server unreachable")));

    sendReported(send, { action: "stop-all" }, "Stop All");

    expect(toast).toHaveBeenCalledWith("error", "Stop All failed: noodle server unreachable", 0);
  });

  it("stays quiet when the command succeeds", () => {
    const send = vi.fn((_cmd, opts) =>
      opts.onSuccess({ id: "c1", action: "approve", status: "ok", at: "" }),
    );

    sendReported(send, { action: "approve", order_id: "o-1" }, "Approve");

    expect(toast).not.toHaveBeenCalled();
  });

  it("passes success and error through to the caller's callbacks", () => {
    const onSuccess = vi.fn();
    const onError = vi.fn();
    const err = new Error("boom");
    const failing = vi.fn((_cmd, opts) => opts.onError(err));
    sendReported(failing, { action: "stop-all" }, "Stop All", { onSuccess, onError });
    expect(onError).toHaveBeenCalledWith(err);

    const ack = { id: "c1", action: "stop-all", status: "ok", at: "" } as const;
    const ok = vi.fn((_cmd, opts) => opts.onSuccess(ack));
    sendReported(ok, { action: "stop-all" }, "Stop All", { onSuccess, onError });
    expect(onSuccess).toHaveBeenCalledWith(ack);
  });
});

describe("isBusy", () => {
  it("is true only for the pending command's action", () => {
    const mutation = { isPending: true, variables: { action: "merge" as const, order_id: "o-1" } };
    expect(isBusy(mutation, "merge")).toBe(true);
    expect(isBusy(mutation, "reject")).toBe(false);
    expect(isBusy({ isPending: false, variables: mutation.variables }, "merge")).toBe(false);
  });
});

describe("blockedReason", () => {
  it("returns the loop's reason only for a blocked action", async () => {
    const { blockedReason } = await import("./control-feedback");
    const review = { blocked_actions: ["approve"], blocked_reason: "order 5 is held at stage quality" };
    expect(blockedReason(review, "approve")).toBe("order 5 is held at stage quality");
    expect(blockedReason({ blocked_actions: [] }, "approve")).toBeUndefined();
    expect(blockedReason({}, "approve")).toBeUndefined();
    const gated = { blocked_actions: ["merge"], blocked_reason: "this order publishes at the end" };
    expect(blockedReason(gated, "merge")).toBe("this order publishes at the end");
    expect(blockedReason(gated, "approve")).toBeUndefined();
  });
});

describe("approveLabel and reviewNote", () => {
  it("names the next stage of the review's order", async () => {
    const { approveLabel } = await import("./control-feedback");
    const orders = [
      { id: "o-2", stages: [{ task_key: "execute" }] },
      { id: "o-1", stages: [{ task_key: "execute" }, { task_key: "quality" }, { task_key: "reflect" }] },
    ];
    expect(approveLabel(orders, { order_id: "o-1", stage_index: 0 })).toBe("Continue to quality");
    expect(approveLabel(orders, { order_id: "o-1", stage_index: 1 })).toBe("Continue to reflect");
    expect(approveLabel(orders, { order_id: "o-1", stage_index: 2 })).toBe("Approve");
    expect(approveLabel(orders, { order_id: "missing", stage_index: 0 })).toBe("Approve");
    expect(approveLabel([{ id: "o-3", stages: [{}, {}] }], { order_id: "o-3", stage_index: 0 })).toBe("Approve");
  });

  it("replaces the gated merge refusal with when the order publishes", async () => {
    const { reviewNote } = await import("./control-feedback");
    const refusal = "this order publishes at the end; approve the stage, then approve-and-publish";
    expect(reviewNote({ publish_gated: true, blocked_actions: ["merge"], blocked_reason: refusal })).toBe(
      "Publishes when the last stage completes",
    );
    expect(reviewNote({ publish_gated: true })).toBe("Publishes when the last stage completes");
    const hold = "order o-1 is held at stage adversarial-review for external review";
    expect(reviewNote({ publish_gated: true, blocked_actions: ["approve", "merge"], blocked_reason: hold })).toBe(hold);
    expect(reviewNote({ blocked_actions: ["approve"], blocked_reason: hold })).toBe(hold);
    expect(reviewNote({})).toBeUndefined();
  });
});
