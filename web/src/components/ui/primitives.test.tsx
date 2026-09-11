import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { EmptyState } from "@/components/empty-state";
import { LoadingState } from "@/components/loading-state";
import { ProblemState } from "@/components/problem-state";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { StatusBadge } from "@/components/ui/status-badge";

function DialogHarness(): React.ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <div data-testid="outside">
      <Button onClick={() => setOpen(true)}>Edit case</Button>
      <Dialog
        description="Replace all editable fields of this case."
        onOpenChange={setOpen}
        open={open}
        title="Edit dataset case"
      >
        <label htmlFor="question">Question</label>
        <input id="question" />
        <Button disabled>Save</Button>
        <Button onClick={() => setOpen(false)}>Cancel</Button>
      </Dialog>
    </div>
  );
}

function polyfillDialogEnv(): void {
  if (typeof window.ResizeObserver === "undefined") {
    class ResizeObserverMock {
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {}
    }
    window.ResizeObserver = ResizeObserverMock as unknown as typeof ResizeObserver;
  }
  const elementPrototype = window.HTMLElement.prototype;
  if (elementPrototype.scrollIntoView === undefined) {
    elementPrototype.scrollIntoView = () => undefined;
  }
  if (typeof elementPrototype.hasPointerCapture === "undefined") {
    elementPrototype.hasPointerCapture = () => false;
  }
}

describe("Button", () => {
  it("renders a variant button that respects disabled and clicks", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<Button variant="primary">Save</Button>);
    render(<Button disabled>Locked</Button>);
    render(
      <Button onClick={onSelect} variant="danger">
        Delete
      </Button>,
    );
    expect(screen.getByRole("button", { name: "Save" })).toHaveClass("bg-accent");
    expect(screen.getByRole("button", { name: "Locked" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(onSelect).toHaveBeenCalled();
  });
});

describe("Dialog", () => {
  beforeEach(() => {
    polyfillDialogEnv();
  });

  it("opens with an accessible title and description", async () => {
    const user = userEvent.setup();
    render(<DialogHarness />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Edit case" }));
    const dialog = screen.getByRole("dialog", { name: "Edit dataset case" });
    expect(dialog).toHaveAccessibleDescription("Replace all editable fields of this case.");
    expect(screen.getByText("Question")).toBeInTheDocument();
  });

  it("closes on Escape and restores focus to the trigger", async () => {
    const user = userEvent.setup();
    render(<DialogHarness />);
    const trigger = screen.getByRole("button", { name: "Edit case" });
    trigger.focus();
    await user.click(trigger);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("keeps keyboard focus inside the dialog", async () => {
    const user = userEvent.setup();
    render(<DialogHarness />);
    await user.click(screen.getByRole("button", { name: "Edit case" }));
    const dialog = screen.getByRole("dialog");
    for (let press = 0; press < 6; press++) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
    await user.keyboard("{Shift>}{Tab}{/Shift}");
    expect(dialog.contains(document.activeElement)).toBe(true);
  });

  it("makes background content inert while open", async () => {
    const user = userEvent.setup();
    const { container } = render(<DialogHarness />);
    expect(container).not.toHaveAttribute("aria-hidden");
    await user.click(screen.getByRole("button", { name: "Edit case" }));
    expect(container).toHaveAttribute("aria-hidden", "true");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(container).not.toHaveAttribute("aria-hidden", "true"));
  });

  it("keeps the disabled submit from acting and scrolls long content", async () => {
    const user = userEvent.setup();
    render(<DialogHarness />);
    await user.click(screen.getByRole("button", { name: "Edit case" }));
    const dialog = screen.getByRole("dialog");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(dialog).toHaveClass("overflow-y-auto");
  });
});

describe("Field", () => {
  it("wires label and error text", () => {
    render(
      <Field error="Required value" hint="Used for scoring" label="Question">
        <input />
      </Field>,
    );
    expect(screen.getByText("Question")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Required value");
    expect(screen.getByText("Used for scoring")).toBeInTheDocument();
  });
});

describe("StatusBadge", () => {
  it("labels a semantic tone without content loss", () => {
    const { container } = render(<StatusBadge tone="success">passed</StatusBadge>);
    expect(container.querySelector("span")).toHaveTextContent("passed");
    expect(container.querySelector("span")).toHaveClass("bg-success/10");
  });
});

describe("shared states", () => {
  it("offers a retry action from a problem state", async () => {
    const user = userEvent.setup();
    const retry = vi.fn();
    render(<ProblemState detail="Could not load" onRetry={retry} title="Request failed" />);
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalled();
  });

  it("renders an empty state action and a labeled loading state", () => {
    render(
      <EmptyState
        action={<Button variant="primary">Add dataset</Button>}
        description="No cases yet."
        title="No datasets"
      />,
    );
    expect(screen.getByRole("heading", { name: "No datasets" })).toBeInTheDocument();
    render(<LoadingState label="Loading datasets" />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
  });
});
