import { ReactNode, useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { XIcon } from "@/icons";
import { useEscapeClose } from "../hooks/useEscapeClose";
import { Button } from "./Button";

type Common = {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  // When true, backdrop click + close button are disabled (e.g. while a
  // mutation is pending). Escape is also suppressed.
  busy?: boolean;
};

export function Modal({
  open,
  onClose,
  title,
  children,
  busy = false,
}: Common) {
  return (
    <DialogShell open={open} onClose={onClose} busy={busy} flavor="modal">
      <div className="relative w-[520px] max-w-[calc(100vw-2rem)] rounded border border-border bg-bg shadow-lg">
        <DialogHeader title={title} onClose={onClose} busy={busy} />
        {children}
      </div>
    </DialogShell>
  );
}

export function Drawer({
  open,
  onClose,
  title,
  children,
  busy = false,
}: Common) {
  return (
    <DialogShell open={open} onClose={onClose} busy={busy} flavor="drawer">
      <aside className="flex w-[480px] flex-col border-l border-border bg-bg">
        <DialogHeader title={title} onClose={onClose} busy={busy} />
        {children}
      </aside>
    </DialogShell>
  );
}

function DialogShell({
  open,
  onClose,
  busy,
  flavor,
  children,
}: {
  open: boolean;
  onClose: () => void;
  busy: boolean;
  flavor: "modal" | "drawer";
  children: ReactNode;
}) {
  useEscapeClose(open && !busy, onClose);
  useScrollLock(open);
  useRestoreFocus(open);

  if (!open) return null;

  const layoutClass =
    flavor === "modal"
      ? "fixed inset-0 z-40 flex items-center justify-center"
      : "fixed inset-0 z-40 flex";
  const backdrop =
    flavor === "modal" ? (
      <div
        className="absolute inset-0 bg-black/50"
        onClick={busy ? undefined : onClose}
      />
    ) : (
      <div
        className="flex-1 bg-black/50"
        onClick={busy ? undefined : onClose}
      />
    );

  return createPortal(
    <div className={layoutClass} role="dialog" aria-modal="true">
      {backdrop}
      {children}
    </div>,
    document.body,
  );
}

function DialogHeader({
  title,
  onClose,
  busy,
}: {
  title: string;
  onClose: () => void;
  busy: boolean;
}) {
  return (
    <header className="flex items-center justify-between border-b border-border px-3 py-2">
      <h2 className="text-sm font-semibold">{title}</h2>
      <Button
        variant="neutral"
        size="icon"
        onClick={onClose}
        disabled={busy}
        aria-label="Close"
      >
        <XIcon />
      </Button>
    </header>
  );
}

function useScrollLock(active: boolean) {
  useEffect(() => {
    if (!active) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [active]);
}

function useRestoreFocus(active: boolean) {
  const previousRef = useRef<Element | null>(null);
  useEffect(() => {
    if (!active) return;
    previousRef.current = document.activeElement;
    return () => {
      const el = previousRef.current;
      if (el instanceof HTMLElement) el.focus();
    };
  }, [active]);
}
