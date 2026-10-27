import type { ControlAck } from "./types";
import { jsonBody } from "./api";

/**
 * Polls until the loop has processed command `id`, returning its ack.
 * The POST ack only confirms the command was queued.
 */
export async function waitForControlAck(
  id: string,
  intervalMs = 1000,
  deadline = Date.now() + 10 * 60 * 1000,
): Promise<ControlAck> {
  const res = await fetch(`/api/control/${encodeURIComponent(id)}`);
  if (res.ok) {
    return jsonBody<ControlAck>(res);
  }
  if (res.status !== 404) {
    const body = await res.text().catch(() => "");
    throw new Error(`waitForControlAck: ${res.status}${body ? ` — ${body.trim()}` : ""}`);
  }
  if (Date.now() >= deadline) {
    throw new Error("timed out waiting for the loop to process the command");
  }
  await new Promise((resolve) => {
    setTimeout(resolve, intervalMs);
  });
  return waitForControlAck(id, intervalMs, deadline);
}
