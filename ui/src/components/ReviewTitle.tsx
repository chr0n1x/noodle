import type { PendingReviewItem } from "~/client";

function stageLabel(taskKey?: string) {
  if (!taskKey) {
    return "Stage";
  }
  return taskKey.charAt(0).toUpperCase() + taskKey.slice(1);
}

export function ReviewTitle({ review }: { review: PendingReviewItem }) {
  return (
    <div className="feed-title">
      {stageLabel(review.task_key)} Review:{" "}
      {review.title && <span className="review-session-name">{review.title}</span>}{" "}
      <span
        className={review.title ? "text-[var(--color-text-tertiary)] text-xs" : "review-session-name"}
      >
        {review.session_id || review.order_id}
      </span>
      {review.task_key && <span className="feed-badge badge-task">{review.task_key}</span>}
    </div>
  );
}
