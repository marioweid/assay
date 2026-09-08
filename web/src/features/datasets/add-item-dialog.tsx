import { useEffect, useRef, useState } from "react";

import { Problem } from "@/api/errors";
import { createDatasetItems } from "@/api/generated/sdk.gen";
import type { DatasetItemInput, DatasetItemResponse } from "@/api/generated/types.gen";
import { Modal } from "@/components/modal";

type Props = { datasetID: string; onCreated: (items: DatasetItemResponse[]) => void };
const fields = [
  ["question", "Question"],
  ["output", "Recorded answer (optional)"],
  ["expected", "Expected answer (optional)"],
  ["context", "Supporting context (optional)"],
] as const;
type Values = Record<(typeof fields)[number][0], string>;

export function AddDatasetItem(props: Props) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        className="mt-4 bg-blue-700 px-4 py-2 text-sm text-white"
        onClick={() => setOpen(true)}
      >
        Add item
      </button>
      {open && <AddItemDialog {...props} onClose={() => setOpen(false)} />}
    </>
  );
}

function AddItemDialog({ datasetID, onCreated, onClose }: Props & { onClose: () => void }) {
  const [values, setValues] = useState<Values>({
    question: "",
    output: "",
    expected: "",
    context: "",
  });
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (active.current !== null || values.question.trim() === "") return;
    const controller = new AbortController();
    active.current = controller;
    setSubmitting(true);
    setError(null);
    try {
      const response = await createDatasetItems({
        path: { id: datasetID },
        body: { items: [toItem(values)] },
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
    <Modal label="Add dataset item" onClose={onClose}>
      <form
        className="max-h-[90vh] w-full max-w-xl space-y-4 overflow-y-auto border border-line bg-white p-6 shadow-xl"
        onSubmit={(event) => void submit(event)}
      >
        <h2 className="text-xl font-semibold">Add dataset item</h2>
        <p className="text-sm text-muted">
          Record an answer to score it directly, or leave it blank to generate one during
          evaluation. Correctness uses the expected answer; groundedness uses supporting context.
        </p>
        {fields.map(([key, label]) => (
          <label className="block text-sm" key={key}>
            {label}
            <textarea
              className="mt-1 block w-full border border-line bg-white px-3 py-2"
              rows={2}
              required={key === "question"}
              disabled={submitting}
              value={values[key]}
              onChange={(event) => setValues({ ...values, [key]: event.target.value })}
            />
          </label>
        ))}
        {error !== null && (
          <p role="alert" className="text-sm text-red-700">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-3">
          <button type="button" className="border border-line px-4 py-2 text-sm" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className="bg-blue-700 px-4 py-2 text-sm text-white disabled:opacity-50"
            disabled={submitting || values.question.trim() === ""}
          >
            {submitting ? "Saving..." : "Save item"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function toItem(values: Values): DatasetItemInput {
  const item: DatasetItemInput = { input: { question: values.question.trim() } };
  if (values.output.trim() !== "") item.output = values.output.trim();
  if (values.expected.trim() !== "") item.expected_output = values.expected.trim();
  if (values.context.trim() !== "")
    item.context = [{ id: "context-1", text: values.context.trim() }];
  return item;
}
