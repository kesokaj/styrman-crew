// Shared UI primitives (v1.3 design system): one Button/Input/Dialog family
// so the M3 treatment lives in exactly one file, plus the Confirm/Prompt
// dialogs that replace every native confirm()/prompt() (operator request).

import {
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
  type ButtonHTMLAttributes,
  type ComponentProps,
  type ReactNode,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import { dismissToast, getToasts, subscribeToasts } from "../lib/toast";
import { XIcon } from "./icons";

type ButtonVariant = "filled" | "tonal" | "text" | "danger" | "danger-text";

const buttonClasses: Record<ButtonVariant, string> = {
  filled:
    "bg-accent text-on-accent hover:brightness-110 disabled:opacity-40 font-medium",
  tonal:
    "bg-accent-soft text-on-accent-soft hover:brightness-105 disabled:opacity-40 font-medium",
  text: "text-text-dim hover:bg-bg-hover hover:text-text disabled:opacity-40",
  danger:
    "bg-danger text-on-danger hover:brightness-110 disabled:opacity-40 font-medium",
  "danger-text": "text-danger hover:bg-danger-soft disabled:opacity-40",
};

export function Button({
  variant = "text",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant }) {
  return (
    <button
      className={`inline-flex items-center justify-center gap-1.5 rounded-full px-3.5 py-1.5 text-xs transition-colors ${buttonClasses[variant]} ${className}`}
      {...props}
    />
  );
}

/** Square icon-only button with a real (28px) hit target. */
export function IconButton({
  label,
  className = "",
  danger = false,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  danger?: boolean;
}) {
  return (
    <button
      aria-label={label}
      title={label}
      className={`inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-text-dim transition-colors ${
        danger
          ? "hover:bg-danger-soft hover:text-danger"
          : "hover:bg-bg-hover hover:text-text"
      } ${className}`}
      {...props}
    />
  );
}

export function TextInput({
  className = "",
  ...props
}: ComponentProps<"input">) {
  return (
    <input
      className={`w-full rounded-lg border border-border bg-bg px-3 py-1.5 text-sm outline-none transition-colors focus:border-accent ${className}`}
      {...props}
    />
  );
}

// Escape must close only the TOPMOST dialog when dialogs stack (e.g. the
// file viewer's discard confirm on top of the viewer).
const dialogStack: symbol[] = [];

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * useModalA11y — the shared modal contract (v1.14.0 polish batch): topmost-
 * Escape closes, Tab is trapped inside the panel, focus moves into the panel
 * on open and RETURNS to the opener on close. Used by the Dialog shell and by
 * the custom-layout overlays (credential viewer, add-credential modal, new-
 * project dialog) so every modal behaves identically for keyboard users.
 */
export function useModalA11y(
  panelRef: RefObject<HTMLDivElement | null>,
  onClose: () => void,
) {
  // onClose through a ref: the mount effect below must run EXACTLY ONCE.
  // Keying it on [onClose] (every caller passes a fresh closure per render)
  // re-ran the whole effect per keystroke — yanking focus back to the first
  // control (the file editor was unusable) and re-pushing this modal ABOVE a
  // nested ConfirmDialog on the stack, deadlocking Escape (2026-08-30
  // review, both blocking).
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  });
  useEffect(() => {
    const me = Symbol("dialog");
    dialogStack.push(me);
    const opener = document.activeElement as HTMLElement | null;
    const panel = panelRef.current;
    if (panel) {
      panel.tabIndex = -1;
      // React's autoFocus prop focuses imperatively at mount and never
      // writes the attribute, so [autofocus] queries can't see it (audit):
      // if something inside the panel ALREADY has focus (React autoFocus,
      // an input's own autoFocus), leave it alone; else focus the first
      // focusable, else the panel.
      if (!panel.contains(document.activeElement)) {
        (panel.querySelector<HTMLElement>(FOCUSABLE) ?? panel).focus();
      }
    }
    const onKey = (e: KeyboardEvent) => {
      if (dialogStack[dialogStack.length - 1] !== me) return;
      if (e.key === "Escape") {
        onCloseRef.current();
        return;
      }
      if (e.key === "Tab" && panelRef.current) {
        // Leave Tab alone for widgets that consume it themselves (the
        // embedded terminal: xterm handles + cancels its own keydowns).
        if ((e.target as HTMLElement | null)?.closest?.(".xterm")) return;
        const items = [
          ...panelRef.current.querySelectorAll<HTMLElement>(FOCUSABLE),
        ];
        const first = items[0];
        const last = items[items.length - 1];
        if (!first || !last) return; // empty list — nothing to trap
        const active = document.activeElement;
        if (e.shiftKey && (active === first || active === panelRef.current)) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && active === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    addEventListener("keydown", onKey);
    return () => {
      dialogStack.splice(dialogStack.indexOf(me), 1);
      removeEventListener("keydown", onKey);
      opener?.focus?.();
    };
    // Mount-only BY DESIGN (see onCloseRef above).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}

/**
 * Modal shell: scrim + rounded panel; Escape or scrim click closes. Rendered
 * through a portal to document.body — ancestors with a CSS transform (the
 * sidebar drawer) would otherwise become the containing block for
 * position:fixed and shrink the dialog into themselves.
 */
export function Dialog({
  title,
  onClose,
  children,
  width = "w-96",
  danger = false,
}: {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
  width?: string;
  danger?: boolean;
}) {
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  // Full modal contract: topmost-Escape, focus trap, focus restore.
  useModalA11y(panelRef, onClose);
  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={(e) => {
        // Contain the click: portals still bubble through the REACT tree, so
        // without this a scrim click would also hit an ancestor's own
        // backdrop handler (file viewer) and instantly reopen the dialog.
        e.stopPropagation();
        onClose();
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={`${width} max-w-full rounded-3xl bg-bg-raised p-5 shadow-2xl`}
        onClick={(e) => e.stopPropagation()}
      >
        <h2
          id={titleId}
          className={`mb-3 text-sm font-semibold ${danger ? "text-danger" : ""}`}
        >
          {title}
        </h2>
        {children}
      </div>
    </div>,
    document.body,
  );
}

/** Replaces confirm(): a question with explicit actions. */
export function ConfirmDialog({
  title,
  body,
  confirmLabel = "Confirm",
  danger = false,
  onConfirm,
  onCancel,
  onClose,
}: {
  title: ReactNode;
  body?: ReactNode;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  /**
   * Dismissal that is specifically a CANCEL (backdrop, Escape, the Cancel
   * button) — never fired after onConfirm. onClose alone cannot express this:
   * it runs after confirming too, so a caller that used it to "undo" a pending
   * choice silently reverted the confirmed action (EditorPane, 2026-09-01).
   */
  onCancel?: () => void;
  onClose: () => void;
}) {
  return (
    <Dialog
      title={title}
      onClose={() => {
        onCancel?.();
        onClose();
      }}
      danger={danger}
    >
      {body && <div className="mb-4 text-xs text-text-dim">{body}</div>}
      <div className="flex justify-end gap-2">
        <Button
          onClick={() => {
            onCancel?.();
            onClose();
          }}
        >
          Cancel
        </Button>
        <Button
          variant={danger ? "danger" : "filled"}
          autoFocus
          onClick={() => {
            onConfirm();
            onClose();
          }}
        >
          {confirmLabel}
        </Button>
      </div>
    </Dialog>
  );
}

/** Replaces prompt(): one text field with validation. */
export function PromptDialog({
  title,
  label,
  initial = "",
  submitLabel = "Save",
  validate,
  onSubmit,
  onClose,
}: {
  title: ReactNode;
  label?: ReactNode;
  initial?: string;
  submitLabel?: string;
  /** Return an error message to block submission, or undefined to allow. */
  validate?: (value: string) => string | undefined;
  onSubmit: (value: string) => void;
  onClose: () => void;
}) {
  const [value, setValue] = useState(initial);
  const [error, setError] = useState<string>();
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    ref.current?.focus(); // select() alone isn't specified to focus
    ref.current?.select();
  }, []);
  const submit = () => {
    const v = value.trim();
    if (!v) return; // Enter must not bypass the disabled Save button
    const err = validate?.(v);
    if (err) {
      setError(err);
      return;
    }
    onSubmit(v);
    onClose();
  };
  return (
    <Dialog title={title} onClose={onClose}>
      {label && <p className="mb-2 text-xs text-text-dim">{label}</p>}
      <TextInput
        ref={ref}
        value={value}
        onChange={(e) => {
          setValue(e.target.value);
          setError(undefined);
        }}
        onKeyDown={(e) => e.key === "Enter" && submit()}
      />
      {error && <p className="mt-2 text-xs text-danger">{error}</p>}
      <div className="mt-4 flex justify-end gap-2">
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="filled" onClick={submit} disabled={!value.trim()}>
          {submitLabel}
        </Button>
      </div>
    </Dialog>
  );
}

/** Renders the toast queue (lib/toast.ts) bottom-center; replaces alert(). */
export function ToastHost() {
  const toasts = useSyncExternalStore(subscribeToasts, getToasts, getToasts);
  if (toasts.length === 0) return null;
  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-4 z-[60] flex flex-col items-center gap-2 px-4">
      {toasts.map((t) => (
        <div
          key={t.id}
          role="status"
          className={`pointer-events-auto flex max-w-md items-center gap-2 rounded-2xl px-4 py-2 text-xs shadow-lg ${
            t.kind === "error"
              ? "bg-danger-soft text-danger"
              : t.kind === "success"
                ? "bg-ok-soft text-ok"
                : "bg-bg-active text-text"
          }`}
        >
          <span className="min-w-0 break-words">{t.text}</span>
          <button
            aria-label="Dismiss"
            className="shrink-0 opacity-60 hover:opacity-100"
            onClick={() => dismissToast(t.id)}
          >
            <XIcon className="h-3.5 w-3.5" />
          </button>
        </div>
      ))}
    </div>
  );
}
