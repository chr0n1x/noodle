import { useState } from "react";
import { useSendControl, useSuspenseSnapshot } from "~/client";
import type { PendingReviewItem } from "~/client";
import { ApproveButton } from "./ApproveButton";
import { ReviewReason } from "./ReviewReason";
import { ControlResult } from "./ControlResult";
import { approveLabel, blockedReason, isBusy, requestChangesWarning, reviewNote, sendReported } from "./control-feedback";

export function ReviewBanner({ review }: { review: PendingReviewItem }) {
  const control = useSendControl();
  const { mutate: send, isPending } = control;
  const merging = isBusy(control, "merge");
  const rejecting = isBusy(control, "reject");
  const requesting = isBusy(control, "request-changes");
  const [showFeedback, setShowFeedback] = useState(false);
  const [feedback, setFeedback] = useState("");
  const mergeBlocked = blockedReason(review, "merge");
  const { data: snapshot } = useSuspenseSnapshot();
  const gated = review.publish_gated === true;
  const note = reviewNote(review);

  function handleMerge() {
    sendReported(send, { action: "merge", order_id: review.order_id }, "Approve & Merge");
  }

  function handleReject() {
    sendReported(send, { action: "reject", order_id: review.order_id }, "Reject");
  }

  function handleRequestChanges() {
    if (!showFeedback) {
      setShowFeedback(true);
      return;
    }
    sendReported(
      send,
      { action: "request-changes", order_id: review.order_id, prompt: feedback },
      "Request changes",
    );
    setShowFeedback(false);
    setFeedback("");
  }

  let requestChangesLabel = "REQUEST CHANGES";
  if (requesting) {
    requestChangesLabel = "SENDING…";
  } else if (showFeedback) {
    requestChangesLabel = "SEND";
  }

  return (
    <div className="border-l-2 border-accent p-4 bg-accent/5 mx-4 mb-2">
      <div className="text-xs font-display font-bold uppercase tracking-wider text-accent mb-1">
        Ready for review - decision needed
      </div>
      {review.reason && (
        <ReviewReason reason={review.reason} className="text-xs font-body text-neutral-400 mb-3" />
      )}

      {note && (
        <div data-testid="review-blocked-reason" className="text-xs font-body text-accent mb-3">
          {note}
        </div>
      )}

      {showFeedback && (
        <div className="mb-3">
          <input
            type="text"
            className="w-full px-2 py-1.5 font-body text-xs border border-border-subtle bg-transparent text-text-primary outline-none focus:border-accent"
            placeholder="What needs to change?"
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                handleRequestChanges();
              }
              if (e.key === "Escape") {
                setShowFeedback(false);
              }
            }}
            autoFocus
          />
        </div>
      )}

      <div className="flex gap-2">
        {!gated && (
          <button
            type="button"
            onClick={handleMerge}
            disabled={isPending || mergeBlocked !== undefined}
            title={mergeBlocked ? `Disabled: ${mergeBlocked}` : undefined}
            aria-busy={merging || undefined}
            className="font-body font-bold uppercase text-[10px] tracking-wider px-3 py-1.5 bg-green text-black disabled:opacity-50"
          >
            {merging ? "MERGING…" : "MERGE"}
          </button>
        )}
        <ApproveButton
          orderId={review.order_id}
          label={approveLabel(snapshot.orders ?? [], review)}
          gated={gated}
          blockedReason={blockedReason(review, "approve")}
          className={
            gated
              ? "font-body font-bold uppercase text-[10px] tracking-wider px-3 py-1.5 bg-green text-black disabled:opacity-50"
              : "font-body font-bold uppercase text-[10px] tracking-wider px-3 py-1.5 border border-green text-green"
          }
        />
        <button
          type="button"
          onClick={handleReject}
          disabled={isPending}
          aria-busy={rejecting || undefined}
          className="font-body font-bold uppercase text-[10px] tracking-wider px-3 py-1.5 bg-red text-white"
        >
          {rejecting ? "REJECTING…" : "REJECT"}
        </button>
        <button
          type="button"
          onClick={handleRequestChanges}
          disabled={isPending}
          title={requestChangesWarning(review)}
          aria-busy={requesting || undefined}
          className="font-body font-bold uppercase text-[10px] tracking-wider px-3 py-1.5 border border-accent text-accent"
        >
          {requestChangesLabel}
        </button>
      </div>
      <ControlResult
        mutation={control}
        labels={{ merge: "MERGE", reject: "REJECT", "request-changes": "REQUEST CHANGES" }}
        subject={review.order_id}
      />
    </div>
  );
}
