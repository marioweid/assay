import { useEffect, useRef, useState } from "react";

import { Problem } from "@/api/errors";
import { createDatasetItems } from "@/api/generated/sdk.gen";
import type { DatasetItemInput, DatasetItemResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

type Props = { datasetID: string; onCreated: (items: DatasetItemResponse[]) => void };
const fields = [
  ["question", "Question"],
  ["output", "Recorded answer (optional)"],
  ["expected", "Expected answer (optional)"],
  ["context", "Supporting context (optional)"],
  ["externalID", "External ID (optional)"],
  ["inputJSON", "Input JSON"],
  ["metadataJSON", "Metadata JSON"],
] as const;
type Values = Record<(typeof fields)[number][0], string>;

export function AddDatasetItem(props: Props): React.ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)} variant="primary">
        Add item
      </Button>
      <AddItemDialog {...props} onClose={() => setOpen(false)} open={open} />
    </>
  );
}

function AddItemDialog({
  datasetID,
  onCreated,
  onClose,
  open,
}: Props & { onClose: () => void; open: boolean }) {
  const [values, setValues] = useState<Values>({
    question: "",
    output: "",
    expected: "",
    context: "",
    externalID: "",
    inputJSON: "{}",
    metadataJSON: "{}",
  });
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (active.current !== null || values.question.trim() === "") return;
    const item = toItem(values);
    if (item === null) {
      setError("Input JSON and Metadata JSON must each be a JSON object.");
      return;
    }
    const controller = new AbortController();
    active.current = controller;
    setSubmitting(true);
    setError(null);
    try {
      const response = await createDatasetItems({
        path: { id: datasetID },
        body: { items: [item] },
        signal: controller.signal,
        throwOnError: true,
      });
      if (!controller.signal.aborted) {
        onCreated(response.data.items ?? []);
        onClose();
      }
    } catch (reason) {
      if (!controller.signal.aborted)
        setError(
          reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to add item",
        );
    } finally {
      active.current = null;
      if (!controller.signal.aborted) setSubmitting(false);
    }
  }

  return (
    <Dialog
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      open={open}
      title="Add dataset item"
    >
      <form className="mt-4 space-y-4" onSubmit={(event) => void submit(event)}>
        <p className="text-sm text-muted">
          Record an answer to score it directly, or leave it blank to generate one during
          evaluation. Correctness uses the expected answer; groundedness uses supporting context.
        </p>
        {fields.map(([key, label]) => (
          <label className="block text-sm" key={key}>
            <span className="font-medium text-ink">{label}</span>
            <textarea
              className={fieldControlClass + " mt-1"}
              rows={2}
              required={key === "question"}
              disabled={submitting}
              value={values[key]}
              onChange={(event) => setValues({ ...values, [key]: event.target.value })}
            />
          </label>
        ))}
        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-3">
          <Button onClick={onClose}>Cancel</Button>
          <Button
            disabled={submitting || values.question.trim() === ""}
            type="submit"
            variant="primary"
          >
            {submitting ? "Saving..." : "Save item"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function toItem(values: Values): DatasetItemInput | null {
  const input = parseObject(values.inputJSON);
  const metadata = parseObject(values.metadataJSON);
  if (input === null || metadata === null) return null;
  const item: DatasetItemInput = { input: { ...input, question: values.question.trim() } };
  if (values.externalID.trim() !== "") item.external_id = values.externalID.trim();
  if (Object.keys(metadata).length > 0) item.metadata = metadata;
  if (values.output.trim() !== "") item.output = values.output.trim();
  if (values.expected.trim() !== "") item.expected_output = values.expected.trim();
  if (values.context.trim() !== "")
    item.context = [{ id: "context-1", text: values.context.trim() }];
  return item;
}

function parseObject(value: string): Record<string, unknown> | null {
  try {
    const parsed: unknown = JSON.parse(value);
    return parsed !== null && !Array.isArray(parsed) && typeof parsed === "object"
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}
