import {
  createRootRoute,
  HeadContent,
  Outlet,
  Scripts,
} from "@tanstack/react-router";
import { Sidebar } from "../components/Sidebar";
import appCss from "../index.css?url";

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1.0" },
      { title: "styrman" },
    ],
    links: [{ rel: "stylesheet", href: appCss }],
    scripts: [
      {
        // FOWT prevention: apply the persisted theme before first paint.
        children: `try{if(localStorage.getItem('styrman-theme')==='light')document.documentElement.dataset.theme='light'}catch(e){}`,
      },
    ],
  }),
  component: RootDocument,
});

function RootDocument() {
  return (
    <html lang="en" data-theme="dark">
      <head>
        <HeadContent />
      </head>
      <body>
        <div className="flex h-full">
          <Sidebar />
          <main className="flex min-w-0 flex-1">
            <Outlet />
          </main>
        </div>
        <Scripts />
      </body>
    </html>
  );
}
