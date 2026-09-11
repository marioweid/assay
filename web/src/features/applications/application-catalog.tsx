import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

import { Problem } from "@/api/errors";
import { listApplications } from "@/api/generated/sdk.gen";
import type { ApplicationResponse } from "@/api/generated/types.gen";
import { useAuth } from "@/auth/auth-context";

type ApplicationCatalog = {
  applications: ApplicationResponse[];
  error: string | null;
  loading: boolean;
  refresh: () => Promise<void>;
};

const ApplicationCatalogContext = createContext<ApplicationCatalog | null>(null);

export function ApplicationCatalogProvider({ children }: { children: ReactNode }): ReactNode {
  const { applications: authenticatedApplications, status } = useAuth();
  const generation = useRef(0);
  const request = useRef<AbortController | null>(null);
  const [refreshedApplications, setRefreshedApplications] = useState<ApplicationResponse[] | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async (): Promise<void> => {
    if (status !== "connected") return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const currentGeneration = ++generation.current;
    setLoading(true);
    setError(null);
    try {
      const response = await listApplications({ signal: controller.signal, throwOnError: true });
      if (controller.signal.aborted || generation.current !== currentGeneration) return;
      setRefreshedApplications(response.data.items ?? []);
    } catch (reason) {
      if (controller.signal.aborted || generation.current !== currentGeneration) return;
      setError(reason instanceof Problem ? reason.title : "Unable to load applications");
    } finally {
      if (generation.current === currentGeneration) {
        request.current = null;
        setLoading(false);
      }
    }
  }, [status]);

  useEffect(() => {
    if (status === "connected") return;
    generation.current++;
    request.current?.abort();
    request.current = null;
    setRefreshedApplications(null);
    setError(null);
    setLoading(false);
  }, [status]);

  useEffect(
    () => () => {
      generation.current++;
      request.current?.abort();
    },
    [],
  );

  const applications = refreshedApplications ?? authenticatedApplications;
  return (
    <ApplicationCatalogContext value={{ applications, error, loading, refresh }}>
      {children}
    </ApplicationCatalogContext>
  );
}

export function useApplicationCatalog(): ApplicationCatalog {
  const value = useContext(ApplicationCatalogContext);
  if (value === null) {
    throw new Error("useApplicationCatalog must be used within ApplicationCatalogProvider");
  }
  return value;
}
