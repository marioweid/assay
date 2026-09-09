import { useState } from "react";

import { Problem } from "@/api/errors";
import { replaceDatasetItem } from "@/api/generated/sdk.gen";
import type { DatasetItemResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

export function DatasetItemEditor({
  datasetID,
  item,
  onSaved,
}: {
  datasetID: string;
  item: DatasetItemResponse;
  onSaved: (item: DatasetItemResponse) => void;
}): React.ReactElement {
  const [open, setOpen] = useState(false);
  const [question, setQuestion] = useState(questionOf(item));
  const [output, setOutput] = useState(item.output ?? "");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function save(): Promise<void> {
    if (question.trim() === "") {
      setError("Question is required.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const response = await replaceDatasetItem({
        body: {
          context: item.context ?? [],
          expected_output: item.expected_output ?? null,
          external_id: item.external_id ?? null,
          input: { ...item.input, question: question.trim() },
          metadata: item.metadata,
          output: output.trim() === "" ? null : output.trim(),
        },
        path: { id: datasetID, itemId: item.id },
        throwOnError: true,
      });
      onSaved(response.data);
      setOpen(false);
    } catch (reason) {
      setError(reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to save item");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <Button
        aria-label={`Edit ${item.external_id ?? item.id}`}
        onClick={() => setOpen(true)}
        variant="ghost"
      >
        Edit
      </Button>
      {open && (
        <Dialog onOpenChange={(next) => !next && setOpen(false)} open title="Edit dataset case">
          <div className="mt-4 space-y-4">
            <label className="block text-sm">
              <span className="font-medium">Question</span>
              <textarea
                aria-label="Question"
                autoFocus
                className={`${fieldControlClass} mt-1`}
                onChange={(event) => setQuestion(event.target.value)}
                value={question}
              />
            </label>
            <label className="block text-sm">
              <span className="font-medium">Recorded answer (optional)</span>
              <textarea
                aria-label="Recorded answer (optional)"
                className={`${fieldControlClass} mt-1`}
                onChange={(event) => setOutput(event.target.value)}
                value={output}
              />
            </label>
            {error !== null && (
              <p className="text-sm text-danger" role="alert">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-3">
              <Button disabled={saving} onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button disabled={saving} onClick={() => void save()} variant="primary">
                Save item
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  );
}

function questionOf(item: DatasetItemResponse): string {
  const question = item.input["question"];
  return typeof question === "string" ? question : "";
}
