import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Link, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { getDataset, listDatasetItems } from "@/api/generated/sdk.gen";
import type { DatasetItemResponse, DatasetResponse } from "@/api/generated/types.gen";
import { JsonView } from "@/components/json-view";
import { AddDatasetItem } from "@/features/datasets/add-item-dialog";
import { DatasetItemDelete } from "@/features/datasets/dataset-item-delete";
import { DatasetItemEditor } from "@/features/datasets/dataset-item-editor";
import { DatasetMetadataDialog } from "@/features/datasets/dataset-metadata-dialog";

export function DatasetDetail() {
  const { appId = "", datasetId = "" } = useParams();
  const requestNumber = useRef(0);
  const activeRequest = useRef<AbortController | null>(null);
  const [dataset, setDataset] = useState<DatasetResponse | null>(null);
  const [items, setItems] = useState<DatasetItemResponse[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [seenCursors, setSeenCursors] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const currentRequest = ++requestNumber.current;
    activeRequest.current?.abort();
    activeRequest.current = controller;
    setDataset(null);
    setItems([]);
    setNextCursor(null);
    setSeenCursors(new Set());
    setLoading(true);
    setError(null);
    void Promise.all([
      getDataset({ path: { id: datasetId }, signal: controller.signal, throwOnError: true }),
      listDatasetItems({ path: { id: datasetId }, signal: controller.signal, throwOnError: true }),
    ])
      .then(([datasetResponse, itemsResponse]) => {
        if (controller.signal.aborted || requestNumber.current !== currentRequest) return;
        if (datasetResponse.data.application_id !== appId) {
          setError("Dataset does not belong to this application");
          return;
        }
        setDataset(datasetResponse.data);
        setItems(itemsResponse.data.items ?? []);
        setNextCursor(itemsResponse.data.next_cursor ?? null);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted && requestNumber.current === currentRequest) {
          setError(
            reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to load dataset",
          );
        }
      })
      .finally(() => {
        if (requestNumber.current === currentRequest) setLoading(false);
      });
    return () => {
      controller.abort();
      activeRequest.current?.abort();
    };
  }, [appId, datasetId]);

  async function loadMore(): Promise<void> {
    if (nextCursor === null) return;
    const cursor = nextCursor;
    const consumed = new Set(seenCursors).add(cursor);
    const controller = new AbortController();
    const currentRequest = ++requestNumber.current;
    activeRequest.current = controller;
    setLoading(true);
    setError(null);
    try {
      const response = await listDatasetItems({
        path: { id: datasetId },
        query: { cursor },
        signal: controller.signal,
        throwOnError: true,
      });
      if (controller.signal.aborted || requestNumber.current !== currentRequest) return;
      const receivedCursor = response.data.next_cursor ?? null;
      setItems((current) => {
        const existing = new Set(current.map((item) => item.id));
        return [
          ...current,
          ...(response.data.items ?? []).filter((item) => !existing.has(item.id)),
        ];
      });
      setSeenCursors(consumed);
      if (receivedCursor !== null && consumed.has(receivedCursor)) {
        setError("The server returned a repeated cursor. Pagination stopped.");
        setNextCursor(null);
      } else setNextCursor(receivedCursor);
    } catch (reason) {
      if (!controller.signal.aborted)
        setError(reason instanceof Problem ? reason.title : "Unable to load more items");
    } finally {
      if (requestNumber.current === currentRequest) setLoading(false);
    }
  }

  return (
    <DatasetView
      appId={appId}
      dataset={dataset}
      error={error}
      items={items}
      loading={loading}
      nextCursor={nextCursor}
      onLoadMore={loadMore}
      onCreated={(created) => setItems((current) => [...created, ...current])}
      onDatasetUpdated={setDataset}
      onUpdated={(updated) =>
        setItems((current) => current.map((item) => (item.id === updated.id ? updated : item)))
      }
      onDeleted={(itemID) => setItems((current) => current.filter((item) => item.id !== itemID))}
    />
  );
}

