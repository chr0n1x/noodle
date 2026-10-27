import { useState } from "react";
import { useSuspenseSnapshot, useSendControl, useReviewDiff } from "~/client";
import type { Order, PendingReviewItem } from "~/client";
import { DiffViewer } from "./DiffViewer";
import { ApproveButton } from "./ApproveButton";
import { ReviewReason } from "./ReviewReason";
import { ReviewTitle } from "./ReviewTitle";
import { ControlResult } from "./ControlResult";
import { approveLabel, blockedReason, isBusy, requestChangesWarning, reviewNote, sendReported } from "./control-feedback";

function ReviewReport({ review }: { review: PendingReviewItem }) {
  const { data, isLoading, error } = useReviewDiff(review.order_id);

  return (
    <main className="review-report">
      <header className="feed-header">
        <ReviewTitle review={review} />
      </header>

      <section className="review-section">
        <div className="border-l-2 border-accent bg-accent/5 p-3 font-mono text-xs text-accent">
          Decision needed: choose an action in the Review Actions panel on the right.
        </div>
        {review.reason && (
          <>
            <h2 className="review-section-title">Agent Summary</h2>
            <ReviewReason
              reason={review.reason}
              className="font-mono text-xs text-[var(--color-text-tertiary)]"
            />
          </>
        )}
      </section>

      <section className="review-section">
        <h2 className="review-section-title">Generated Diff</h2>
        <DiffViewer
          diff={data?.diff ?? ""}
          stat={data?.stat ?? ""}
          isLoading={isLoading}
          error={error?.message}
        />
      </section>
    </main>
  );
}

function ReviewActions({ review, orders }: { review: PendingReviewItem; orders: Order[] }) {
  const control = useSendControl();
  const { mutate: send, isPending } = control;
  const merging = isBusy(control, "merge");
  const rejecting = isBusy(control, "reject");
  const requesting = isBusy(control, "request-changes");
  const [showFeedback, setShowFeedback] = useState(false);
  const [feedback, setFeedback] = useState("");
  const mergeBlocked = blockedReason(review, "merge");
  const gated = review.publish_gated === true;
  const note = reviewNote(review);

  function handleMerge() {
    sendReported(send, { action: "merge", order_id: review.order_id }, "Approve & Merge");
  }

  function handleRequestChanges() {
    if (!showFeedback) {
      setShowFeedback(true);
      return;
    }
    if (feedback.trim()) {
      sendReported(
        send,
        { action: "request-changes", order_id: review.order_id, prompt: feedback },
        "Request changes",
      );
      setShowFeedback(false);
      setFeedback("");
    }
  }

  function handleReject() {
    sendReported(send, { action: "reject", order_id: review.order_id }, "Reject");
  }

  let requestChangesLabel = "Request Changes";
  if (requesting) {
    requestChangesLabel = "Sending…";
  } else if (showFeedback) {
    requestChangesLabel = "Send";
  }

  return (
    <aside className="context-panel">
      <div className="context-header">Review Actions</div>

      <div className="review-actions-body">
        {review.skill && (
          <div className="review-meta-item">
            <span className="review-meta-label">Skill</span>
            <span className="review-meta-value">{review.skill}</span>
          </div>
        )}
        {review.model && (
          <div className="review-meta-item">
            <span className="review-meta-label">Model</span>
            <span className="review-meta-value">{review.model}</span>
          </div>
        )}
        {review.worktree_name && (
          <div className="review-meta-item">
            <span className="review-meta-label">Worktree</span>
            <span className="review-meta-value">{review.worktree_name}</span>
          </div>
        )}
      </div>

      <div style={{ flex: 1 }} />

      <div className="review-action-footer">
        {note && (
          <div data-testid="review-blocked-reason" className="review-meta-value">
            {note}
          </div>
        )}
        {showFeedback && (
          <input
            type="text"
            className="review-feedback-input"
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
        )}
        {!gated && (
          <button
            type="button"
            className="review-action-btn review-action-primary"
            onClick={handleMerge}
            disabled={isPending || mergeBlocked !== undefined}
            title={mergeBlocked ? `Disabled: ${mergeBlocked}` : undefined}
            aria-busy={merging || undefined}
          >
            {merging ? "Merging…" : "Approve & Merge"}
          </button>
        )}
        <ApproveButton
          orderId={review.order_id}
          label={approveLabel(orders, review)}
          gated={gated}
          className={`review-action-btn ${gated ? "review-action-primary" : "review-action-outline"}`}
          wrapperClassName="flex flex-col w-full"
          tooltipPlacement="left"
          blockedReason={blockedReason(review, "approve")}
        />
        <div className="review-action-row">
          <button
            type="button"
            className="review-action-btn review-action-outline"
            onClick={handleRequestChanges}
            disabled={isPending}
            title={requestChangesWarning(review)}
            aria-busy={requesting || undefined}
          >
            {requestChangesLabel}
          </button>
          <button
            type="button"
            className="review-action-btn review-action-danger"
            onClick={handleReject}
            disabled={isPending}
            aria-busy={rejecting || undefined}
          >
            {rejecting ? "Dismissing…" : "Dismiss Order"}
          </button>
        </div>
        <ControlResult
          mutation={control}
          labels={{ merge: "Approve & Merge", "request-changes": "Request Changes", reject: "Dismiss Order" }}
          subject={review.order_id}
        />
      </div>
    </aside>
  );
}

function EmptyReviews() {
  return (
    <>
      <main className="review-report">
        <header className="feed-header">
          <div className="feed-title">Reviews</div>
        </header>
        <div className="review-empty">
          No pending reviews. Completed sessions in supervised mode appear here for review.
        </div>
      </main>
      <aside className="context-panel">
        <div className="context-header">Review Actions</div>
      </aside>
    </>
  );
}

export function ReviewList() {
  const { data: snapshot } = useSuspenseSnapshot();
  const reviews = snapshot.pending_reviews ?? [];
  const [selectedIdx, setSelectedIdx] = useState(0);

  if (reviews.length === 0) {
    return (
      <div className="grid grid-cols-[1fr_300px] h-full overflow-hidden">
        <EmptyReviews />
      </div>
    );
  }

  const activeIdx = selectedIdx >= 0 && selectedIdx < reviews.length ? selectedIdx : 0;
  const review = reviews[activeIdx];
  if (!review) {
    return (
      <div className="grid grid-cols-[1fr_300px] h-full overflow-hidden">
        <EmptyReviews />
      </div>
    );
  }

  return (
    <div
      className={`grid grid-cols-[1fr_300px] h-full overflow-hidden ${reviews.length > 1 ? "grid-rows-[auto_minmax(0,1fr)]" : ""}`}
    >
      {reviews.length > 1 && (
        <div className="review-tab-bar" role="tablist">
          {reviews.map((r, i) => (
            <button
              key={r.order_id}
              type="button"
              role="tab"
              aria-selected={i === activeIdx}
              className={`review-tab ${i === activeIdx ? "active" : ""}`}
              onClick={() => setSelectedIdx(i)}
            >
              {[r.title || r.order_id, r.task_key].filter(Boolean).join(" · ")}
            </button>
          ))}
        </div>
      )}
      <ReviewReport review={review} />
      <ReviewActions review={review} orders={snapshot.orders ?? []} />
    </div>
  );
}
