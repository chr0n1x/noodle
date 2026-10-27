import { useEffect, useRef } from "react";
import type { ControlAck, ControlCommand } from "~/client";

type Action = ControlCommand["action"];

export interface ControlMutation {
  isSuccess?: boolean;
  isError?: boolean;
  data?: ControlAck;
  error?: Error | null;
  variables?: Pick<ControlCommand, "action">;
  reset?: () => void;
}

/**
 * Shows the outcome of the last control command next to the buttons that can
 * send it. `labels` maps each action of the button group to its button label;
 * the result renders only when the last command was one of those actions.
 * It stays until dismissed, the next command, or a change of `subject`.
 */
export function ControlResult({
  mutation,
  labels,
  subject,
}: {
  mutation: ControlMutation;
  labels: Partial<Record<Action, string>>;
  subject?: string;
}) {
  const lastSubject = useRef(subject);
  const reset = mutation.reset;
  useEffect(() => {
    if (lastSubject.current !== subject) {
      lastSubject.current = subject;
      reset?.();
    }
  }, [subject, reset]);

  const action = mutation.variables?.action;
  const label = action ? labels[action] : undefined;
  if (!label || !(mutation.isSuccess || mutation.isError)) {
    return null;
  }
  const failed = Boolean(mutation.isError);
  const message = failed ? mutation.error?.message || "unknown error" : mutation.data?.message || "ok";

  return (
    <div
      className={`control-result ${failed ? "control-result-error" : "control-result-success"}`}
      role={failed ? "alert" : "status"}
      data-action={action}
    >
      <span className="control-result-text">
        {label}: {message}
      </span>
      <button
        type="button"
        className="control-result-dismiss"
        aria-label="Dismiss result"
        onClick={() => reset?.()}
      >
        Dismiss
      </button>
    </div>
  );
}
