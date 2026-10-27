import { describe, expect, it } from "vitest";
import { publishMessage } from "./publish-message";
import { buildOrder } from "../test-utils";
import type { StageStatus } from "~/client/enums";

const stage = (task_key: string, status: StageStatus) => ({ task_key, status, provider: "claude", model: "m" });

describe("publishMessage", () => {
  it("lists the stages left with names and statuses", () => {
    const order = buildOrder({
      id: "1",
      status: "active",
      publish_gated: true,
      publish_blocked_reason: "approve-and-publish refused: raw",
      stages: [stage("execute", "active"), stage("quality", "pending"), stage("adversarial-review", "pending")],
    });
    expect(publishMessage(order)).toBe("Not ready: 3 stages left (execute running, quality, adversarial-review)");
  });

  it("uses the singular and skips completed stages", () => {
    const order = buildOrder({
      id: "1",
      status: "active",
      publish_gated: true,
      publish_blocked_reason: "raw",
      stages: [stage("execute", "completed"), stage("quality", "failed")],
    });
    expect(publishMessage(order)).toBe("Not ready: 1 stage left (quality failed)");
  });

  it("says not ready when blocked with every stage completed", () => {
    const order = buildOrder({ id: "1", status: "active", publish_gated: true, publish_blocked_reason: "raw", stages: [stage("execute", "completed")] });
    expect(publishMessage(order)).toBe("Not ready to publish yet");
  });

  it("explains the action when ready", () => {
    const order = buildOrder({ id: "1", status: "awaiting-publish", publish_gated: true, stages: [stage("execute", "completed")] });
    expect(publishMessage(order)).toBe("Ready: publishes all of this order's kept work and completes the order.");
  });

  it("shows the first line of a publish error", () => {
    const order = buildOrder({ id: "1", status: "awaiting-publish", publish_gated: true, publish_error: "merge 1-1-review: rebase conflict\nCONFLICT in a.go" });
    expect(publishMessage(order)).toBe("Last publish failed: merge 1-1-review: rebase conflict");
  });
});
