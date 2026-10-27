import { useState, useRef, useCallback, useEffect } from "react";
import type { ReactNode } from "react";
import { createPortal } from "react-dom";

const TRANSLATE = {
  top: "-translate-x-1/2 -translate-y-full",
  left: "-translate-x-full -translate-y-1/2",
  right: "-translate-y-1/2",
} as const;

export function Tooltip({
  content,
  children,
  multiline = false,
  wrapperClassName = "inline-flex",
  placement = "top",
}: {
  content: string;
  children: ReactNode;
  multiline?: boolean;
  wrapperClassName?: string;
  placement?: "top" | "left" | "right";
}) {
  const [visible, setVisible] = useState(false);
  const [position, setPosition] = useState<{ top: number; left: number }>({
    top: 0,
    left: 0,
  });
  const triggerRef = useRef<HTMLSpanElement>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout>>(null);

  useEffect(
    () => () => {
      if (timeoutRef.current !== null) {
        clearTimeout(timeoutRef.current);
      }
    },
    [],
  );

  const show = useCallback(() => {
    if (timeoutRef.current !== null) {
      clearTimeout(timeoutRef.current);
    }
    timeoutRef.current = setTimeout(() => {
      if (!triggerRef.current) {
        return;
      }
      const rect = triggerRef.current.getBoundingClientRect();
      setPosition(
        placement === "left"
          ? { top: rect.top + rect.height / 2, left: rect.left - 8 }
          : placement === "right"
            ? { top: rect.top + rect.height / 2, left: rect.right + 8 }
            : { top: rect.top - 4, left: rect.left + rect.width / 2 },
      );
      setVisible(true);
    }, 400);
  }, [placement]);

  const hide = useCallback(() => {
    if (timeoutRef.current !== null) {
      clearTimeout(timeoutRef.current);
    }
    setVisible(false);
  }, []);

  return (
    <>
      <span
        ref={triggerRef}
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
        className={wrapperClassName}
      >
        {children}
      </span>
      {visible &&
        createPortal(
          <div
            className={`fixed ${TRANSLATE[placement]} pointer-events-none z-[200] shadow-poster-sm animate-tooltip ${
              multiline
                ? "w-80 max-w-[calc(100vw-16px)] px-3 py-2 bg-bg-depth text-text-primary border border-accent font-body text-[13px] leading-relaxed font-normal whitespace-normal"
                : "px-2.5 py-1 bg-accent text-bg-0 font-mono text-xs font-semibold whitespace-nowrap"
            }`}
            style={{ top: position.top, left: position.left }}
            role="tooltip"
          >
            {content}
          </div>,
          document.body,
        )}
    </>
  );
}
