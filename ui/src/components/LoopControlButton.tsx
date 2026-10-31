import { Pause, Play } from "lucide-react";
import { useSendControl } from "~/client";
import type { LoopState } from "~/client";
import { ControlResult } from "./ControlResult";
import { isBusy, sendReported } from "./control-feedback";

const LABELS = { pause: "Pause loop", resume: "Resume loop" } as const;

/**
 * Pauses or resumes scheduling and dispatch. Running cooks keep going; a
 * paused or draining loop starts no new work until resumed.
 */
export function LoopControlButton({ loopState }: { loopState: LoopState }) {
  const control = useSendControl();
  const { mutate: send, isPending } = control;
  const halted = loopState === "paused" || loopState === "draining";
  const action = halted ? "resume" : "pause";
  const label = LABELS[action];

  return (
    <div className="loop-control">
      <button
        type="button"
        className="agent-item loop-control-btn"
        onClick={() => sendReported(send, { action }, label)}
        disabled={isPending}
        aria-pressed={halted}
        aria-busy={isBusy(control, action) || undefined}
        aria-label={label}
      >
        <div className="agent-avatar">
          {halted ? <Play size={14} strokeWidth={2} /> : <Pause size={14} strokeWidth={2} />}
        </div>
        <div className="agent-info">
          <span className="agent-name">{halted ? "Resume" : "Pause"}</span>
          <span className="agent-meta-line">{halted ? loopState.toUpperCase() : "TAKING NEW WORK"}</span>
        </div>
      </button>
      <ControlResult mutation={control} labels={LABELS} subject="loop" />
    </div>
  );
}
