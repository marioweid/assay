import { useEffect, useRef } from "react";
import type { KeyboardEvent, ReactNode } from "react";

type ModalProps = {
  children: ReactNode;
  label: string;
  onClose: () => void;
  role?: "alertdialog" | "dialog";
};

export function Modal({ children, label, onClose, role = "dialog" }: ModalProps) {
  const modal = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const previousFocus =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    modal.current?.querySelector<HTMLElement>("input, select, button")?.focus();
    return () => previousFocus?.focus();
  }, []);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>): void {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== "Tab" || modal.current === null) return;
    const focusable = Array.from(
      modal.current.querySelectorAll<HTMLElement>("button, input, select"),
    ).filter((element) => !element.hasAttribute("disabled"));
    const first = focusable[0];
    const last = focusable.at(-1);
    if (first === undefined || last === undefined) return;
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  return (
    <div
      aria-label={label}
      aria-modal="true"
      className="fixed inset-0 z-20 grid place-items-center bg-black/40 p-4"
      onKeyDown={handleKeyDown}
      ref={modal}
      role={role}
    >
      {children}
    </div>
  );
}
