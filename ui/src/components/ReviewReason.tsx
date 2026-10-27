import { useState } from "react";

export function ReviewReason({ reason, className = "" }: { reason: string; className?: string }) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div className={className}>
      <div
        data-testid="review-reason"
        className={`whitespace-pre-wrap break-words ${expanded ? "" : "line-clamp-3"}`}
      >
        {reason}
      </div>
      <button
        type="button"
        className="mt-1 text-accent underline text-[10px] uppercase tracking-wider"
        onClick={() => setExpanded((v) => !v)}
      >
        {expanded ? "Show less" : "Show more"}
      </button>
    </div>
  );
}
