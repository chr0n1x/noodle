import { useCallback, useMemo, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useSuspenseSnapshot, useSessionEvents, useSendControl, formatCost } from "~/client";
import type { Session } from "~/client";
import { StreamingDelta } from "./StreamingDelta";
import { groupConsecutiveTools } from "./group-tools";
import { VirtualizedFeed } from "./VirtualizedFeed";
import { isBusy, sendReported } from "./control-feedback";
import { ControlResult } from "./ControlResult";
import { PendingSteers, usePendingSteers } from "./PendingSteers";

function findSchedulerSession(sessions: Session[]): Session | undefined {
  return sessions.find((s) => s.task_key?.toLowerCase().trim() === "schedule");
}

function isBootstrapScheduleSession(session: Session): boolean {
  return session.id.toLowerCase().startsWith("bootstrap-schedule");
}

function isScheduleOnlyOrder(order: { stages: { task_key?: string }[] }): boolean {
  return (
    order.stages.length > 0 &&
    order.stages.every((s) => s.task_key?.toLowerCase().trim() === "schedule")
  );
}

export function SchedulerFeed() {
  const { data: snapshot } = useSuspenseSnapshot();
  const control = useSendControl();
  const { mutate: send, isPending } = control;
  const [input, setInput] = useState("");

  const schedulerSession = findSchedulerSession(snapshot.sessions);
  const activeSchedulerSession = snapshot.active.find(
    (s) => s.task_key?.toLowerCase().trim() === "schedule" && s.status === "running",
  );
  const bootstrapScheduleSession = snapshot.sessions.find(
    (s) => s.status === "running" && isBootstrapScheduleSession(s),
  );
  const isBootstrappingSchedule = Boolean(bootstrapScheduleSession);
  const isBootstrappingSchedulePending =
    snapshot.loop_state === "running" &&
    !schedulerSession &&
    snapshot.orders.some(isScheduleOnlyOrder);
  const isSchedulerThinking =
    snapshot.loop_state === "running" && Boolean(activeSchedulerSession?.current_action?.trim());
  const initialEvents = schedulerSession?.id
    ? snapshot.events_by_session[schedulerSession.id]
    : undefined;
  const { data: events = [] } = useSessionEvents(schedulerSession?.id, initialEvents);
  const pendingSteers = usePendingSteers(send, "schedule", events);

  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const resizeTextarea = useCallback(() => {
    const el = textareaRef.current;
    if (!el) {
      return;
    }
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }, []);

  const items = useMemo(() => groupConsecutiveTools(events), [events]);

  let emptyMessage: React.ReactNode | undefined;
  if (events.length === 0) {
    if (isBootstrappingSchedule || isBootstrappingSchedulePending) {
      emptyMessage = "Bootstrapping schedule skill. This usually takes a few seconds.";
    } else if (!schedulerSession) {
      emptyMessage = (
        <>
          {"The scheduler starts when you run "}
          <code style={{ fontFamily: "var(--font-mono)" }}>noodle start</code>
          {" and have skills installed."}
          <br />
          <Link to="/onboarding" style={{ color: "var(--color-accent)" }}>
            Learn how the loop works
          </Link>
        </>
      );
    }
  }

  function handleSubmit() {
    const prompt = input.trim();
    if (!prompt) {
      return;
    }
    pendingSteers.submit("schedule", prompt);
    setInput("");
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  }

  function handleStopAll() {
    sendReported(send, { action: "stop-all" }, "Stop All");
  }

  function handleStop() {
    if (schedulerSession) {
      sendReported(send, { action: "stop", name: schedulerSession.id }, "Stop");
    }
  }

  function renderInputLabel() {
    if (isBootstrappingSchedule || isBootstrappingSchedulePending) {
      return "Bootstrapping schedule skill...";
    }
    if (isSchedulerThinking) {
      return (
        <>
          <span className="thinking-dots">
            <span className="thinking-dot" />
            <span className="thinking-dot" />
            <span className="thinking-dot" />
          </span>
          Thinking…
        </>
      );
    }
    return "Talk to the scheduler...";
  }

  return (
    <>
      <header className="feed-header">
        <div className="feed-title">
          Scheduler
          <span className="feed-badge badge-task">{snapshot.loop_state}</span>
          {(isBootstrappingSchedule || isBootstrappingSchedulePending) && (
            <span className="feed-badge badge-task">bootstrap</span>
          )}
          {schedulerSession && <span className="feed-badge">{schedulerSession.model}</span>}
        </div>
        <div className="feed-actions">
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: 12,
              color: "var(--color-text-tertiary)",
            }}
          >
            {formatCost(snapshot.total_cost_usd)}
          </span>
          <button
            type="button"
            className="feed-action-btn stop-btn"
            onClick={handleStopAll}
            disabled={isPending}
            aria-busy={isBusy(control, "stop-all") || undefined}
          >
            Stop All
          </button>
        </div>
      </header>

      <div className="feed-watermark">NOODLE</div>

      <VirtualizedFeed
        items={items}
        emptyMessage={emptyMessage}
        tail={
          <>
            <PendingSteers steers={pendingSteers.steers} />
            {isSchedulerThinking && schedulerSession?.id && (
              <StreamingDelta sessionId={schedulerSession.id} />
            )}
          </>
        }
      />

      <div className="input-area">
        <div className="input-label">{renderInputLabel()}</div>
        <div className="input-row">
          <div className="input-row-field">
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => {
                setInput(e.target.value);
                resizeTextarea();
              }}
              onKeyDown={handleKeyDown}
              placeholder="Enter instructions or critique..."
              rows={1}
            />
            <div className="input-hint">
              <kbd>Enter</kbd>
              <span>send</span>
              <kbd>Shift+Enter</kbd>
              <span>newline</span>
            </div>
          </div>
          <div className="input-row-actions">
            {isSchedulerThinking && (
              <button
                type="button"
                className="btn-stop"
                onClick={handleStop}
                disabled={isPending}
                aria-busy={isBusy(control, "stop") || undefined}
              >
                {isBusy(control, "stop") ? "Stopping…" : "Stop"}
              </button>
            )}
            <button
              type="button"
              className="btn-submit"
              onClick={handleSubmit}
              disabled={isPending || !input.trim()}
              aria-busy={isBusy(control, "steer") || undefined}
            >
              Send
            </button>
          </div>
        </div>
        <ControlResult
          mutation={control}
          labels={{ "stop-all": "Stop All", stop: "Stop", steer: "Send" }}
        />
      </div>
    </>
  );
}
