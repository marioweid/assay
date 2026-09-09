import { useState } from "react";

import { Problem } from "@/api/errors";
import { updateApplicationEndpoint } from "@/api/generated/sdk.gen";
import type { TargetEndpointResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

export function EndpointForm({
  applicationID,
  endpoint,
  onSaved,
}: {
  applicationID: string;
  endpoint?: TargetEndpointResponse;
  onSaved: () => void;
}): React.ReactElement {
  const [url, setURL] = useState(endpoint?.url ?? "");
  const [method, setMethod] = useState(endpoint?.method ?? "POST");
  const [headers, setHeaders] = useState(() => JSON.stringify(endpoint?.headers ?? {}, null, 2));
  const [template, setTemplate] = useState(() =>
    JSON.stringify(endpoint?.request_template ?? {}, null, 2),
  );
  const [output, setOutput] = useState(endpoint?.response_mapping.output ?? "");
  const [context, setContext] = useState(endpoint?.response_mapping.context ?? "");
  const [timeout, setTimeout] = useState(String(endpoint?.timeout_ms ?? 30000));
  const [secret, setSecret] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmingClear, setConfirmingClear] = useState(false);

  async function save(event: React.FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    const parsedHeaders = parseObject(headers);
    const parsedTemplate = parseObject(template);
    const timeoutMS = Number(timeout);
    if (parsedHeaders === null || parsedTemplate === null) {
      setError("Headers and request template must be JSON objects");
      return;
    }
    if (!Number.isInteger(timeoutMS) || timeoutMS < 0 || output.trim() === "") {
      setError("Enter a response output path and a non-negative whole-number timeout");
      return;
    }
    const authorization = parsedHeaders["Authorization"];
    if (typeof authorization === "string" && !authorization.includes("{{ .secret }}")) {
      setError("Use {{ .secret }} in Authorization and enter its value in Replace secret.");
      return;
    }
    if (!Object.values(parsedHeaders).every((item) => typeof item === "string")) {
      setError("Each header value must be a string.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await updateApplicationEndpoint({
        body: {
          endpoint: {
            headers: stringsOnly(parsedHeaders),
            method: method.trim() || "POST",
            request_template: parsedTemplate,
            response_mapping: {
              ...(context.trim() === "" ? {} : { context: context.trim() }),
              output: output.trim(),
            },
            ...(secret === "" ? {} : { secret }),
            timeout_ms: timeoutMS,
            url: url.trim(),
          },
        },
        path: { id: applicationID },
        throwOnError: true,
      });
      onSaved();
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to save endpoint",
      );
    } finally {
      setSaving(false);
    }
  }

  async function clear(): Promise<void> {
    setSaving(true);
    setError(null);
    try {
      await updateApplicationEndpoint({
        body: { clear: true },
        path: { id: applicationID },
        throwOnError: true,
      });
      setConfirmingClear(false);
      onSaved();
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to remove endpoint",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="space-y-4 border-t border-line pt-6">
      <div>
        <h2 className="text-lg font-semibold">Target endpoint</h2>
        <p className="mt-1 text-sm text-muted">
          Used only by generate-then-score runs. Saving does not call the target.
        </p>
      </div>
      <form className="grid max-w-3xl gap-4" onSubmit={(event) => void save(event)}>
        <label className="text-sm">
          <span className="font-medium">URL</span>
          <input
            className={`${fieldControlClass} mt-1`}
            onChange={(event) => setURL(event.target.value)}
            type="url"
            value={url}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Method</span>
          <input
            className={`${fieldControlClass} mt-1`}
            onChange={(event) => setMethod(event.target.value)}
            value={method}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Headers JSON</span>
          <textarea
            className={`${fieldControlClass} mt-1 min-h-24 font-mono`}
            onChange={(event) => setHeaders(event.target.value)}
            value={headers}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Request template JSON</span>
          <textarea
            className={`${fieldControlClass} mt-1 min-h-24 font-mono`}
            onChange={(event) => setTemplate(event.target.value)}
            value={template}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Response output JSONPath</span>
          <input
            className={`${fieldControlClass} mt-1`}
            onChange={(event) => setOutput(event.target.value)}
            value={output}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Response context JSONPath</span>
          <input
            className={`${fieldControlClass} mt-1`}
            onChange={(event) => setContext(event.target.value)}
            value={context}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Timeout (ms)</span>
          <input
            className={`${fieldControlClass} mt-1`}
            min="0"
            onChange={(event) => setTimeout(event.target.value)}
            type="number"
            value={timeout}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Replace secret</span>
          <input
            className={`${fieldControlClass} mt-1`}
            autoComplete="new-password"
            onChange={(event) => setSecret(event.target.value)}
            type="password"
            value={secret}
          />
          <span className="mt-1 block text-xs text-muted">
            Leave blank to preserve the saved secret.
          </span>
        </label>
        {error !== null && (
          <p className="text-sm text-danger" role="alert">
            {error}
          </p>
        )}
        <div className="flex flex-wrap gap-3">
          <Button disabled={saving} type="submit" variant="primary">
            Save endpoint
          </Button>
          {endpoint !== undefined && (
            <Button disabled={saving} onClick={() => setConfirmingClear(true)} variant="danger">
              Remove endpoint
            </Button>
          )}
        </div>
      </form>
      {confirmingClear && (
        <Dialog
          onOpenChange={(open) => !open && setConfirmingClear(false)}
          open
          title="Remove target endpoint?"
        >
          <p className="mt-3 text-sm text-muted">
            Generate-then-score runs will be unavailable until a new endpoint is saved.
          </p>
          <div className="mt-6 flex justify-end gap-3">
            <Button onClick={() => setConfirmingClear(false)}>Cancel</Button>
            <Button disabled={saving} onClick={() => void clear()} variant="danger">
              Remove endpoint
            </Button>
          </div>
        </Dialog>
      )}
    </section>
  );
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

function stringsOnly(value: Record<string, unknown>): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [key, item] of Object.entries(value)) {
    if (typeof item !== "string") throw new Error(`Header ${key} must be a string`);
    result[key] = item;
  }
  return result;
}
