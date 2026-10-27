import type { Order } from "~/client";

const STATUS_WORD: Record<string, string> = {
  active: "running",
  merging: "merging",
  failed: "failed",
  cancelled: "cancelled",
};

/** One-line explanation for the Approve & Publish button, built from the snapshot order. */
export function publishMessage(order: Order): string {
  if (order.publish_error) {
    return `Last publish failed: ${order.publish_error.split("\n")[0]}`;
  }
  if (!order.publish_blocked_reason) {
    return "Ready: publishes all of this order's kept work and completes the order.";
  }
  const left = (order.stages ?? [])
    .map((stage, i) => ({ stage, i }))
    .filter(({ stage }) => stage.status !== "completed")
    .map(({ stage, i }) => {
      const name = stage.task_key || stage.skill || `stage ${i + 1}`;
      const word = STATUS_WORD[stage.status];
      return word ? `${name} ${word}` : name;
    });
  if (left.length === 0) {
    return "Not ready to publish yet";
  }
  return `Not ready: ${left.length} ${left.length === 1 ? "stage" : "stages"} left (${left.join(", ")})`;
}
