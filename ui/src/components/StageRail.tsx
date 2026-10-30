import type { Stage, StageStatus } from "~/client";

const dotClass: Record<StageStatus, string> = {
  completed: "done",
  active: "active",
  merging: "active",
  pending: "pending",
  failed: "failed",
  cancelled: "skipped",
};

// StageRail lists an order's stages. A stage that has run is a button that
// opens its session through onOpenSession; a stage that has not run is a
// plain row.
export function StageRail({
  stages,
  onOpenSession,
}: {
  stages: Stage[];
  onOpenSession?: (sessionId: string) => void;
}) {
  if (stages.length === 0) {
    return null;
  }

  return (
    <div className="stage-rail">
      {stages.map((stage, i) => {
        const isActive = stage.status === "active";
        const stageKey =
          stage.session_id ||
          stage.task_key ||
          `${stage.skill || "stage"}:${stage.provider}:${stage.model}:${stage.group ?? "none"}`;
        const className = `stage-item ${isActive ? "current" : ""}`;
        const style = {
          animation: "fade-in 0.15s ease-out both",
          animationDelay: `${i * 30}ms`,
        };
        const content = (
          <>
            <div className={`stage-dot ${dotClass[stage.status] || "pending"}`} />
            <span>{stage.task_key || stage.skill || `Stage ${i + 1}`}</span>
          </>
        );
        const sessionId = stage.session_id;
        if (sessionId && onOpenSession) {
          return (
            <button
              type="button"
              key={stageKey}
              className={className}
              style={style}
              onClick={() => onOpenSession(sessionId)}
            >
              {content}
            </button>
          );
        }
        return (
          <div key={stageKey} className={className} style={style}>
            {content}
          </div>
        );
      })}
    </div>
  );
}
