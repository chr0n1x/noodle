import type { ControlAck, ControlCommand } from "~/client";
import { toast } from "./Toaster";

type Send = (
  cmd: ControlCommand,
  opts: { onSuccess: (ack: ControlAck) => void; onError: (err: Error) => void },
) => void;

interface Callbacks {
  onSuccess?: (ack: ControlAck) => void;
  onError?: (err: Error) => void;
}

/** Sends `cmd` and reports a failure from the server or the loop as a toast. */
export function sendReported(
  send: Send,
  cmd: ControlCommand,
  label: string,
  callbacks: Callbacks = {},
): void {
  send(cmd, {
    onSuccess: (ack) => callbacks.onSuccess?.(ack),
    onError: (err) => {
      toast("error", `${label} failed: ${err.message}`, 0);
      callbacks.onError?.(err);
    },
  });
}

/** True while the control mutation is running the command with this action. */
export function isBusy(
  mutation: { isPending: boolean; variables?: Pick<ControlCommand, "action"> },
  action: ControlCommand["action"],
): boolean {
  return mutation.isPending && mutation.variables?.action === action;
}

/**
 * blockedReason returns the loop's reason when it refuses action for this
 * review, read from the pending review's blocked_actions and blocked_reason.
 */
export function blockedReason(
  review: { blocked_actions?: string[]; blocked_reason?: string },
  action: "approve" | "merge",
): string | undefined {
  if (!review.blocked_actions?.includes(action)) {
    return undefined;
  }
  return review.blocked_reason || `${action} is blocked by the loop`;
}

/** Tooltip for Request Changes; a gated config re-runs the stage instead of failing the order. */
export function requestChangesWarning(review: { publish_gated?: boolean }): string {
  return review.publish_gated
    ? "Re-runs this stage in the same worktree with your feedback. Fails the order after the rework limit."
    : "Fails the order. The worktree and its uncommitted work are kept.";
}

/** Label for approving a review: the next stage of its order, or plain Approve at the last stage. */
export function approveLabel(
  orders: { id: string; stages: { task_key?: string }[] }[],
  review: { order_id: string; stage_index: number },
): string {
  const next = orders.find((o) => o.id === review.order_id)?.stages[review.stage_index + 1]?.task_key;
  return next ? `Continue to ${next}` : "Approve";
}

export const GATED_PUBLISH_NOTE = "Publishes when the last stage completes";

/**
 * Note shown above the review actions: the loop's reason when approve is
 * blocked, otherwise when a gated order publishes.
 */
export function reviewNote(review: {
  publish_gated?: boolean;
  blocked_actions?: string[];
  blocked_reason?: string;
}): string | undefined {
  if (review.blocked_actions?.includes("approve")) {
    return review.blocked_reason || "approve is blocked by the loop";
  }
  if (review.publish_gated) {
    return GATED_PUBLISH_NOTE;
  }
  return review.blocked_reason || undefined;
}
