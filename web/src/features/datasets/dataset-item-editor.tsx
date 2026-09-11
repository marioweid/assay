import { useState } from "react";

import { Problem } from "@/api/errors";
import { replaceDatasetItem } from "@/api/generated/sdk.gen";
import type { Chunk, DatasetItemResponse } from "@/api/generated/types.gen";
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
  const [expectedOutput, setExpectedOutput] = useState(item.expected_output ?? "");
  const [externalID, setExternalID] = useState(item.external_id ?? "");
  const [contextJSON, setContextJSON] = useState(JSON.stringify(item.context ?? [], null, 2));
  const [inputJSON, setInputJSON] = useState(JSON.stringify(withoutQuestion(item.input), null, 2));
  const [metadataJSON, setMetadataJSON] = useState(JSON.stringify(item.metadata, null, 2));
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function save(): Promise<void> {
    if (question.trim() === "") {
      setError("Question is required.");
      return;
    }
    const context = parseChunks(contextJSON);
    const input = parseObject(inputJSON);
    const metadata = parseObject(metadataJSON);
    if (context === null || input === null || metadata === null) {
      setError("Context must be chunks; Input JSON and Metadata JSON must each be JSON objects.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const response = await replaceDatasetItem({
        body: {
          context,
          expected_output: expectedOutput.trim() === "" ? null : expectedOutput.trim(),
          external_id: externalID.trim() === "" ? null : externalID.trim(),
          input: { ...input, question: question.trim() },
          metadata,
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
              <span className="font-medium">Context JSON</span>
              <textarea
                aria-label="Context JSON"
                className={`${fieldControlClass} mt-1 font-mono`}
                onChange={(event) => setContextJSON(event.target.value)}
                value={contextJSON}
              />
            </label>
            <label className="block text-sm">
              <span className="font-medium">External ID (optional)</span>
              <input
                aria-label="External ID (optional)"
                className={`${fieldControlClass} mt-1`}
                onChange={(event) => setExternalID(event.target.value)}
                value={externalID}
              />
            </label>
            <label className="block text-sm">
              <span className="font-medium">Expected answer (optional)</span>
              <textarea
                aria-label="Expected answer (optional)"
                className={`${fieldControlClass} mt-1`}
                onChange={(event) => setExpectedOutput(event.target.value)}
                value={expectedOutput}
              />
            </label>
            <label className="block text-sm">
              <span className="font-medium">Input JSON</span>
              <textarea
                aria-label="Input JSON"
                className={`${fieldControlClass} mt-1 font-mono`}
                onChange={(event) => setInputJSON(event.target.value)}
                value={inputJSON}
              />
            </label>
            <label className="block text-sm">
              <span className="font-medium">Metadata JSON</span>
              <textarea
                aria-label="Metadata JSON"
                className={`${fieldControlClass} mt-1 font-mono`}
                onChange={(event) => setMetadataJSON(event.target.value)}
                value={metadataJSON}
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

function parseChunks(value: string): Chunk[] | null {
  try {
    const parsed: unknown = JSON.parse(value);
    if (!Array.isArray(parsed)) return null;
    if (!parsed.every((chunk) => isChunk(chunk))) return null;
    return parsed;
  } catch {
    return null;
  }
}

function isChunk(value: unknown): value is Chunk {
  if (value === null || typeof value !== "object" || Array.isArray(value)) return false;
  const chunk = value as Record<string, unknown>;
  return typeof chunk["id"] === "string" && typeof chunk["text"] === "string";
}

function withoutQuestion(input: Record<string, unknown>): Record<string, unknown> {
  const rest = { ...input };
  delete rest["question"];
  return rest;
}

function parseObject(value: string): Record<string, unknown> | null {
  try {
    const parsed: unknown = JSON.parse(value);
    if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object") return null;
    return parsed as Record<string, unknown>;
  } catch {
    return null;
  }
}

function questionOf(item: DatasetItemResponse): string {
  const question = item.input["question"];
  return typeof question === "string" ? question : "";
}
