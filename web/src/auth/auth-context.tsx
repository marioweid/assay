import { createContext, startTransition, useContext, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

import { configureClient } from "@/api/client";
import { Problem } from "@/api/errors";
import { listApplications } from "@/api/generated/sdk.gen";
import type { ApplicationResponse } from "@/api/generated/types.gen";

const storageKey = "assay.admin-token.v1";

type AuthStatus = "checking" | "connected" | "disconnected";

type AuthValue = {
  applications: ApplicationResponse[];
  connect: (token: string) => Promise<void>;
  disconnect: () => void;
  error: string | null;
  status: AuthStatus;
};

const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }): ReactNode {
  const storedToken = useRef(localStorage.getItem(storageKey));
  const token = useRef<string | null>(null);
  const [status, setStatus] = useState<AuthStatus>(
    storedToken.current ? "checking" : "disconnected",
  );
  const [applications, setApplications] = useState<ApplicationResponse[]>([]);
  const [error, setError] = useState<string | null>(null);

  async function connect(candidate: string): Promise<void> {
    token.current = candidate;
    setStatus("checking");
    setError(null);
    try {
      const response = await listApplications({ throwOnError: true });
      localStorage.setItem(storageKey, candidate);
      startTransition(() => {
        setApplications(response.data.items ?? []);
        setStatus("connected");
      });
    } catch (reason) {
      token.current = null;
      if (reason instanceof Problem && reason.status === 401) {
        localStorage.removeItem(storageKey);
      }
      setStatus("disconnected");
      setError(reason instanceof Problem ? reason.title : "Unable to connect to Assay");
    }
  }

  function disconnect(): void {
    token.current = null;
    localStorage.removeItem(storageKey);
    setApplications([]);
    setError(null);
    setStatus("disconnected");
  }

  useEffect(() => {
    configureClient(() => token.current);
    if (storedToken.current !== null) {
      void connect(storedToken.current);
    }
  }, []);

  return (
    <AuthContext value={{ applications, connect, disconnect, error, status }}>
      {children}
    </AuthContext>
  );
}

export function useAuth(): AuthValue {
  const value = useContext(AuthContext);
  if (value === null) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return value;
}
