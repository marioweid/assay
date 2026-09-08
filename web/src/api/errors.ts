type ProblemFields = {
  operation: string;
  status: number;
  title: string;
  detail: string | undefined;
};

export class Problem extends Error {
  readonly operation: string;
  readonly status: number;
  readonly title: string;
  readonly detail: string | undefined;

  constructor(fields: ProblemFields) {
    super(`${fields.operation}: ${fields.detail ?? fields.title}`);
    this.name = "Problem";
    this.operation = fields.operation;
    this.status = fields.status;
    this.title = fields.title;
    this.detail = fields.detail;
  }
}

export function normalizeProblem(
  error: unknown,
  response: Response | undefined,
  operation: string,
): Problem {
  if (error instanceof Problem) {
    return error;
  }
  const body = isRecord(error) ? error : {};
  const status = response?.status ?? numberField(body, "status") ?? 0;
  const title = (stringField(body, "title") ?? response?.statusText) || "Request failed";
  const messages = Array.isArray(body["errors"])
    ? body["errors"].flatMap((item: unknown) => {
        const message = isRecord(item) ? stringField(item, "message") : undefined;
        return message === undefined ? [] : [message];
      })
    : [];
  const detail = [stringField(body, "detail"), ...messages].filter(Boolean).join(": ") || undefined;
  return new Problem({ operation, status, title, detail });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringField(value: Record<string, unknown>, key: string): string | undefined {
  const field = value[key];
  return typeof field === "string" && field.trim() !== "" ? field : undefined;
}

function numberField(value: Record<string, unknown>, key: string): number | undefined {
  const field = value[key];
  return typeof field === "number" && Number.isFinite(field) ? field : undefined;
}
