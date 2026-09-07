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
    <div className="border border-line bg-slate-950">
      <div className="flex justify-end border-b border-slate-700 px-3 py-2">
        <button className="text-xs text-slate-200" onClick={() => void copy()}>
          Copy JSON
        </button>
        {copyStatus === "copied" && <span className="ml-3 text-xs text-emerald-300">Copied</span>}
        {copyStatus === "failed" && (
          <span className="ml-3 text-xs text-red-300" role="alert">
            Copy failed
          </span>
        )}
      </div>
      <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-xs leading-6 text-slate-100">
        {formatted}
      </pre>
    </div>
  );
}
