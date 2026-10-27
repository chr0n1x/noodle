import { useId } from "react";
import { useSendControl } from "~/client";
import type { Order } from "~/client";
import { Tooltip } from "./Tooltip";
import { ControlResult } from "./ControlResult";
import { isBusy, sendReported } from "./control-feedback";
import { publishMessage } from "./publish-message";

const LABEL = "Approve & Publish";

/**
 * Approve-and-publish for an order; renders only for a gated order whose stages
 * have all completed (status awaiting-publish).
 */
export function PublishButton({ order }: { order: Order }) {
  return order.publish_gated && order.status === "awaiting-publish" ? (
    <GatedPublishButton order={order} />
  ) : null;
}

function GatedPublishButton({ order }: { order: Order }) {
  const control = useSendControl();
  const { mutate: send, isPending } = control;
  const descriptionId = useId();
  const blockedReason = order.publish_blocked_reason || undefined;
  const description = publishMessage(order);

  return (
    <div className="pl-9 pb-1" data-loop-reason={blockedReason}>
      <Tooltip content={description} multiline placement="right">
        <button
          type="button"
          className="review-action-btn review-action-outline"
          onClick={() =>
            sendReported(send, { action: "approve-and-publish", order_id: order.id }, LABEL)
          }
          disabled={isPending || blockedReason !== undefined}
          aria-describedby={descriptionId}
          aria-busy={isBusy(control, "approve-and-publish") || undefined}
        >
          {LABEL}
        </button>
        <span id={descriptionId} hidden>
          {description}
        </span>
      </Tooltip>
      <ControlResult
        mutation={control}
        labels={{ "approve-and-publish": LABEL }}
        subject={order.id}
      />
      {order.publish_error && (
        <div className="font-mono text-[10px] text-red break-words">{order.publish_error}</div>
      )}
    </div>
  );
}
