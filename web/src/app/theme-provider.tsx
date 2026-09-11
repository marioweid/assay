import { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";

export type ThemePreference = "system" | "light" | "dark";

export type ThemeValue = {
  preference: ThemePreference;
  setPreference: (preference: ThemePreference) => void;
};

const storageKey = "assay.theme.v1";
const preferences = new Set<ThemePreference>(["system", "light", "dark"]);

const ThemeContext = createContext<ThemeValue | null>(null);

function storedPreference(): ThemePreference {
  try {
    const stored = localStorage.getItem(storageKey);
    if (stored !== null && preferences.has(stored as ThemePreference)) {
      return stored as ThemePreference;
    }
  } catch {
    // Storage may be unavailable; fall back to the system theme.
  }
  return "system";
}

function systemTheme(): "light" | "dark" {
  if (typeof window.matchMedia === "function") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  return "light";
}

export function resolveTheme(
  preference: ThemePreference,
  system: "light" | "dark",
): "light" | "dark" {
  if (preference === "system") return system;
  return preference;
}

export function ThemeProvider({ children }: { children: ReactNode }): ReactNode {
  const [preference, setPreferenceState] = useState<ThemePreference>(storedPreference);
  const [system, setSystem] = useState<"light" | "dark">(systemTheme);
  const resolved = resolveTheme(preference, system);

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia("(prefers-color-scheme: dark)");
    if (typeof query.addEventListener !== "function") return;
    const apply = (event: MediaQueryListEvent): void => setSystem(event.matches ? "dark" : "light");
    query.addEventListener("change", apply);
    return () => query.removeEventListener("change", apply);
  }, []);

  useEffect(() => {
    document.documentElement["dataset"]["theme"] = resolved;
    document.documentElement.style.colorScheme = resolved;
  }, [resolved]);

  function setPreference(next: ThemePreference): void {
    setPreferenceState(next);
    try {
      localStorage.setItem(storageKey, next);
    } catch {
      // Keep the in-memory preference when persistent storage is unavailable.
    }
  }

  return <ThemeContext value={{ preference, setPreference }}>{children}</ThemeContext>;
}

export function useTheme(): ThemeValue {
  const value = useContext(ThemeContext);
  if (value === null) throw new Error("useTheme must be used within ThemeProvider");
  return value;
}
