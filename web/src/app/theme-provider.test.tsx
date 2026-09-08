import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ThemeProvider, useTheme, resolveTheme } from "@/app/theme-provider";

const storageKey = "assay.theme.v1";

function ThemeProbe(): React.ReactElement {
  const theme = useTheme();
  return (
    <div>
      <output data-testid="preference">{theme.preference}</output>
      <button onClick={() => theme.setPreference("dark")}>dark</button>
      <button onClick={() => theme.setPreference("system")}>system</button>
    </div>
  );
}

type MediaQueryListener = (event: { matches: boolean }) => void;

function installMatchMedia(dark: boolean): {
  listeners: Set<MediaQueryListener>;
  dispatch: (matches: boolean) => void;
} {
  const listeners = new Set<MediaQueryListener>();
  const dispatch = (matches: boolean): void => {
    for (const listener of listeners) listener({ matches });
  };
  const mediaQuery = {
    get matches(): boolean {
      return dark;
    },
    addEventListener: (_: string, listener: MediaQueryListener): void => {
      listeners.add(listener);
    },
    removeEventListener: (_: string, listener: MediaQueryListener): void => {
      listeners.delete(listener);
    },
  };
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue(mediaQuery));
  return { listeners, dispatch };
}

function changeTheme(media: { dispatch: (matches: boolean) => void }, matches: boolean): void {
  act(() => media.dispatch(matches));
}

describe("resolveTheme", () => {
  it("maps each preference to a concrete theme", () => {
    expect(resolveTheme("system", "dark")).toBe("dark");
    expect(resolveTheme("system", "light")).toBe("light");
    expect(resolveTheme("light", "dark")).toBe("light");
    expect(resolveTheme("dark", "light")).toBe("dark");
  });
});

describe("ThemeProvider", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    document.documentElement.style.colorScheme = "";
    vi.unstubAllGlobals();
  });

  it("applies the saved preference on mount", async () => {
    localStorage.setItem(storageKey, "light");
    render(
      <ThemeProvider>
        <ThemeProbe />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("preference")).toHaveTextContent("light");
    expect(document.documentElement["dataset"]["theme"]).toBe("light");
  });

  it("follows the system theme in system mode and updates on change", async () => {
    const media = installMatchMedia(true);
    render(
      <ThemeProvider>
        <ThemeProbe />
      </ThemeProvider>,
    );
    expect(document.documentElement["dataset"]["theme"]).toBe("dark");
    changeTheme(media, false);
    expect(document.documentElement["dataset"]["theme"]).toBe("light");
  });

  it("stores an explicit preference and ignores later system changes", async () => {
    const media = installMatchMedia(true);
    render(
      <ThemeProvider>
        <ThemeProbe />
      </ThemeProvider>,
    );
    expect(document.documentElement["dataset"]["theme"]).toBe("dark");
    await userEvent.click(screen.getByRole("button", { name: "dark" }));
    expect(localStorage.getItem(storageKey)).toBe("dark");
    changeTheme(media, false);
    expect(document.documentElement["dataset"]["theme"]).toBe("dark");
  });

  it("continues in system mode when storage throws", async () => {
    installMatchMedia(false);
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage unavailable");
    });
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("storage unavailable");
    });
    expect(() =>
      render(
        <ThemeProvider>
          <ThemeProbe />
        </ThemeProvider>,
      ),
    ).not.toThrow();
    await userEvent.click(screen.getByRole("button", { name: "dark" }));
    expect(setItem).toHaveBeenCalled();
    expect(document.documentElement["dataset"]["theme"]).toBe("dark");
  });

  it("never turns a stored value into markup or an unsafe attribute", async () => {
    localStorage.setItem(storageKey, '"><img src=x onerror=alert(1)>');
    installMatchMedia(false);
    render(
      <ThemeProvider>
        <ThemeProbe />
      </ThemeProvider>,
    );
    expect(document.documentElement["dataset"]["theme"]).toBe("light");
    expect(screen.getByTestId("preference")).toHaveTextContent("system");
    expect(document.documentElement.innerHTML).not.toContain("onerror");
  });
});
