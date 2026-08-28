import {
  createRootRoute,
  HeadContent,
  Outlet,
  Scripts,
} from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Sidebar } from "../components/Sidebar";
import { ToastHost } from "../components/ui";
import { CrewMark, MenuIcon } from "../components/icons";
import { NotificationBell } from "../components/NotificationBell";
import appCss from "../index.css?url";

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1.0" },
      { title: "crew" },
    ],
    links: [
      { rel: "stylesheet", href: appCss },
      { rel: "icon", type: "image/svg+xml", href: "/favicon.svg" },
    ],
    scripts: [
      {
        // FOWT prevention: stamp an explicit theme choice before first paint;
        // no stored choice = system (un-stamped, media query decides).
        // v2 key — see lib/hooks.ts for why the legacy key is ignored.
        children: `try{var t=localStorage.getItem('styrman-theme-v2');if(t==='light'||t==='dark')document.documentElement.dataset.theme=t}catch(e){}`,
      },
    ],
  }),
  component: RootDocument,
});

function RootDocument() {
  // Mobile/tablet: the sidebar becomes an overlay drawer behind a top bar.
  // Desktop (md+) keeps the permanent rail; the top bar disappears.
  const [navOpen, setNavOpen] = useState(false);
  useEffect(() => {
    if (!navOpen) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setNavOpen(false);
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [navOpen]);
  return (
    <html lang="en">
      <head>
        <HeadContent />
      </head>
      <body>
        <div className="flex h-full flex-col md:flex-row">
          <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-bg-raised px-3 md:hidden">
            <button
              aria-label="Open navigation"
              className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-text-dim hover:bg-bg-hover hover:text-text"
              onClick={() => setNavOpen(true)}
            >
              <MenuIcon />
            </button>
            <CrewMark className="h-5 w-5" />
            <span className="text-sm font-semibold">crew</span>
            <div className="ml-auto">
              <NotificationBell />
            </div>
          </header>
          {navOpen && (
            <div
              className="fixed inset-0 z-40 bg-black/40 md:hidden"
              onClick={() => setNavOpen(false)}
            />
          )}
          <Sidebar open={navOpen} onNavigate={() => setNavOpen(false)} />
          <main className="flex min-h-0 min-w-0 flex-1">
            <Outlet />
          </main>
        </div>
        <ToastHost />
        <Scripts />
      </body>
    </html>
  );
}
