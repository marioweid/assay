type JsonViewProps = {
  value: unknown;
};

export function JsonView({ value }: JsonViewProps) {
  return (
    <pre className="max-h-96 overflow-auto border border-line bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-100">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}
