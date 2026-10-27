import { useId } from "react";
import { useSendControl } from "~/client";
import { Tooltip } from "./Tooltip";
import { ControlResult } from "./ControlResult";
import { isBusy, sendReported } from "./control-feedback";

const EXPLANATION =
  "Approves this stage and moves the order to its next stage without merging or opening a PR. The stage branch is kept, and later stages start from it, so a later review can decide whether to merge or open a draft PR.";

const GATED_EXPLANATION =
  "Approves this stage and starts the next one. The branch is kept. The order publishes, or closes with nothing to publish, after its last stage.";

export function ApproveButton({
  orderId,
  className,
  wrapperClassName,
  tooltipPlacement,
  label = "Approve",
  blockedReason,
  gated = false,
}: {
  orderId: string;
  className?: string;
  wrapperClassName?: string;
  tooltipPlacement?: "top" | "left";
  label?: string;
  blockedReason?: string;
  gated?: boolean;
}) {
  const control = useSendControl();
  const { mutate: send, isPending } = control;
  const descriptionId = useId();
  const description = blockedReason ? `Disabled: ${blockedReason}` : gated ? GATED_EXPLANATION : EXPLANATION;

  return (
    <>
    <Tooltip content={description} multiline wrapperClassName={wrapperClassName}
      placement={tooltipPlacement}
    >
      <button
        type="button"
        className={className}
        onClick={() => sendReported(send, { action: "approve", order_id: orderId }, label)}
        disabled={isPending || blockedReason !== undefined}
        aria-describedby={descriptionId}
        aria-busy={isBusy(control, "approve") || undefined}
      >
        {label}
      </button>
      <span id={descriptionId} hidden>
        {description}
      </span>
    </Tooltip>
    <ControlResult mutation={control} labels={{ approve: label }} subject={orderId} />
    </>
  );
}
