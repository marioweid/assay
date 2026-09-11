import { Dialog as RadixDialog } from "radix-ui";
import { useEffect, useRef } from "react";
import type { ReactNode } from "react";

export type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  dismissible?: boolean;
  children: ReactNode;
};

export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  dismissible = true,
  children,
}: DialogProps): React.ReactElement {
  const lastFocused = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    lastFocused.current =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    return () => {
      if (lastFocused.current !== null) {
        lastFocused.current.focus();
        lastFocused.current = null;
      }
    };
  }, [open]);
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Portal>
        <RadixDialog.Overlay
          className="fixed inset-0 z-40 bg-black/40"
          data-testid="dialog-overlay"
        />
        <RadixDialog.Content
          onEscapeKeyDown={(event) => {
            if (!dismissible) event.preventDefault();
          }}
          onPointerDownOutside={(event) => {
            if (!dismissible) event.preventDefault();
          }}
          className={[
            "fixed left-1/2 top-1/2 z-50 grid w-[min(100%-2rem,36rem)]",
            "max-h-[calc(100dvh-2rem)] -translate-x-1/2 -translate-y-1/2",
            "overflow-y-auto rounded-lg border border-line bg-surface p-6 shadow-xl",
            "text-ink",
          ].join(" ")}
        >
          <RadixDialog.Title className="text-lg font-semibold">{title}</RadixDialog.Title>
          {description !== undefined && (
            <RadixDialog.Description className="mt-1 text-sm text-muted">
              {description}
            </RadixDialog.Description>
          )}
          {children}
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
