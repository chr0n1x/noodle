import { useSyncExternalStore } from "react";

type ToastKind = "info" | "success" | "error";

interface Toast {
  id: number;
  kind: ToastKind;
  text: string;
}

let toasts: Toast[] = [];
let nextId = 1;
const listeners = new Set<() => void>();

function emit() {
  for (const fn of listeners) {
    fn();
  }
}

export function dismissToast(id: number) {
  toasts = toasts.filter((t) => t.id !== id);
  emit();
}

export function toast(kind: ToastKind, text: string, ttlMs = 8000): number {
  const id = nextId++;
  toasts = [...toasts, { id, kind, text }];
  emit();
  if (ttlMs > 0) {
    setTimeout(() => dismissToast(id), ttlMs);
  }
  return id;
}

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

const URL_RE = /(https?:\/\/\S+)/;

function ToastText({ text }: { text: string }) {
  const match = URL_RE.exec(text);
  if (!match) {
    return text;
  }
  const url = match[1] ?? "";
  return (
    <>
      {text.slice(0, match.index)}
      <a href={url} target="_blank" rel="noreferrer" className="underline">
        {url}
      </a>
      {text.slice(match.index + url.length)}
    </>
  );
}

const KIND_CLASS: Record<ToastKind, string> = {
  info: "border-accent text-accent",
  success: "border-green text-green",
  error: "border-red text-red",
};

export function Toaster() {
  const items = useSyncExternalStore(subscribe, () => toasts);
  return (
    <div className="fixed bottom-4 right-4 z-[300] flex flex-col gap-2 max-w-md">
      {items.map((t) => (
        <div
          key={t.id}
          role="alert"
          className={`border-l-2 bg-bg-depth px-3 py-2 font-mono text-xs shadow-poster-sm flex gap-3 items-start ${KIND_CLASS[t.kind]}`}
        >
          <span className="flex-1 break-words">
            <ToastText text={t.text} />
          </span>
          <button type="button" aria-label="Dismiss" onClick={() => dismissToast(t.id)}>
            x
          </button>
        </div>
      ))}
    </div>
  );
}
