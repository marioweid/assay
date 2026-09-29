import { createContext, useContext, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

import { configureClient } from "@/api/client";
import { Problem } from "@/api/errors";
import { getServerInfo, listApplications } from "@/api/generated/sdk.gen";
import type { ApplicationResponse } from "@/api/generated/types.gen";

const storageKey = "assay.admin-token.v1";
type ConnectionState = {
  applications: ApplicationResponse[];
  error: string | null;
  status: "checking" | "connected" | "disconnected";
  localMode: boolean | null;
};
type AuthValue = ConnectionState & {
  connect: (token: string) => Promise<void>;
  disconnect: () => void;
  retry: () => Promise<void>;
};
const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }): ReactNode {
  const connection = useConnection();
  return <AuthContext value={connection}>{children}</AuthContext>;
}

function useConnection(): AuthValue {
  const token = useRef<string | null>(null);
  const request = useRef<AbortController | null>(null);
  const [state, setState] = useState<ConnectionState>({
    applications: [],
    error: null,
    status: "checking",
    localMode: null,
  });

  function begin(): AbortController {
    request.current?.abort();
    request.current = new AbortController();
    setState((value) => ({ ...value, applications: [], error: null, status: "checking" }));
    return request.current;
  }

  function disconnect(): void {
    request.current?.abort();
    token.current = null;
    localStorage.removeItem(storageKey);
    setState((value) => ({ ...value, applications: [], error: null, status: "disconnected" }));
  }

  function fail(reason: unknown, controller: AbortController): void {
    if (controller.signal.aborted) return;
    token.current = null;
    if (reason instanceof Problem && reason.status === 401) localStorage.removeItem(storageKey);
    setState((value) => ({
      ...value,
      applications: [],
      status: "disconnected",
      error: connectionError(reason),
    }));
  }

  async function connect(candidate: string | null): Promise<void> {
    const controller = begin();
    token.current = candidate;
    try {
      const { data } = await listApplications({ signal: controller.signal, throwOnError: true });
      if (controller.signal.aborted) return;
      if (candidate !== null) localStorage.setItem(storageKey, candidate);
      setState((value) => ({ ...value, applications: data.items ?? [], status: "connected" }));
    } catch (reason) {
      fail(reason, controller);
    }
  }

  async function initialize(): Promise<void> {
    const controller = begin();
    token.current = null;
    setState((value) => ({ ...value, localMode: null }));
    try {
      const { data } = await getServerInfo({ signal: controller.signal, throwOnError: true });
      if (controller.signal.aborted) return;
      if (typeof data?.local_mode !== "boolean")
        throw new Error("Invalid server authentication mode. Reload or check the server version.");
      setState((value) => ({ ...value, localMode: data.local_mode }));
      if (data.local_mode) {
        localStorage.removeItem(storageKey);
        await connect(null);
      } else {
        const stored = localStorage.getItem(storageKey);
        if (stored !== null) await connect(stored);
        else setState((value) => ({ ...value, status: "disconnected" }));
      }
    } catch (reason) {
      fail(reason, controller);
    }
  }

  useEffect(() => {
    configureClient(
      () => token.current,
      () => {
        disconnect();
        setState((value) => ({
          ...value,
          error: "Admin token was rejected. Enter a valid token to reconnect.",
        }));
      },
    );
    void initialize();
    return () => request.current?.abort();
  }, []);
  return { ...state, connect, disconnect, retry: initialize };
}

function connectionError(reason: unknown): string {
  if (reason instanceof Problem) return reason.title;
  return reason instanceof Error ? reason.message : "Unable to connect to Assay";
}

export function useAuth(): AuthValue {
  const value = useContext(AuthContext);
  if (value === null) throw new Error("useAuth must be used within AuthProvider");
  return value;
}
