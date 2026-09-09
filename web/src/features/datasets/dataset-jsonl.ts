import type { DatasetItemInput, DatasetItemResponse } from "@/api/generated/types.gen";

export function parseDatasetJsonl(text: string): DatasetItemInput[] {
  const items: DatasetItemInput[] = [];
  const externalIDs = new Set<string>();
  for (const [index, line] of text
    .replace(/^\uFEFF/, "")
    .split(/\r?\n/)
    .entries()) {
    if (line.trim() === "") continue;
    let value: unknown;
    try {
      value = JSON.parse(line);
    } catch {
      throw new Error(`Line ${index + 1}: invalid JSON`);
    }
    if (!isDatasetItemInput(value)) throw new Error(`Line ${index + 1}: invalid dataset item`);
    if (value.external_id !== undefined) {
      if (externalIDs.has(value.external_id))
        throw new Error(`Line ${index + 1}: duplicate external_id`);
      externalIDs.add(value.external_id);
    }
    items.push(value);
  }
  return items;
}

export function serializeDatasetJsonl(items: readonly DatasetItemResponse[]): string {
  return items.map((item) => JSON.stringify(writableFields(item))).join("\n");
}

function isDatasetItemInput(value: unknown): value is DatasetItemInput {
  if (value === null || Array.isArray(value) || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  if (item["input"] === null || typeof item["input"] !== "object" || Array.isArray(item["input"])) {
    return false;
  }
  if (item["external_id"] !== undefined && typeof item["external_id"] !== "string") return false;
  if (item["context"] !== undefined && !isChunks(item["context"])) return false;
  return item["metadata"] === undefined || isObject(item["metadata"]);
}

function isChunks(value: unknown): boolean {
  return (
    Array.isArray(value) &&
    value.every(
      (chunk) =>
        isObject(chunk) && typeof chunk["id"] === "string" && typeof chunk["text"] === "string",
    )
  );
}

function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && !Array.isArray(value) && typeof value === "object";
}

function writableFields(item: DatasetItemResponse): DatasetItemInput {
  const fields: DatasetItemInput = {
    context: item.context ?? [],
    input: item.input,
    metadata: item.metadata,
  };
  if (item.external_id !== undefined) fields.external_id = item.external_id;
  if (item.output !== undefined) fields.output = item.output;
  if (item.expected_output !== undefined) fields.expected_output = item.expected_output;
  return fields;
}
