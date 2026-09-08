import { useEffect, useRef, useState } from "react";
import { Clipboard } from "lucide-react";

import { Problem } from "@/api/errors";
import { createApiKey, listApiKeys, revokeApiKey } from "@/api/generated/sdk.gen";
import type { ApiKeyResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/empty-state";
import { fieldControlClass } from "@/components/ui/field";
import { LoadingState } from "@/components/loading-state";
import { ProblemState } from "@/components/problem-state";

export function ApiKeysPanel({ projectID }: { projectID: string }): React.ReactElement {
  const [refresh, setRefresh] = useState(0);
  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-ink">API keys</h2>
        <CreateKeyButton onCreated={() => setRefresh((value) => value + 1)} projectID={projectID} />
      </div>
      <KeyList projectID={projectID} refreshKey={refresh} />
    </section>
  );
}

export function CreateKeyButton({
  projectID,
  onCreated,
}: {
  projectID: string;
  onCreated: () => void;
}): React.ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)} variant="primary">
        New key
      </Button>
      {open ? (
        <CreateKeyDialog
          onClose={() => setOpen(false)}
          onCreated={onCreated}
          projectID={projectID}
        />
      ) : null}
    </>
  );
}

function CreateKeyDialog({
  projectID,
  onClose,
  onCreated,
}: {
  projectID: string;
  onClose: () => void;
  onCreated: () => void;
}): React.ReactElement {
  const activeRequest = useRef<AbortController | null>(null);
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [copied, setCopied] = useState<"idle" | "copied" | "failed">("idle");
  useEffect(() => () => activeRequest.current?.abort(), []);

  async function submit(event: React.FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    const trimmed = name.trim();
    if (trimmed === "" || activeRequest.current !== null) return;
    const controller = new AbortController();
    activeRequest.current = controller;
    setSubmitting(true);
    setError(null);
    try {
      const response = await createApiKey({
        path: { id: projectID },
        body: { name: trimmed },
        signal: controller.signal,
        throwOnError: true,
      });
      if (!controller.signal.aborted) {
        setCreatedKey(response.data.key);
        setCopied("idle");
        onCreated();
      }
    } catch (reason) {
      if (!controller.signal.aborted)
        setError(
          reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to create key",
        );
    } finally {
      activeRequest.current = null;
      if (!controller.signal.aborted) setSubmitting(false);
    }
  }

  async function copyKey(): Promise<void> {
    if (createdKey === null) return;
    try {
      await navigator.clipboard.writeText(createdKey);
      setCopied("copied");
    } catch {
      setCopied("failed");
    }
  }

  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      open
      title={createdKey === null ? "New API key" : "Copy your key"}
    >
      {createdKey === null ? (
        <form className="mt-4 space-y-4" onSubmit={(event) => void submit(event)}>
          <label className="block text-sm">
            <span className="font-medium text-ink">Key name</span>
            <input
              autoFocus
              className={fieldControlClass + " mt-1"}
              onChange={(event) => setName(event.target.value)}
              value={name}
            />
          </label>
          {error !== null && (
            <p className="text-sm text-danger" role="alert">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-3">
            <Button onClick={onClose}>Cancel</Button>
            <Button disabled={submitting || name.trim() === ""} type="submit" variant="primary">
              {submitting ? "Creating..." : "Create key"}
            </Button>
          </div>
        </form>
      ) : (
        <div className="mt-4 space-y-4">
          <p className="text-sm text-muted">
            The full key is shown only once. Store it in your emitter configuration now.
          </p>
          <pre className="break-all rounded-md border border-line bg-canvas p-3 font-mono text-sm">
            {createdKey}
          </pre>
          {copied !== "idle" && (
            <p aria-live="polite" className="text-sm text-muted" role="status">
              {copied === "copied"
                ? "Key copied to clipboard."
                : "Copy failed — select the key above to copy it manually."}
            </p>
          )}
          <div className="flex flex-wrap justify-end gap-3">
            <Button onClick={() => void copyKey()}>
              <Clipboard aria-hidden="true" size={16} />
              {copied === "copied" ? "Copied" : "Copy key"}
            </Button>
            <Button onClick={onClose} variant="secondary">
              Done
            </Button>
          </div>
        </div>
      )}
    </Dialog>
  );
}

function KeyList({
  projectID,
  refreshKey,
}: {
  projectID: string;
  refreshKey: number;
}): React.ReactElement {
  const [keys, setKeys] = useState<ApiKeyResponse[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refresh, setRefresh] = useState(0);
  const [revoking, setRevoking] = useState<ApiKeyResponse | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    void listApiKeys({
      path: { id: projectID },
      signal: controller.signal,
      throwOnError: true,
    })
      .then((response) => {
        if (!controller.signal.aborted) setKeys(response.data.items ?? []);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("Unable to load API keys");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [projectID, refresh, refreshKey]);

  if (loading) return <LoadingState label="Loading API keys" />;
  if (error !== null)
    return (
      <ProblemState
        detail={error}
        onRetry={() => setRefresh((value) => value + 1)}
        title="API keys unavailable"
      />
    );
  if ((keys ?? []).length === 0)
    return (
      <EmptyState
        description="Create a key to configure emitters that send traces into Assay."
        title="No API keys yet"
      />
    );

  return (
    <>
      <div className="overflow-x-auto border border-line bg-surface">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-line bg-canvas text-xs uppercase text-muted">
            <tr>
              <th className="px-4 py-3">Name</th>
              <th className="px-4 py-3">Prefix</th>
              <th className="px-4 py-3">Last used</th>
              <th className="px-4 py-3">Status</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {(keys ?? []).map((key) => (
              <tr key={key.id} className="border-b border-line last:border-0">
                <td className="px-4 py-3 font-medium">{key.name}</td>
                <td className="px-4 py-3 font-mono text-xs">{key.key_prefix}</td>
                <td className="px-4 py-3 font-mono text-xs">
                  {key.last_used_at ? new Date(key.last_used_at).toLocaleString() : "Never"}
                </td>
                <td className="px-4 py-3">{key.revoked_at ? "Revoked" : "Active"}</td>
                <td className="px-4 py-3 text-right">
                  {key.revoked_at === undefined ? (
                    <Button onClick={() => setRevoking(key)} variant="ghost">
                      Revoke
                    </Button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {revoking !== null ? (
        <RevokeKeyDialog
          keyID={revoking.id}
          keyName={revoking.name}
          onClose={() => setRevoking(null)}
          onRevoked={() => {
            setRevoking(null);
            setRefresh((value) => value + 1);
          }}
          projectID={projectID}
        />
      ) : null}
    </>
  );
}

function RevokeKeyDialog({
  projectID,
  keyID,
  keyName,
  onClose,
  onRevoked,
}: {
  projectID: string;
  keyID: string;
  keyName: string;
  onClose: () => void;
  onRevoked: () => void;
}): React.ReactElement {
  const [revoking, setRevoking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function revoke(): Promise<void> {
    setRevoking(true);
    setError(null);
    try {
      await revokeApiKey({
        path: { id: projectID, keyId: keyID },
        throwOnError: true,
      });
      onRevoked();
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to revoke key",
      );
      setRevoking(false);
    }
  }

  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      open
      title={`Revoke ${keyName}?`}
    >
      <p className="mt-3 text-sm text-muted">
        Emitters using this key will stop being accepted. You cannot recover a revoked key; create a
        replacement and update your emitters instead.
      </p>
      {error !== null && (
        <p className="mt-3 text-sm text-danger" role="alert">
          {error}
        </p>
      )}
      <div className="mt-6 flex justify-end gap-3">
        <Button onClick={onClose}>Cancel</Button>
        <Button disabled={revoking} onClick={() => void revoke()} variant="danger">
          Revoke key
        </Button>
      </div>
    </Dialog>
  );
}