type DatasetViewProps = {
  appId: string;
  dataset: DatasetResponse | null;
  error: string | null;
  items: DatasetItemResponse[];
  loading: boolean;
  nextCursor: string | null;
  onLoadMore: () => Promise<void>;
  onCreated: (items: DatasetItemResponse[]) => void;
  onDatasetUpdated: (dataset: DatasetResponse) => void;
  onUpdated: (item: DatasetItemResponse) => void;
  onDeleted: (itemID: string) => void;
};

function DatasetView(props: DatasetViewProps) {
  if (props.loading && props.dataset === null && props.error === null) {
    return <p className="text-muted">Loading dataset...</p>;
  }
  if (props.dataset === null) {
    return (
      <p className="border border-red-300 bg-red-50 p-4" role="alert">
        {props.error ?? "Dataset unavailable"}
      </p>
    );
  }
  const datasetID = props.dataset.id;
  return (
    <section aria-labelledby="dataset-heading">
      <Link className="text-sm text-accent hover:underline" to={`/apps/${props.appId}/datasets`}>
        Back to datasets
      </Link>
      <h1 className="mt-4 text-2xl font-semibold" id="dataset-heading">
        {props.dataset.name}
      </h1>
      <p className="mt-2 text-sm text-muted">
        {props.dataset.description?.trim() || "No description"}
      </p>
      <div className="mt-4">
        <DatasetMetadataDialog dataset={props.dataset} onSaved={props.onDatasetUpdated} />
      </div>
      {!props.loading && (
        <AddDatasetItem
          key={props.dataset.id}
          datasetID={props.dataset.id}
          onCreated={props.onCreated}
        />
      )}
      {props.error && (
        <p className="mt-4 border border-amber-300 bg-amber-50 p-3 text-sm" role="alert">
          {props.error}
        </p>
      )}
      <div className="mt-6 space-y-3">
        {props.items.map((item) => (
          <DatasetItem
            datasetID={datasetID}
            item={item}
            key={item.id}
            onDeleted={props.onDeleted}
            onUpdated={props.onUpdated}
          />
        ))}
      </div>
      {!props.loading && props.error === null && props.items.length === 0 && (
        <p className="mt-8 text-muted">No dataset items yet.</p>
      )}
      {props.nextCursor !== null && (
        <button
          className="mt-4 border border-line bg-surface px-4 py-2 text-sm font-medium disabled:opacity-50"
          disabled={props.loading}
          onClick={() => void props.onLoadMore()}
        >
          Load more items
        </button>
      )}
    </section>
  );
}

function DatasetItem({
  datasetID,
  item,
  onDeleted,
  onUpdated,
}: {
  datasetID: string;
  item: DatasetItemResponse;
  onDeleted: (itemID: string) => void;
  onUpdated: (item: DatasetItemResponse) => void;
}) {
  return (
    <details className="border border-line bg-surface">
      <summary className="cursor-pointer px-4 py-3 font-medium">
        {item.external_id ??
          (typeof item.input["question"] === "string" ? item.input["question"] : item.id)}
      </summary>
      <div className="flex justify-end border-t border-line px-4 pt-3">
        <DatasetItemEditor datasetID={datasetID} item={item} onSaved={onUpdated} />
        <DatasetItemDelete
          datasetID={datasetID}
          itemID={item.id}
          label={item.external_id ?? item.id}
          onDeleted={() => onDeleted(item.id)}
        />
      </div>
      <div className="grid gap-5 border-t border-line p-4 lg:grid-cols-2">
        <ItemField label="Input">
          <JsonView value={item.input} />
        </ItemField>
        <ItemField label="Output">
          <TextValue value={item.output} />
        </ItemField>
        <ItemField label="Expected output">
          <TextValue value={item.expected_output} />
        </ItemField>
        <ItemField label="Context">
          <JsonView value={item.context} />
        </ItemField>
        <ItemField label="Metadata">
          <JsonView value={item.metadata} />
        </ItemField>
      </div>
    </details>
  );
}

function ItemField({ children, label }: { children: ReactNode; label: string }) {
  return (
    <section>
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted">{label}</h3>
      {children}
    </section>
  );
}

function TextValue({ value }: { value: string | undefined }) {
  return (
    <p className="whitespace-pre-wrap break-words text-sm">{value?.trim() || "Not provided"}</p>
  );
}
