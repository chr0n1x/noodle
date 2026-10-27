import { useEffect, useRef, useState } from "react";
import type { ControlAck, ControlCommand, EventLine } from "~/client";
import { sendReported } from "./control-feedback";

type Send = (
  cmd: ControlCommand,
  opts: { onSuccess: (ack: ControlAck) => void; onError: (err: Error) => void },
) => void;

export interface PendingSteer {
  key: number;
  target: string;
  prompt: string;
  state: "pending" | "delivered" | "error";
  error?: string;
  // Echoes of the same prompt already in the feed when it was sent.
  baseline: number;
}

// The loop records a delivered steer as a "User" action event whose body is the prompt.
function echoCount(events: EventLine[], prompt: string): number {
  return events.filter((e) => e.label === "User" && e.body.trim() === prompt).length;
}

/**
 * Tracks steers sent to `target` until the loop echoes them into `events`.
 * A failed steer stays with its error message.
 */
export function usePendingSteers(send: Send, target: string | undefined, events: EventLine[]) {
  const [steers, setSteers] = useState<PendingSteer[]>([]);
  const nextKey = useRef(0);
  const eventsRef = useRef(events);
  eventsRef.current = events;

  const isEchoed = (s: PendingSteer) =>
    s.state === "delivered" && echoCount(events, s.prompt) > s.baseline;
  const visible = steers.filter((s) => s.target === target && !isEchoed(s));
  const echoed = steers.some(isEchoed);

  useEffect(() => {
    if (echoed) {
      setSteers((prev) =>
        prev.filter(
          (s) => !(s.state === "delivered" && echoCount(eventsRef.current, s.prompt) > s.baseline),
        ),
      );
    }
  }, [echoed]);

  function update(key: number, patch: Partial<PendingSteer>) {
    setSteers((prev) => prev.map((s) => (s.key === key ? { ...s, ...patch } : s)));
  }

  function submit(steerTarget: string, prompt: string) {
    const key = nextKey.current++;
    setSteers((prev) => [
      ...prev,
      {
        key,
        target: steerTarget,
        prompt,
        state: "pending",
        baseline: echoCount(eventsRef.current, prompt),
      },
    ]);
    sendReported(send, { action: "steer", target: steerTarget, prompt }, "Send", {
      onSuccess: () => update(key, { state: "delivered" }),
      onError: (err) => update(key, { state: "error", error: err.message }),
    });
  }

  return { steers: visible, submit };
}

const STATE_TEXT: Record<PendingSteer["state"], string> = {
  pending: "sending…",
  delivered: "delivered",
  error: "failed",
};

export function PendingSteers({ steers }: { steers: PendingSteer[] }) {
  return (
    <>
      {steers.map((s) => (
        <div
          key={s.key}
          className="message-row from-user pending-steer"
          data-testid="pending-steer"
          data-state={s.state}
          aria-busy={s.state === "pending" || undefined}
        >
          <div className="msg-meta">
            <span className="msg-badge badge-user">User</span>
            <span>{STATE_TEXT[s.state]}</span>
          </div>
          <div className="msg-body">{s.prompt}</div>
          {s.state === "error" && (
            <div className="msg-body" style={{ color: "var(--color-red)" }}>
              {s.error}
            </div>
          )}
        </div>
      ))}
    </>
  );
}
