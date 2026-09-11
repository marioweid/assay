import { createContext, useContext, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

import { configureClient } from "@/api/client";
import { Problem } from "@/api/errors";
import { listApplications } from "@/api/generated/sdk.gen";

const storageKey = "assay.admin-token.v1";

type AuthStatus = "checking" | "connected" | "disconnected";

type AuthValue = {
  connect: (token: string) => Promise<void>;
  disconnect: () => void;
  error: string | null;
  status: AuthStatus;
};

const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }): ReactNode {
  const token = useRef<string | null>(null);
  const generation = useRef(0);
  const connectRequest = useRef<AbortController | null>(null);
  const storedToken = useRef(localStorage.getItem(storageKey));
  const [status, setStatus] = useState<AuthStatus>(
    storedToken.current === null ? "disconnected" : "checking",
  );
  const [error, setError] = useState<string | null>(null);

  function disconnect(): void {
    generation.current++;
    connectRequest.current?.abort();
    connectRequest.current = null;
    token.current = null;
    localStorage.removeItem(storageKey);
    setError(null);
    setStatus("disconnected");
  }

  async function connect(candidate: string): Promise<void> {
    const currentGeneration = ++generation.current;
    connectRequest.current?.abort();
    const controller = new AbortController();
    connectRequest.current = controller;
    token.current = candidate;
    setStatus("checking");
    setError(null);
    try {
      await listApplications({ signal: controller.signal, throwOnError: true });
      if (controller.signal.aborted || generation.current !== currentGeneration) return;
      localStorage.setItem(storageKey, candidate);
      setStatus("connected");
    } catch (reason) {
      if (controller.signal.aborted || generation.current !== currentGeneration) return;
      token.current = null;
      if (reason instanceof Problem && reason.status === 401) localStorage.removeItem(storageKey);
      setStatus("disconnected");
      setError(reason instanceof Problem ? reason.title : "Unable to connect to Assay");
    } finally {
      if (generation.current === currentGeneration) connectRequest.current = null;
    }
  }

  useEffect(() => {
    configureClient(() => token.current, disconnect);
    if (storedToken.current !== null) void connect(storedToken.current);
    return () => {
      generation.current++;
      connectRequest.current?.abort();
    };
  }, []);

  return <AuthContext value={{ connect, disconnect, error, status }}>{children}</AuthContext>;
}

export function useAuth(): AuthValue {
  const value = useContext(AuthContext);
  if (value === null) throw new Error("useAuth must be used within AuthProvider");
  return value;
}
