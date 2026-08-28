import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";

/** Poll an async fetcher on an interval; exposes an immediate refresh. */
export function usePolled<T>(
  fetcher: () => Promise<T>,
  intervalMs: number,
): {
  data: T | undefined;
  refresh: () => void;
  error: string | undefined;
} {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<string>();
  const fetcherRef = useRef(fetcher);
  useEffect(() => {
    fetcherRef.current = fetcher;
  });
  // Guards: skip if a request is still in flight (slow VM boots stacked polls
  // otherwise), drop stale responses (a slow earlier reply must not overwrite
  // a newer one), and ignore results after unmount.
  const inFlight = useRef(false);
  const gen = useRef(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const refresh = useCallback(() => {
    if (inFlight.current) return;
    inFlight.current = true;
    const myGen = ++gen.current;
    fetcherRef
      .current()
      .then((d) => {
        if (!mounted.current || myGen !== gen.current) return;
        setData(d);
        setError(undefined);
      })
      .catch((e: Error) => {
        if (!mounted.current || myGen !== gen.current) return;
        setError(e.message);
      })
      .finally(() => {
        inFlight.current = false;
      });
  }, []);

  useEffect(() => {
    refresh();
    // A hidden tab keeps its data but stops generating load (the dashboard
    // alone would otherwise sustain N×M polls forever in a background tab);
    // refresh immediately on return so the view is fresh when seen.
    const t = setInterval(() => {
      if (typeof document !== "undefined" && document.hidden) return;
      refresh();
    }, intervalMs);
    const onVisible = () => {
      if (!document.hidden) refresh();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(t);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refresh, intervalMs]);

  return { data, refresh, error };
}

// v2 key ON PURPOSE: the pre-v1.3 hook wrote 'styrman-theme' unconditionally
// (dark default), so every existing browser holds an explicit 'dark' that
// would silently disable the new "system" default if honored.
const THEME_KEY = "styrman-theme-v2";

export type ThemePref = "light" | "dark" | "system";

// ─── Module-level theme store ─────────────────────────────────────────────
// One store, many consumers (sidebar radio, settings, terminal, editor): a
// change anywhere must reskin everything, including the live xterm palette.
// Per-component state proved wrong — instances disagreed (review finding).

const storedPref = (): ThemePref => {
  try {
    const v = localStorage.getItem(THEME_KEY);
    return v === "light" || v === "dark" ? v : "system";
  } catch {
    return "system";
  }
};

let themePref: ThemePref = storedPref();
const themeListeners = new Set<() => void>();

function setThemePref(p: ThemePref): void {
  themePref = p;
  if (p === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = p;
  try {
    if (p === "system") localStorage.removeItem(THEME_KEY);
    else localStorage.setItem(THEME_KEY, p);
  } catch {
    /* private mode */
  }
  themeListeners.forEach((l) => l());
}

const subscribeTheme = (l: () => void) => {
  themeListeners.add(l);
  return () => {
    themeListeners.delete(l);
  };
};

// OS scheme as a second external input (live while pref === 'system').
const sysDarkMq = () =>
  typeof matchMedia !== "undefined"
    ? matchMedia("(prefers-color-scheme: dark)")
    : undefined;
const subscribeSysDark = (l: () => void) => {
  const mq = sysDarkMq();
  mq?.addEventListener("change", l);
  return () => mq?.removeEventListener("change", l);
};
const getSysDark = () => sysDarkMq()?.matches ?? false;

/**
 * Three-state theme (Cloud Console pattern): explicit light/dark stamps
 * data-theme on <html>; "system" (the default) stamps nothing and follows
 * the OS live. `resolved` is what is actually on screen — use it for
 * anything that needs a concrete palette (xterm, CodeMirror).
 *
 * Server snapshots return the prerender's view (system/light) so hydration
 * matches the shell; the stored preference applies right after mount.
 */
export function useTheme(): {
  pref: ThemePref;
  resolved: "dark" | "light";
  setPref: (p: ThemePref) => void;
} {
  const pref = useSyncExternalStore(
    subscribeTheme,
    () => themePref,
    () => "system" as ThemePref,
  );
  const sysDark = useSyncExternalStore(
    subscribeSysDark,
    getSysDark,
    () => false,
  );
  return {
    pref,
    resolved: pref === "system" ? (sysDark ? "dark" : "light") : pref,
    setPref: setThemePref,
  };
}

/** Reactive media query (drawer-vs-rail breakpoints and the like). */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    useCallback(
      (l: () => void) => {
        const mq = matchMedia(query);
        mq.addEventListener("change", l);
        return () => mq.removeEventListener("change", l);
      },
      [query],
    ),
    () => matchMedia(query).matches,
    () => false,
  );
}
