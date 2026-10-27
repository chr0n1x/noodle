import type { Order, Snapshot } from "~/client";

/** Returns url when it is an absolute http(s) link with a host, else undefined. */
export function safeOrderUrl(url: string | undefined): string | undefined {
  if (!url) {
    return undefined;
  }
  try {
    const parsed = new URL(url);
    if ((parsed.protocol === "http:" || parsed.protocol === "https:") && parsed.host !== "") {
      return url;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

/** A short label for a source link: its host and last path segment. */
export function sourceLinkLabel(url: string): string {
  const parsed = new URL(url);
  const last = parsed.pathname.split("/").toReversed().find(Boolean);
  return last ? `${parsed.host} ${decodeURIComponent(last)}` : parsed.host;
}

/** A one-line link from an order to its source, or nothing without a usable url. */
export function OrderSourceLink({ url }: { url?: string }) {
  const href = safeOrderUrl(url);
  if (!href) {
    return null;
  }
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="block pl-9 pb-1 font-mono text-[10px] text-accent underline"
      title={href}
    >
      {sourceLinkLabel(href)}
    </a>
  );
}

/** The order whose stage ran the session, if any. */
export function orderForSession(snapshot: Snapshot, sessionId: string): Order | undefined {
  return snapshot.orders.find((order) => order.stages.some((s) => s.session_id === sessionId));
}

/**
 * An order or cook title: a link to the order's source when it has a usable
 * url, plain text otherwise. The link opens in a new tab and does not trigger
 * the click handler of the row it sits in.
 */
export function OrderTitle({ title, url }: { title: string; url?: string }) {
  const href = safeOrderUrl(url);
  if (!href) {
    return title;
  }
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="underline decoration-dotted underline-offset-2 hover:text-accent"
      onClick={(e) => e.stopPropagation()}
    >
      {title}
    </a>
  );
}
