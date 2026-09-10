import { useState } from "react";

type JsonViewProps = {
  value: unknown;
};

export function JsonView({ value }: JsonViewProps) {
  const [copyStatus, setCopyStatus] = useState<"idle" | "copied" | "failed">("idle");
  const formatted = JSON.stringify(value, null, 2) ?? String(value);

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(formatted);
      setCopyStatus("copied");
    } catch {
      setCopyStatus("failed");
    }
  }

  return (
    <div className="border border-line bg-rail">
      <div className="flex justify-end border-b border-line px-3 py-2">
        <button className="text-xs text-ink" onClick={() => void copy()}>
          Copy JSON
        </button>
        {copyStatus === "copied" && <span className="ml-3 text-xs text-success">Copied</span>}
        {copyStatus === "failed" && (
          <span className="ml-3 text-xs text-danger" role="alert">
            Copy failed
          </span>
        )}
      </div>
      <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-xs leading-6 text-ink">
        {formatted}
      </pre>
    </div>
  );
}
