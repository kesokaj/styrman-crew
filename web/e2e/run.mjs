// styrman UI end-to-end test (Playwright, headless Chromium).
// Drives the real UI against a running server: sidebar, setup banner,
// project creation (boots a microVM), terminals (xterm uses the DOM
// renderer, so terminal text is assertable), the side panel's Files / Ports
// / History / Diff tabs, settings, deep links, and cleanup.
//
// Usage: node web/e2e/run.mjs [--base http://127.0.0.1:8080] [--shots /tmp/shots]
import { readFileSync } from "node:fs";
import { chromium } from "playwright";

const arg = (name, def) => {
  const i = process.argv.indexOf(name);
  return i > -1 ? process.argv[i + 1] : def;
};
const BASE = arg("--base", "http://127.0.0.1:8080");
const SHOTS = arg("--shots", "/tmp/styrman-shots");
const PROJECT = "e2e-ui";

let passed = 0;
let failed = 0;
let skipped = 0;
const skip = (label, why) => {
  console.log(`SKIP ${label} (${why})`);
  skipped++;
};
const ok = (label) => {
  passed++;
  console.log(`PASS ${label}`);
};
const bad = (label, extra = "") => {
  failed++;
  console.log(`FAIL ${label} ${extra}`);
};
const check = async (label, fn) => {
  try {
    await fn();
    ok(label);
  } catch (e) {
    bad(label, `— ${e.message.split("\n")[0]}`);
  }
};

// Pre-clean any leftover project from an aborted previous run.
await fetch(`${BASE}/api/projects/${PROJECT}?wipe=true`, {
  method: "DELETE",
}).catch(() => {});

const browser = await chromium.launch();
// An explicit context, not browser.newPage(): the terminal-recovery checks
// open a second and a third page in the SAME session, and Playwright refuses
// context.newPage() on the context browser.newPage() creates implicitly.
const context = await browser.newContext({ viewport: { width: 1440, height: 860 } });
const page = await context.newPage();
// Surface uncaught client errors as suite failures (they were invisible).
// React hydration warnings (#418/#423/#425) are recoverable — React re-renders
// on the client — and pre-date this suite; closed as known-benign in ADR-045
// (client-only state flows through useSyncExternalStore with server
// snapshots). Reported separately so a genuine runtime error still fails.
const HYDRATION = /Minified React error #(418|423|425)/;
const pageErrors = [];
const hydrationWarns = [];
const record = (s) => (HYDRATION.test(s) ? hydrationWarns : pageErrors).push(s);
page.on("pageerror", (e) => record(String(e)));
page.on("console", (m) => {
  if (m.type() !== "error") return;
  // Include the URL. The browser's own text for a failed request is just
  // "Failed to load resource: the server responded with a status of 404",
  // which names neither the endpoint nor the caller — useless in a suite this
  // size (cost a whole debugging run in v1.22.0).
  const url = m.location()?.url;
  record(url ? `${m.text()} [${url}]` : m.text());
});
page.setDefaultTimeout(15000);
const shot = (name) => page.screenshot({ path: `${SHOTS}/${name}.png` });
const panel = () => page.locator("aside").last();
// File clicks are scoped to the listing: the Recent chips strip (2026-09-01)
// repeats file basenames inside the same aside, and an unscoped getByText
// would strict-mode-collide with them.
const fileList = () => panel().getByTestId("file-list");
/** Wait until the MAIN pane's rendered text contains `needle`. */
const mainContains = (needle, timeout = 20000) =>
  page.waitForFunction(
    (n) => (document.querySelector("main")?.innerText ?? "").includes(n),
    needle,
    { timeout },
  );

try {
  // ── Home ────────────────────────────────────────────────────────────────
  await page.goto(BASE + "/");
  await check("app shell renders", () =>
    page.waitForSelector('aside:has-text("crew")'),
  );
  await check("projects section present", () =>
    page.waitForSelector('nav:has-text("Projects")'),
  );
  // The banner only exists until setup is completed — on an instance where
  // the operator finished onboarding, its absence is the correct behavior.
  const setupDone = await page.evaluate(() =>
    fetch("/api/setup/status")
      .then((r) => r.json())
      .then((s) => s.done),
  );
  await check("setup banner matches setup state", async () => {
    if (setupDone) {
      if (await page.locator("text=First-time setup").count())
        throw new Error("banner shown despite setup done");
      return;
    }
    await page.waitForSelector("text=First-time setup");
    await page.waitForSelector("text=claude");
  });
  await shot("01-home");

  // ── New-project dialog: a clone takes the REPO's name ───────────────────
  // Operator expectation (2026-09-02): "Clone a repo = use the same name of
  // the repo who is cloned". The name field auto-fills from the URL until the
  // user types their own, which then wins. No project is created here — the
  // dialog is closed again before the real create below.
  await page.click("aside >> text=New project");
  await check("a clone URL carrying a token is refused", async () => {
    await page.click('button:has-text("Clone a repo")');
    await page.fill('input[aria-label="Repository URL to clone"]',
      "https://user:ghp_notarealtoken@github.com/o/r.git");
    await page.waitForSelector("text=Remove the token from the URL", { timeout: 5000 });
    const armed = await page.locator('button:has-text("Create")').isEnabled();
    if (armed) throw new Error("Create stayed enabled for a credential-bearing URL");
    await page.fill('input[aria-label="Repository URL to clone"]', "");
  });
  await check("clone mode derives the project name from the repo URL", async () => {
    await page.click('button:has-text("Clone a repo")');
    await page.fill('input[aria-label="Repository URL to clone"]',
      "git@github.com:kesokaj/styrman-harness.git");
    await page.waitForFunction(
      () => document.querySelector("#np-name")?.value === "styrman-harness",
      null, { timeout: 5000 });
  });
  await check("a name the user types overrides the derived one", async () => {
    await page.fill("#np-name", "my-own-name");
    await page.fill('input[aria-label="Repository URL to clone"]',
      "https://github.com/kesokaj/other-repo.git");
    await page.waitForTimeout(200);
    const v = await page.inputValue("#np-name");
    if (v !== "my-own-name") throw new Error(`name field = ${v}, want my-own-name`);
  });
  await shot("02a-clone-name-derivation");
  await page.keyboard.press("Escape");

  // ── Create a project (boots a real microVM) ─────────────────────────────
  await page.click("aside >> text=New project");
  await page.fill("#np-name", PROJECT);
  await shot("02-new-project-modal");
  await page.click('button:has-text("Create")');
  await check("navigates into a terminal after create", () =>
    page.waitForURL(`**/p/*/1`, { timeout: 60000 }),
  );
  // Resolve the project's UUID for URL-based assertions.
  const PID = await (async () => {
    const r = await (await fetch(`${BASE}/api/projects`)).json();
    return r.projects.find((p) => p.name === PROJECT)?.id ?? "missing";
  })();
  await check("URL uses the project UUID", async () => {
    if (!page.url().includes(`/p/${PID}/`))
      throw new Error(`url=${page.url()} pid=${PID}`);
  });
  // Terminals get GENERATED friendly names since v1.3 — assert a non-empty
  // title via the rename affordance instead of a fixed string.
  await check("terminal header shows a generated title", async () => {
    await page.waitForSelector('main header button[title="Rename terminal"]');
    const title = (
      await page
        .locator('main header button[title="Rename terminal"]')
        .textContent()
    )?.trim();
    if (!title) throw new Error("empty terminal title");
  });
  await check("project listed running (green dot)", () =>
    page.waitForSelector(
      `aside div:has-text("${PROJECT}") span[title="running"]`,
    ),
  );

  // ── The terminal is a shell; typing reaches the microVM ─────────────────
  // The userprofile prompt ends in ❯ (internal/dotfiles); $ covers a home
  // that predates it or a stock-bash fallback.
  await check("shell prompt appears", () =>
    page.waitForFunction(
      () =>
        /[$❯]/.test(document.querySelector(".xterm-rows")?.textContent ?? ""),
      { timeout: 45000 },
    ),
  );
  await page.locator(".xterm textarea").first().focus();
  await page.keyboard.type("echo styrman-e2e-$((21*2))", { delay: 15 });
  await page.keyboard.press("Enter");
  await check("typed command executes in microVM", () =>
    page.waitForFunction(() =>
      [...document.querySelectorAll(".xterm-rows")].some((r) =>
        (r.textContent ?? "").includes("styrman-e2e-42"),
      ),
    ),
  );
  // (The URL "link bar" was removed in v1.3 — copy-on-select superseded it.)

  await check("both agents are installed", async () => {
    // Assert the OUTPUT sentinel, not the echoed command — otherwise this
    // passes even if neither agent exists (F10).
    await page.keyboard.type("echo agents=$(command -v claude agy | wc -l)", {
      delay: 10,
    });
    await page.keyboard.press("Enter");
    await page.waitForFunction(
      () =>
        [...document.querySelectorAll(".xterm-rows")].some((r) =>
          (r.textContent ?? "").includes("agents=2"),
        ),
      null,
      { timeout: 30000 },
    );
  });
  await shot("03-terminal");

  // ── Side panel: Files ───────────────────────────────────────────────────
  await check("files tab lists the workspace", async () => {
    await panel().getByText("Files", { exact: true }).click();
    await page.waitForSelector("aside >> text=.git");
  });
  await check("home scope switches root", async () => {
    await panel().getByText("Home", { exact: true }).click();
    // Per-project home (ADR-031), seeded by the image entrypoint on boot:
    // .bashrc is always there; .claude only after a real sign-in.
    await page.waitForSelector("aside >> text=.bashrc");
  });
  await check("project scope returns", async () => {
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForSelector("aside >> text=.git");
  });
  // Drill into a subdirectory — guards the double-prefix regression where a
  // scoped entry's path was re-prefixed and 400'd on the second level.
  await check("files drill-in into a subdir works", async () => {
    await page.locator(".xterm textarea").first().focus();
    await page.keyboard.type("mkdir -p subdir && echo hi > subdir/nested.txt");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(800);
    await panel().getByText("Files", { exact: true }).click();
    await page.waitForTimeout(500);
    await fileList().getByText("subdir", { exact: true }).click();
    await page.waitForSelector("aside >> text=nested.txt");
  });

  // ── Path navigation (v1.22.0): back/forward/up, none of which existed ────
  await check("explorer back/forward/up navigate and disable at the ends", async () => {
    const back = panel().getByRole("button", { name: "Back" });
    const fwd = panel().getByRole("button", { name: "Forward" });
    const up = panel().getByRole("button", { name: "Up one level" });
    // We are inside subdir, reached from the root: nothing ahead.
    if (await fwd.isEnabled())
      throw new Error("Forward enabled with no forward history");
    await back.click();
    await page.waitForSelector("aside >> text=subdir");
    if (!(await fwd.isEnabled()))
      throw new Error("Forward still disabled after going back");
    if (await back.isEnabled())
      throw new Error("Back still enabled at the oldest entry");
    await fwd.click();
    await page.waitForSelector("aside >> text=nested.txt");
    await up.click();
    await page.waitForSelector("aside >> text=subdir");
    // Leave the explorer inside subdir — the editor checks below open
    // nested.txt from this listing and would not find it at the root.
    await fileList().getByText("subdir", { exact: true }).click();
    await page.waitForSelector("aside >> text=nested.txt");
  });

  await check("breadcrumb jumps straight to an ancestor", async () => {
    // The crumb for the project root carries the friendly name; clicking it
    // must land at the root regardless of how deep we are.
    await panel().getByRole("button", { name: "Project root" }).click();
    await page.waitForSelector("aside >> text=subdir");
    await fileList().getByText("subdir", { exact: true }).click();
    await page.waitForSelector("aside >> text=nested.txt");
  });
  // The listing is capped at files.maxListEntries (5000) since 2026-09-22,
  // because the directory is guest-writable and the explorer polls it. The
  // cap is applied to raw directory order BEFORE sorting, so which entries
  // are missing is arbitrary — a cut listing that rendered as a complete one
  // would be a way to keep a file out of a human's sight. The notice is the
  // only thing that makes truncation visible, and it has no unit test:
  // FileExplorer.tsx has none and the project has no component-testing
  // framework, so this check is its only coverage (security review of the
  // cap, 2026-09-22).
  //
  // Placed here, after the breadcrumb check, because the explorer's position
  // is load-bearing for the editor checks below: this one navigates away and
  // puts it back inside subdir before it returns.
  await check("a directory past the listing cap says so", async () => {
    await page.locator(".xterm textarea").first().focus();
    // Assert an OUTPUT sentinel, never the echoed command — the convention
    // this file sets at :190 and :202-206. A wait on a literal the typed
    // line already contains is satisfied by the terminal's own echo, before
    // the command has run at all: the first version of this check waited for
    // "BIGDONE" while typing `echo BIGDONE`, and passed only because the
    // 30 s selector wait below absorbed the 5200 touches (release review,
    // 2026-09-22). The count is computed by the shell, so the sentinel
    // cannot appear until the files exist — and it proves how many do.
    await page.keyboard.type(
      "mkdir -p big && (cd big && seq 1 5200 | xargs -n1 -P8 touch) && echo BIG-$(ls big | wc -l)-DONE",
      { delay: 10 },
    );
    await page.keyboard.press("Enter");
    await page.waitForFunction(
      () =>
        [...document.querySelectorAll(".xterm-rows")].some((r) =>
          (r.textContent ?? "").includes("BIG-5200-DONE"),
        ),
      null,
      { timeout: 120000 },
    );

    // finally, not a straight line: when this check failed during its own
    // must-fail probe it left the explorer at the wrong path and cascaded
    // into eight later checks, which reads as eight defects instead of one.
    try {
      await panel().getByRole("button", { name: "Project root" }).click();
      await page.waitForSelector("aside >> text=big");
      await fileList().getByText("big", { exact: true }).click();
      await page.waitForSelector("aside >> text=Some are not shown", { timeout: 30000 });

      // Not simply always on: the workspace root has far fewer entries.
      await panel().getByRole("button", { name: "Project root" }).click();
      await page.waitForSelector("aside >> text=subdir");
      if (await panel().getByText("Some are not shown").count()) {
        throw new Error("the truncation notice renders on a small directory too");
      }
    } finally {
      // Restore what the editor checks below expect: inside subdir.
      await panel().getByRole("button", { name: "Project root" }).click().catch(() => {});
      await fileList().getByText("subdir", { exact: true }).click().catch(() => {});
      await page.waitForSelector("aside >> text=nested.txt", { timeout: 15000 }).catch(() => {});
    }
  });

  await shot("04-files");

  // ── Editor pane (2026-09-01): the editor shares the main pane with the
  // terminal instead of opening as a modal. The terminal must stay MOUNTED
  // behind it — TerminalPane closes its WebSocket on unmount, so a toggle that
  // remounted it would silently kill the session on every switch.
  await check("clicking a file opens it in the editor pane", async () => {
    await panel().getByText("Files", { exact: true }).click();
    await page.waitForTimeout(400);
    await fileList().getByText("nested.txt", { exact: true }).click();
    // Editor pane, not a modal: no dialog, and CodeMirror is in the main pane.
    await page.waitForSelector(".cm-content", { timeout: 15000 });
    if (await page.locator('[role="dialog"][aria-label="File viewer"]').count())
      throw new Error("file opened as a modal, expected the pane");
  });
  await check("editor pane shows the file content", async () => {
    await page.waitForSelector(".cm-content >> text=hi", { timeout: 10000 });
  });

  // ── Recents + open-file affordances (2026-09-01): the explorer highlights
  // the file the current terminal's pane holds, a per-project Recent strip
  // reopens files in one click, and the terminal header names the held file.
  await check("explorer highlights the open file", async () => {
    await fileList()
      .locator('[aria-current="true"]')
      .filter({ hasText: "nested.txt" })
      .waitFor({ timeout: 5000 });
  });
  await check("terminal header badge names the held file", async () => {
    const editorToggle = page.getByRole("button", { name: "Editor", exact: true });
    // The visible badge AND its screen-reader description (aria-describedby)
    // both carry the name — assert each on purpose.
    await editorToggle
      .locator("span:not(.sr-only)", { hasText: "nested.txt" })
      .waitFor({ timeout: 5000 });
    await editorToggle
      .locator("span.sr-only", { hasText: "nested.txt" })
      .waitFor({ timeout: 5000 });
  });
  await shot("04c-editor-open"); // the editor pane itself, for design review
  await check("view toggle switches back to the terminal", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.waitForSelector(".xterm", { state: "visible", timeout: 10000 });
  });
  // The real regression guard: the shell is still the SAME live session, so a
  // command typed now still works and the earlier scrollback is intact.
  // NO explicit .focus() here on purpose: switching back to Terminal must
  // restore keyboard focus by itself, or the first keystrokes go to <body>.
  await check("terminal session survived the editor round-trip", async () => {
    await page.keyboard.type("echo pane-roundtrip-ok");
    await page.keyboard.press("Enter");
    await page.waitForSelector(".xterm >> text=pane-roundtrip-ok", {
      timeout: 15000,
    });
  });
  // The terminal must come back at its FULL width. While hidden the
  // ResizeObserver collapses the local xterm, and the recovery was debounced —
  // a full-width terminal came back wrapping at ~12 columns (caught by looking
  // at a screenshot; the typing check above passed the whole time).
  await check("terminal keeps its width across the round-trip", async () => {
    const cols = await page.evaluate(() => {
      const el = document.querySelector(".xterm-screen");
      const row = document.querySelector(".xterm-rows > div");
      if (!el || !row) return 0;
      // Width of the rendered row box vs the screen: a collapsed terminal
      // reports a tiny screen width relative to its container.
      return Math.round(el.getBoundingClientRect().width);
    });
    if (cols < 400)
      throw new Error(`terminal collapsed after the toggle: screen width ${cols}px`);
  });

  await check("toggling back reopens the same file (no reload)", async () => {
    await page.getByRole("button", { name: "Editor", exact: true }).click();
    await page.waitForSelector(".cm-content >> text=hi", { timeout: 10000 });
  });
  await check("word wrap toggle is present", async () => {
    await page.getByRole("button", { name: "Wrap", exact: true }).click();
    await page.waitForTimeout(200);
    await page.getByRole("button", { name: "Wrap", exact: true }).click();
  });
  await check("Dockerfile highlights (basename match, not full path)", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.locator(".xterm textarea").first().focus();
    await page.keyboard.type("printf 'FROM debian:trixie\\nRUN echo hi\\n' > Dockerfile");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(800);
    await panel().getByText("Files", { exact: true }).click();
    // The explorer is still inside subdir/ from the drill-in check; the scope
    // buttons reset both scope AND path back to the workspace root.
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForTimeout(600);
    await fileList().getByText("Dockerfile", { exact: true }).click();
    // With a grammar loaded, CodeMirror wraps tokens in styled spans (the
    // theme generates its own class names, so assert on the spans, not on a
    // `tok-*` class). A file with NO matching grammar renders bare text nodes
    // — which is exactly what the full-path bug produced for Dockerfiles.
    // Content must be the DOCKERFILE's, not the previously open file's — this
    // is the regression that the render-phase reset in useFileEditorState fixes.
    await page.waitForSelector(".cm-content >> text=FROM debian", {
      timeout: 15000,
    });
    await page.waitForSelector(".cm-content .cm-line span", { timeout: 10000 });
  });
  await check("recent strip reopens a file in one click", async () => {
    // Dockerfile is open; nested.txt was opened earlier — jump back to it via
    // its Recent chip and confirm content and the chip's aria-current follow.
    await panel()
      .getByTestId("recent-files")
      .getByLabel("Open recent file subdir/nested.txt", { exact: true })
      .click({ timeout: 7000 });
    // A REAL reopen: the Dockerfile document must be gone and the toolbar must
    // name the reopened file — a lax `text=hi` match was already satisfied by
    // the still-open Dockerfile buffer ("RUN echo hi") (code review 2026-09-02).
    await page
      .locator(".cm-content >> text=FROM debian")
      .waitFor({ state: "detached", timeout: 10000 });
    await page
      .getByTestId("editor-path")
      .filter({ hasText: "subdir/nested.txt" })
      .waitFor({ timeout: 10000 });
    await page.waitForSelector(".cm-content >> text=hi", { timeout: 10000 });
    await panel()
      .getByTestId("recent-files")
      .locator('[aria-current="true"]')
      .filter({ hasText: "nested.txt" })
      .waitFor({ timeout: 5000 });
  });
  // The SAME relative path in both scopes (code review 2026-09-02 — the two
  // worst findings): a cross-scope open must flip the document (the editor
  // reset was keyed on path alone, so Ctrl+S overwrote the other scope's
  // file), and deleting the project copy must leave the home copy's chip AND
  // pane untouched (the pane sweep was path-only and dropped unsaved edits).
  await check("same path in both scopes: content flips, deletes stay scoped", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.waitForTimeout(400);
    await page.keyboard.type("mkdir -p ~/subdir && echo homey > ~/subdir/nested.txt");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(900);
    await panel().getByText("Files", { exact: true }).click();
    await panel().getByText("Home", { exact: true }).click();
    await page.waitForTimeout(600);
    await fileList().getByText("subdir", { exact: true }).click();
    await page.waitForTimeout(400);
    await fileList().getByText("nested.txt", { exact: true }).click();
    await page.waitForSelector(".cm-content >> text=homey", { timeout: 15000 });
    const strip = panel().getByTestId("recent-files");
    // Flip to the PROJECT copy via its chip: its content must replace the
    // home copy's (a path-only editor reset keeps "homey" on screen).
    await strip
      .getByLabel("Open recent file subdir/nested.txt", { exact: true })
      .click();
    await page
      .locator(".cm-content >> text=homey")
      .waitFor({ state: "detached", timeout: 10000 });
    await page.waitForSelector(".cm-content >> text=hi", { timeout: 10000 });
    // …and back to the home copy.
    await strip
      .getByLabel("Open recent file ~/subdir/nested.txt", { exact: true })
      .click();
    await page
      .locator(".cm-content >> text=hi")
      .waitFor({ state: "detached", timeout: 10000 });
    await page.waitForSelector(".cm-content >> text=homey", { timeout: 10000 });
    // Delete the PROJECT copy while the pane holds the HOME copy.
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForTimeout(600);
    await fileList().getByText("subdir", { exact: true }).click();
    await page.waitForTimeout(400);
    await panel()
      .locator("div.group")
      .filter({ hasText: "nested.txt" })
      .first()
      .hover();
    await panel().getByLabel("Delete nested.txt").click();
    await page.getByRole("button", { name: "Delete", exact: true }).click();
    await page.waitForTimeout(1200);
    await strip
      .getByLabel("Open recent file subdir/nested.txt", { exact: true })
      .waitFor({ state: "detached", timeout: 5000 });
    await strip
      .getByLabel("Open recent file ~/subdir/nested.txt", { exact: true })
      .waitFor({ timeout: 5000 });
    // The pane still holds the home copy — a path-only sweep would have
    // cleared it to "No file open".
    await page.waitForSelector(".cm-content >> text=homey", { timeout: 5000 });
    await page
      .getByTestId("editor-path")
      .filter({ hasText: "subdir/nested.txt" })
      .waitFor({ timeout: 5000 });
    await fileList().getByText("../", { exact: true }).click();
    await page.waitForTimeout(400);
  });
  // The read path caps and sanitises whatever storage holds (code review
  // 2026-09-02 mutant): a corrupted 20-entry blob renders exactly the cap.
  // ORDERING: this overwrites the project's real recents and RELOADS the
  // page — it must stay ahead of every dirty-editor check (the route's
  // beforeunload handler would turn the reload into a native dialog) and
  // no later check may rely on the earlier chips.
  await check("recents list is capped on read", async () => {
    const project = new URL(page.url()).pathname.split("/")[2];
    await page.evaluate((p) => {
      const big = Array.from({ length: 20 }, (_, i) => ({
        scopeKind: "project",
        path: `junk${i}.txt`,
      }));
      localStorage.setItem(`styrman.recentFiles.${p}`, JSON.stringify(big));
    }, project);
    await page.reload();
    await page.waitForSelector("aside", { timeout: 15000 });
    await panel().getByText("Files", { exact: true }).click();
    await panel().getByTestId("recent-files").waitFor({ timeout: 10000 });
    const n = await panel()
      .getByTestId("recent-files")
      .getByRole("button")
      .count();
    if (n !== 16)
      throw new Error(`recents strip rendered ${n} chips, cap is 16`);
    // Put the real recents back. This check writes 20 junk entries over the
    // project's actual recents and previously left them there for the rest of
    // the suite, so every later chip assertion ran against junk (2026-09-02
    // review). Restoring is cheaper than reordering the whole section.
    await page.evaluate((p) => {
      localStorage.removeItem(`styrman.recentFiles.${p}`);
    }, project);
    await page.reload();
    await page.waitForSelector("aside", { timeout: 15000 });
  });

  // "Discard and open": editing a file, then clicking another, must prompt and
  // then actually LOAD the second file. This dead-ended on an empty pane in
  // review (the store bailed on an identical object, so the loader never re-ran).
  await check("discard-and-open loads the second file", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.locator(".xterm textarea").first().focus();
    // Two files at the WORKSPACE ROOT (nested.txt lives in subdir/).
    await page.keyboard.type("echo AAA > one.txt; echo BBB > two.txt");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(900);
    await panel().getByText("Files", { exact: true }).click();
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForTimeout(600);
    await fileList().getByText("one.txt", { exact: true }).click();
    await page.waitForSelector(".cm-content >> text=AAA", { timeout: 15000 });
    // Make it dirty.
    await page.locator(".cm-content").first().click();
    await page.keyboard.type("DIRTY");
    await page.waitForTimeout(400);
    // Opening another file must PROMPT, not silently discard.
    await fileList().getByText("two.txt", { exact: true }).click();
    await page.waitForSelector("text=Discard unsaved changes?", { timeout: 10000 });
    await page.getByRole("button", { name: "Discard and open" }).click();
    // ...and the pane must actually show the SECOND file, not sit empty.
    await page.waitForSelector(".cm-content >> text=BBB", { timeout: 15000 });
  });

  // Syntax checking (item 5): the real checker runs in the project VM and its
  // diagnostics land in CodeMirror's lint gutter.
  // An upload that replaces the OPEN file must evict its pane (a stale buffer
  // would overwrite the upload on Save) but keep its Recent chip — the file
  // still exists — and reopening shows the uploaded content (code review
  // 2026-09-02; covers the server-reported saved-names path).
  await check("upload over the open file evicts its pane, keeps its chip", async () => {
    await page.getByTestId("editor-path").filter({ hasText: "two.txt" }).waitFor({ timeout: 10000 });
    await panel()
      .locator('input[type="file"]')
      .setInputFiles({ name: "two.txt", mimeType: "text/plain", buffer: Buffer.from("UPLOADED\n") });
    await page.waitForSelector("text=No file open", { timeout: 10000 });
    const chip = panel()
      .getByTestId("recent-files")
      .getByLabel("Open recent file two.txt", { exact: true });
    await chip.waitFor({ timeout: 5000 });
    await chip.click();
    await page.waitForSelector(".cm-content >> text=UPLOADED", { timeout: 15000 });
  });

  // ── Recent strip: collapsible + resizable grid (v1.22.0) ────────────────
  // ORDERING: these need a NON-EMPTY recents list, so they cannot sit beside
  // the cap check above — that one deliberately clears
  // styrman.recentFiles.<project> and reloads, leaving no strip to assert on
  // (`recents.length > 0` gates the whole thing). By here the upload check has
  // put two.txt back in the list.
  await check("recent strip is a WRAPPING grid, not one scrolling row", async () => {
    await panel().getByText("Files", { exact: true }).click();
    const strip = panel().getByTestId("recent-files");
    await strip.waitFor({ timeout: 10000 });
    const wraps = await strip.evaluate(
      (el) => getComputedStyle(el).flexWrap === "wrap",
    );
    if (!wraps) throw new Error("recents strip does not wrap");
  });

  await check("recent strip collapses, and the choice survives a reload", async () => {
    const toggle = panel().getByRole("button", { name: /^Recent \(\d+\)$/ });
    await toggle.click();
    await panel().getByTestId("recent-files").waitFor({ state: "detached", timeout: 5000 });
    const stored = await page.evaluate(() =>
      localStorage.getItem("styrman.recents.collapsed"),
    );
    if (stored !== "1") throw new Error(`collapsed flag stored as ${stored}`);
    await page.reload();
    await page.waitForSelector("aside", { timeout: 15000 });
    await panel().getByText("Files", { exact: true }).click();
    await page.waitForTimeout(400);
    if (await panel().getByTestId("recent-files").count())
      throw new Error("strip re-expanded after reload");
    // Expand again and leave it that way for anything after this.
    await panel()
      .getByRole("button", { name: /^Recent \(\d+\)$/ })
      .click();
    await panel().getByTestId("recent-files").waitFor({ timeout: 5000 });
    const cleared = await page.evaluate(() =>
      localStorage.getItem("styrman.recents.collapsed"),
    );
    if (cleared !== null)
      throw new Error(`expanded should REMOVE the key, got ${cleared}`);
  });

  await check("a hand-edited height is clamped, not obeyed", async () => {
    await page.evaluate(() =>
      localStorage.setItem("styrman.recents.height", "99999"),
    );
    await page.reload();
    await page.waitForSelector("aside", { timeout: 15000 });
    await panel().getByText("Files", { exact: true }).click();
    const strip = panel().getByTestId("recent-files");
    await strip.waitFor({ timeout: 10000 });
    const h = await strip.evaluate((el) => el.getBoundingClientRect().height);
    if (h > 320) throw new Error(`strip is ${h}px, above the 320px cap`);
    await page.evaluate(() =>
      localStorage.removeItem("styrman.recents.height"),
    );
  });
  await check("syntax check flags a broken shell script", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.locator(".xterm textarea").first().focus();
    await page.keyboard.type("printf 'if [ 1 -eq 1 ]\\nthen\\necho hi\\n' > broken.sh");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(800);
    await panel().getByText("Files", { exact: true }).click();
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForTimeout(600);
    await fileList().getByText("broken.sh", { exact: true }).click();
    await page.waitForSelector(".cm-content >> text=echo hi", { timeout: 15000 });
    await page.waitForSelector(".cm-lint-marker-error", { timeout: 30000 });
  });
  await check("syntax check reports a CLEAN file as clean", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.locator(".xterm textarea").first().focus();
    await page.keyboard.type("printf 'echo hello\\n' > fine.sh");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(800);
    await panel().getByText("Files", { exact: true }).click();
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForTimeout(600);
    await fileList().getByText("fine.sh", { exact: true }).click();
    await page.waitForSelector(".cm-content >> text=echo hello", { timeout: 15000 });
    await page.waitForTimeout(4000); // let the debounced check run
    if (await page.locator(".cm-lint-marker-error").count())
      throw new Error("clean file reported an error");
  });

  // The check/format endpoints write a temp file INTO the workspace so tools
  // find the project's config. They must clean it up — leftovers showed up in
  // the user's file list during development.
  await check("syntax check leaves no temp files in the workspace", async () => {
    // Exercise a PYTHON file too, not just a shell one. py_compile emits a
    // __pycache__ DIRECTORY, which the tmp.* filename pattern below can never
    // match — so this check previously passed while the litter bug was live
    // (verifier finding). Checking both is what makes it a real gate.
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.locator(".xterm textarea").first().focus();
    await page.keyboard.type("printf 'x = 1\\n' > lint.py");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(800);
    await panel().getByText("Files", { exact: true }).click();
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForTimeout(500);
    await fileList().getByText("lint.py", { exact: true }).click();
    await page.waitForSelector(".cm-content >> text=x = 1", { timeout: 15000 });
    await page.waitForTimeout(4000); // let the debounced check run in the VM
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForTimeout(800);
    const listing = await panel().innerText();
    if (/tmp\.[A-Za-z0-9]+\./.test(listing))
      throw new Error(`temp files left in the workspace: ${listing}`);
    if (/__pycache__/.test(listing))
      throw new Error(`__pycache__ left in the workspace: ${listing}`);
  });

  // Deleting a file that is OPEN in the pane must close it — otherwise Save
  // on the stale path recreates what was just deleted (review finding).
  await check("deleting an open file clears the editor pane", async () => {
    await panel().getByText("Files", { exact: true }).click();
    await panel().getByText("Project", { exact: true }).click();
    await page.waitForTimeout(500);
    await fileList().getByText("Dockerfile", { exact: true }).click();
    await page.waitForSelector(".cm-content >> text=FROM debian", {
      timeout: 15000,
    });
    // The row's action buttons are invisible until the row is hovered.
    await panel()
      .locator("div.group")
      .filter({ hasText: "Dockerfile" })
      .first()
      .hover();
    await panel().getByLabel("Delete Dockerfile").click();
    await page.getByRole("button", { name: "Delete", exact: true }).click();
    await page.waitForTimeout(1200);
    // The pane must no longer be offering to save the deleted file.
    await page.getByRole("button", { name: "Editor", exact: true }).click();
    await page.waitForSelector("text=No file open", { timeout: 10000 });
  });
  await page.getByRole("button", { name: "Terminal", exact: true }).click();
  await page.waitForTimeout(600); // let the refit settle before capturing
  await shot("04b-editor-pane");

  // ── Side panel: Ports (start a server in the terminal first) ────────────
  await page.locator(".xterm textarea").first().focus();
  await page.keyboard.type(
    'cd /tmp && echo "<h1>e2e-app</h1>" > index.html && python3 -m http.server 4321 >/dev/null 2>&1 &',
    { delay: 5 },
  );
  await page.keyboard.press("Enter");
  await check("port appears in the ports tab", async () => {
    await panel().getByText("Ports", { exact: true }).click();
    await page.waitForSelector("aside >> text=:4321", { timeout: 25000 });
  });
  // ── Network mode (ADR-067). Two states, no host half: the mode is the
  // bridge interface name the server picks, so there is nothing to stub and
  // nothing that can be "unavailable". The switch is admin-only to turn ON;
  // this e2e user is an admin.
  // The editor mounts its lint gutter only for CHECKABLE_EXTENSIONS
  // (web/src/lib/api.ts), a hand-mirrored copy of the server's checkerFor
  // switch. Read the list from the SOURCE file and ask the server about each
  // one, plus one it does not list, so the two cannot drift silently.
  await check("checkable extensions match the server's checkers", async () => {
    const src = readFileSync(new URL("../src/lib/api.ts", import.meta.url), "utf8");
    const m = /CHECKABLE_EXTENSIONS[^\n]*=\s*new Set\(\[([^\]]*)\]/s.exec(src);
    if (!m) throw new Error("CHECKABLE_EXTENSIONS not found in api.ts");
    const exts = [...m[1].matchAll(/"([a-z0-9]+)"/g)].map((x) => x[1]);
    if (exts.length < 5) throw new Error(`suspiciously short list: ${exts}`);
    const ask = async (ext) => {
      const res = await page.request.post(`${BASE}/api/projects/${PID}/check`, {
        data: { path: `drift-guard.${ext}`, content: "" },
        headers: { "content-type": "application/json" },
        timeout: 60000,
      });
      if (!res.ok()) throw new Error(`check .${ext}: ${res.status()} ${await res.text()}`);
      return (await res.json()).supported;
    };
    for (const ext of exts) {
      if (!(await ask(ext))) throw new Error(`.${ext} is in CHECKABLE_EXTENSIONS but the server has no checker for it`);
    }
    if (await ask("xyzzy")) throw new Error("the server claims a checker for .xyzzy, which the UI does not list");
  });

  await check("network block shows the sandbox default", async () => {
    await panel().getByText("Network", { exact: true }).waitFor({ timeout: 10000 });
    await panel().getByText("Sandboxed.", { exact: false }).waitFor({ timeout: 5000 });
    const sw = panel().getByRole("switch", { name: "Local network access" });
    if (await sw.isChecked()) throw new Error("a new project must default to sandbox");
    if (await sw.isDisabled()) throw new Error("an admin must be able to enable it");
  });

  await check("the API reports sandbox and says the caller may enable", async () => {
    const res = await page.request.get(`${BASE}/api/projects/${PID}/network`);
    const net = await res.json();
    if (net.lanAccess !== false) throw new Error(`want lanAccess:false, got ${JSON.stringify(net)}`);
    if (net.canEnable !== true) throw new Error(`an admin must get canEnable:true, got ${JSON.stringify(net)}`);
  });

  // The badge must NOT appear while sandboxed. This is the assertion tsc
  // cannot make: LanBadge renders from an optional field, so a regression
  // would show "LAN" on every project — a false claim about isolation.
  await check("no LAN badge on a sandboxed project", async () => {
    if (await page.locator('[data-lan="on"]').count() !== 0) {
      throw new Error("a sandboxed project must carry no LAN badge");
    }
  });

  // The two checks that TOGGLE the mode live at the end of the run, not here:
  // a toggle rebuilds the project container (ADR-067, the operator's "restart
  // is fine"), which destroys everything a later check needs inside the guest
  // — the :4321 fixture app started just above, the tmux sessions, the git
  // working state. Search "Network mode: the toggle" below.

  // The preview proxy needs the TEST server (a host process) to reach the
  // guest bridge IP; the hardened host firewall DROPs that (production's
  // containerized server is exempt). Probe once and SKIP the proxy-path
  // checks when blocked — same treatment as integration.sh (v1.14.0).
  let hostFW = false;
  try {
    // No shell (execFileSync) + strict PID shape: PID comes from the
    // server's JSON, and BASE is caller-controlled — a hostile server must
    // not get argv-splice command execution (2026-08-30 review).
    if (!/^[0-9a-f-]{36}$/.test(PID)) throw new Error(`bad project id ${PID}`);
    const { execFileSync } = await import("node:child_process");
    const ip = execFileSync("docker", [
      "inspect",
      "-f",
      "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
      `styrman-${PID}`,
    ])
      .toString()
      .trim();
    await fetch(`http://${ip}:4321/index.html`, {
      signal: AbortSignal.timeout(3000),
    });
  } catch (e) {
    // Only a TIMEOUT is the firewall's DROP signature; anything else
    // (bad PID, docker missing, refused) is a real failure, not a skip
    // (2026-08-30 audit — mirrors integration.sh's exit-28 rule).
    if (e?.name === "TimeoutError") hostFW = true;
    else bad(`firewall probe: ${e?.message ?? e} (NOT the firewall)`);
  }
  if (hostFW) {
    skip("port link serves the app from inside the microVM", "host firewall blocks host→bridge; prod unaffected");
  } else {
    await check("port link serves the app from inside the microVM", async () => {
      const res = await page.request.get(
        `${BASE}/p/${PID}/1/port/4321/index.html`,
      );
      const body = await res.text();
      if (!body.includes("e2e-app"))
        throw new Error("proxy did not serve the app");
    });
  }
  // Per-port sharing: add an email then remove it (the remove button must be
  // visible + working — it was hover-hidden and undeletable before).
  // Split three ways (2026-08-30 audit: the serve assertion needs its own
  // check/skip line for honest counts, and it must run BETWEEN add and
  // remove — the revoke kills the link).
  let sharedUrl = "";
  await check("share add mints a private link", async () => {
    await page.locator('aside button:has-text("share")').first().click();
    await page.fill('aside input[placeholder*="@"]', "e2e-guest@gmail.com");
    await panel().getByRole("button", { name: "add", exact: true }).click();
    await page.waitForSelector("aside >> text=e2e-guest@gmail.com");
    // Sharing mints a stable private link, surfaced in the panel…
    await page.waitForSelector("aside >> text=copy private link", {
      timeout: 5000,
    });
    const shares = await (
      await page.request.get(`${BASE}/api/projects/${PID}/ports/4321/shares`)
    ).json();
    if (!shares.url?.startsWith("/shared/"))
      throw new Error(`share url missing: ${JSON.stringify(shares)}`);
    sharedUrl = shares.url;
  });
  // …and it actually SERVES the app (not just renders a button). NOTE: auth
  // mode is "none" here, so this exercises the OWNER branch of
  // previewAllowed — the shared-guest branch is covered by the Go tests.
  if (hostFW) {
    skip("private link serves the app", "host firewall blocks host→bridge");
  } else {
    await check("private link serves the app", async () => {
      const viaShared = await page.request.get(`${BASE}${sharedUrl}index.html`);
      if (!(await viaShared.text()).includes("e2e-app"))
        throw new Error("private link did not serve the app");
    });
  }
  await check("share remove revokes the link", async () => {
    await panel()
      .getByRole("button", { name: "Remove e2e-guest@gmail.com" })
      .click();
    await page.waitForSelector("aside >> text=e2e-guest@gmail.com", {
      state: "detached",
      timeout: 5000,
    });
    // Unsharing the last email kills the link: UI row gone, route dead.
    await page.waitForSelector("aside >> text=copy private link", {
      state: "detached",
      timeout: 5000,
    });
    const afterRevoke = await page.request.get(`${BASE}${sharedUrl}index.html`);
    if (afterRevoke.status() !== 404)
      throw new Error(
        `revoked private link still serves: ${afterRevoke.status()}`,
      );
  });
  await check("public link create + revoke", async () => {
    const revokeBtn = panel().getByRole("button", { name: "Revoke link" });
    await panel().getByRole("button", { name: "create public link" }).click();
    await revokeBtn.waitFor({ state: "visible", timeout: 10000 });
    await revokeBtn.click();
    await revokeBtn.waitFor({ state: "detached", timeout: 5000 });
  });
  await shot("05-ports");

  // ── Side panel: History (transcript of what we typed) ───────────────────
  await check("history lists the transcript", async () => {
    await panel().getByText("History", { exact: true }).click();
    await page.waitForSelector("aside >> text=conv-1", { timeout: 25000 });
  });
  await check("transcript contains our command", async () => {
    await panel().getByText("conv-1").click();
    await page.waitForFunction(() =>
      (document.querySelector("aside pre")?.textContent ?? "").includes(
        "styrman-e2e-42",
      ),
    );
  });
  await shot("06-history");

  // ── Side panel: Git — a real in-guest client (stage/commit/push), R13 ────
  await check("git tab shows the repo", async () => {
    await panel().getByText("Git", { exact: true }).click();
    // v1.7 rewrote the tab (real git client): assert its shipped surface —
    // the branch row's refresh control plus either the clean-tree notice or
    // a Staged/Changes group (the old "branch" label no longer exists).
    await page.waitForSelector(
      "aside >> text=/Nothing to commit|Staged \\(|Changes \\(/",
    );
  });
  await check("git client controls render", async () => {
    // Replaces the removed "Checkpoint now" button (checkpoints dropped in
    // v1.7.0): assert the shipped client — a Push control and a commit box.
    await page.waitForSelector("aside >> text=Push");
    await page.waitForSelector('aside textarea[placeholder^="Commit message"]');
  });
  // ── Git diff in the MAIN pane + fast commit (v1.22.0) ──────────────────
  // A commit needs an identity. The server injects `-c user.name/-c user.email`
  // from the BASE home's .gitconfig (git.go:72), which this isolated e2e state
  // dir has none of — so without this the commit fails with git's own "please
  // tell me who you are" as a 409. Set it in the guest's global config, which
  // git then falls back to.
  await check("git diff opens in the MAIN pane, not the side panel", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.locator(".xterm textarea").first().focus();
    await page.keyboard.type(
      "git config --global user.email e2e@styrman.test; git config --global user.name e2e; echo diffme >> one.txt; echo second >> two.txt",
    );
    await page.keyboard.press("Enter");
    await page.waitForTimeout(1500);
    await panel().getByText("Git", { exact: true }).click();
    await page.waitForSelector("aside >> text=/Changes \\(/", { timeout: 20000 });
    // panel() IS the aside — do not nest another "aside >>" inside it.
    await panel().getByText("one.txt", { exact: true }).first().click();
    // waitForFunction on innerText, NOT `main >> text=...`: DiffView renders
    // every diff line as its own <div> with white-space:pre, and the text
    // engine does not match inside them (verified — the pane demonstrably
    // contained "+diffme" while `main >> text=diffme` timed out).
    await mainContains("diffme");
    // The point of the move: the diff is no longer inside the 320px panel.
    if (await panel().locator("text=diffme").count())
      throw new Error("diff still rendered inside the side panel");
  });

  await check("clicking a SECOND file swaps the diff (set() no-op guard)", async () => {
    // The pane store's bail-out compares fields by identity; with `diff` left
    // out of that comparison this click is a silent no-op and the pane keeps
    // showing one.txt.
    await panel().getByText("two.txt", { exact: true }).first().click();
    await mainContains("second");
  });

  await check("fast commit stages everything and commits a UUID", async () => {
    await panel().getByRole("button", { name: "Fast commit" }).click();
    await page
      .getByRole("button", { name: "Commit everything", exact: true })
      .click();
    await page.waitForSelector("text=/Committed as [0-9a-f-]{8}-/", {
      timeout: 30000,
    });
    await page.waitForSelector("aside >> text=Nothing to commit", {
      timeout: 30000,
    });
  });

  // ── Git verbs: branches, log, stash (v1.22.0) ──────────────────────────
  await check("branch dropdown lists branches and creates one", async () => {
    await panel().getByText("Git", { exact: true }).click();
    await page.waitForSelector("aside >> text=Nothing to commit", { timeout: 20000 });
    await panel().getByTitle("Switch branch").click();
    await panel().getByText("+ New branch from here").click();
    await page.locator('[role="dialog"] input').fill("e2e/feature-1");
    await page.getByRole("button", { name: "Create", exact: true }).click();
    await page.waitForSelector("text=/Created and switched to e2e\\/feature-1/", {
      timeout: 20000,
    });
    // The header now shows the new branch.
    await page.waitForSelector("aside >> text=e2e/feature-1", { timeout: 20000 });
  });

  await check("switching to an existing branch works", async () => {
    // Exercises the SWITCH path specifically, which runs
    // `checkout --end-of-options <name> --` — a different argv shape from the
    // create path above, and the one a trailing `--` never protected.
    await panel().getByTitle("Switch branch").click();
    await panel().getByRole("button", { name: "main", exact: true }).click();
    await page.waitForSelector("text=/Switched to main/", { timeout: 20000 });
    await page.waitForSelector("aside >> text=main", { timeout: 20000 });
  });

  await check("an invalid branch name is refused before any round trip", async () => {
    await panel().getByTitle("Switch branch").click();
    await panel().getByText("+ New branch from here").click();
    // A leading dash is the argv-injection shape the server also rejects.
    await page.locator('[role="dialog"] input').fill("--upload-pack=evil");
    // PromptDialog validates on SUBMIT, not on input, so the message only
    // appears after Create — and the dialog stays open when it does.
    await page.getByRole("button", { name: "Create", exact: true }).click();
    await page.waitForSelector(
      "text=/No spaces, control characters, or a leading dash/",
      { timeout: 5000 },
    );
    // Close it no matter what: a modal left open fails every later check.
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await page.waitForSelector('[role="dialog"]', { state: "detached", timeout: 5000 });
  });

  await check("fetch and pull controls are reachable", async () => {
    // Regression guard for a shipped-but-unwired capability: gitFetch existed
    // with no button, while the dropdown's tooltip promised the action.
    // Presence only — the e2e project has no remote to fetch from.
    await panel().getByRole("button", { name: "Fetch", exact: true }).waitFor({ timeout: 10000 });
    await panel().getByRole("button", { name: /^Pull/ }).waitFor({ timeout: 10000 });
  });

  await check("history lists commits and opens one in the main pane", async () => {
    // "Commits", not "History": the side panel already has a History TAB and
    // a /History/ locator matched both (strict-mode violation).
    await panel().getByRole("button", { name: /Commits/ }).click();
    // The fast commit above produced a UUID-subject commit.
    await page.waitForSelector("aside >> text=/[0-9a-f]{7}/", { timeout: 20000 });
    await panel().locator("button", { hasText: /^[0-9a-f]{7}/ }).first().click();
    await mainContains("diff --git");
  });

  await check("stash push -u captures untracked work, pop restores it", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.locator(".xterm textarea").first().focus();
    await page.keyboard.type("echo stashme > brand-new.txt");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(1200);
    await panel().getByText("Git", { exact: true }).click();
    await page.waitForSelector("aside >> text=/Changes \\(/", { timeout: 20000 });
    await panel().getByRole("button", { name: "Stash all" }).click();
    // -u is the point: an UNTRACKED file must be stashed, not left behind.
    await page.waitForSelector("text=/Stashed \\(including untracked files\\)/", {
      timeout: 20000,
    });
    await page.waitForSelector("aside >> text=Nothing to commit", { timeout: 20000 });
    await panel().getByRole("button", { name: "Pop", exact: true }).click();
    await page.waitForSelector("text=Popped", { timeout: 20000 });
    await page.waitForSelector("aside >> text=/Changes \\(/", { timeout: 20000 });
  });

  // Clips tab must open without spinning: getClips backs useSyncExternalStore,
  // and an unstable snapshot infinite-loops the SPA (regression guard).
  await check("clips tab opens (no render loop)", async () => {
    await panel().getByText("Clipboard", { exact: true }).click();
    await page.waitForSelector("aside >> text=Copied in crew", {
      timeout: 4000,
    });
  });

  // ── Terminals: rename + second terminal ─────────────────────────────────
  // "main header": the root layout's mobile top bar is also a <header>
  // (hidden on md+ but present in the DOM).
  await page.locator("main header button").first().click();
  await page.fill("main header input", "Renamed by e2e");
  await page.keyboard.press("Enter");
  await check("rename sticks in header", () =>
    page.waitForSelector('header button:has-text("Renamed by e2e")'),
  );
  await check("rename visible in sidebar", () =>
    page.waitForSelector("aside >> text=Renamed by e2e"),
  );
  await page.click("text=New terminal");
  await check("second terminal opens", () => page.waitForURL(`**/p/${PID}/2`));

  // ── Deep link ───────────────────────────────────────────────────────────
  await page.goto(`${BASE}/p/${PID}/1`);
  await check("deep link reattaches terminal 1", () =>
    page.waitForSelector('header button:has-text("Renamed by e2e")'),
  );

  // ── Terminal recovery without a reload (ADR-079, BUG-085) ───────────────
  // The first page sits on terminal 1 and none of these three navigates it.
  // Each closes its extra page and removes its overrides in a `finally`, and
  // the first two end with the first page holding terminal 1 again
  // (.agents/rules/e2e-checks-leave-the-suite-as-they-found-it.md). What the
  // suite cannot produce for real — a hidden tab, a socket that died without
  // closing — is EMULATED at the page's own APIs, and the header of each
  // check says how.
  const waitTermText = (p, needle, timeout = 20000) =>
    p.waitForFunction(
      (n) =>
        [...document.querySelectorAll(".xterm-rows")].some((r) =>
          (r.textContent ?? "").includes(n),
        ),
      needle,
      { timeout },
    );
  const typeIn = async (p, text) => {
    await p.locator(".xterm textarea").first().focus();
    await p.keyboard.type(text, { delay: 5 });
    await p.keyboard.press("Enter");
  };
  // Headless Chromium never hides a background page, so hidden → visible is
  // emulated where the app reads it: the document's own properties, plus the
  // event the browser would dispatch.
  const setVisibility = (p, state) =>
    p.evaluate((v) => {
      Object.defineProperty(document, "visibilityState", { configurable: true, get: () => v });
      Object.defineProperty(document, "hidden", { configurable: true, get: () => v !== "visible" });
      document.dispatchEvent(new Event("visibilitychange"));
    }, state);
  const clearVisibility = (p) =>
    p
      .evaluate(() => {
        delete document.visibilityState;
        delete document.hidden;
      })
      .catch(() => {});
  const notReplaced = (p, timeout = 20000) =>
    p.waitForFunction(
      () => !(document.querySelector("main")?.innerText ?? "").includes("Opened in another tab"),
      null,
      { timeout },
    );
  // Leave the first page holding terminal 1 whatever happened: on a page
  // where the button does not exist (or the pane is already connected) this
  // does nothing.
  const firstPageTakesTerminalBack = async () => {
    const b = page.getByRole("button", { name: "Use here", exact: true });
    if (await b.count().catch(() => 0)) await b.click({ timeout: 5000 }).catch(() => {});
  };

  await check("a terminal taken by another tab is taken back with one click", async () => {
    const second = await context.newPage();
    try {
      await second.goto(`${BASE}/p/${PID}/1`);
      await page.getByText("Opened in another tab").waitFor({ timeout: 20000 });
      await page.getByRole("button", { name: "Use here", exact: true }).click();
      await notReplaced(page);
      await typeIn(page, "echo reclaim-click-$((20+3))");
      await mainContains("reclaim-click-23");
    } finally {
      await second.close().catch(() => {});
      await firstPageTakesTerminalBack();
    }
  });

  await check("a replaced terminal comes back by itself when its tab is shown again", async () => {
    const second = await context.newPage();
    try {
      await second.goto(`${BASE}/p/${PID}/1`);
      await page.getByText("Opened in another tab").waitFor({ timeout: 20000 });
      await setVisibility(page, "hidden");
      await setVisibility(page, "visible");
      await notReplaced(page);
      await typeIn(page, "echo reclaim-shown-$((30+4))");
      await mainContains("reclaim-shown-34");
    } finally {
      await clearVisibility(page);
      await second.close().catch(() => {});
      await firstPageTakesTerminalBack();
    }
  });

  // The socket fixture of the two checks below, installed on a THROWAWAY page
  // before its first navigation. It wraps window.WebSocket so one socket
  // instance can be driven from the test:
  //   __frozen       its sends are dropped and its open/message/close events
  //                  withheld while readyState stays OPEN — what a page sees of
  //                  a connection that died without a close;
  //   __dropNow(c)   the app's close handler is called with code c, as a
  //                  transient or a final close would;
  //   window.__failNextTermSocket = c   the NEXT terminal socket the page
  //                  opens is closed with code c before it ever opens.
  // The app sets its handlers through the on* properties, so those setters
  // are what the wrapper intercepts. The suite's first page never carries it.
  const installSocketFixture = (p) =>
    p.addInitScript(() => {
      const Real = window.WebSocket;
      const all = [];
      class FreezableSocket extends Real {
        constructor(...args) {
          super(...args);
          this.__frozen = false;
          this.__texts = [];
          this.__failCode = 0;
          if (window.__failNextTermSocket && String(args[0]).includes("/ws/term/")) {
            this.__failCode = window.__failNextTermSocket;
            window.__failNextTermSocket = 0;
            this.__frozen = true;
          }
          all.push(this);
        }
        send(data) {
          if (this.__frozen) return;
          super.send(data);
        }
        __dropNow(code) {
          this.__frozen = true;
          try {
            super.close();
          } catch {
            // already closing
          }
          this.__appClose?.call(this, { code, reason: "" });
        }
        get onopen() {
          return super.onopen;
        }
        set onopen(fn) {
          super.onopen =
            fn &&
            ((ev) => {
              if (!this.__frozen) fn.call(this, ev);
            });
        }
        get onmessage() {
          return super.onmessage;
        }
        set onmessage(fn) {
          super.onmessage =
            fn &&
            ((ev) => {
              if (this.__frozen) return;
              if (typeof ev.data === "string") this.__texts.push(ev.data);
              fn.call(this, ev);
            });
        }
        get onclose() {
          return super.onclose;
        }
        set onclose(fn) {
          this.__appClose = fn;
          super.onclose =
            fn &&
            ((ev) => {
              if (!this.__frozen) fn.call(this, ev);
            });
          if (fn && this.__failCode) {
            const code = this.__failCode;
            setTimeout(() => this.__dropNow(code), 0);
          }
        }
      }
      window.WebSocket = FreezableSocket;
      window.__termSockets = () => all.filter((s) => s.url.includes("/ws/term/"));
    });
  const termHas = (p, needle) =>
    p.evaluate(
      (n) => [...document.querySelectorAll(".xterm-rows")].some((r) => (r.textContent ?? "").includes(n)),
      needle,
    );
  const waitPrompt = (p) =>
    p.waitForFunction(
      () => /[$❯]/.test(document.querySelector(".xterm-rows")?.textContent ?? ""),
      null,
      { timeout: 45000 },
    );
  const waitLiveSocket = (p, timeout = 20000) =>
    p.waitForFunction(
      () => window.__termSockets().some((s) => !s.__frozen && s.readyState === 1),
      null,
      { timeout },
    );

  await check("a terminal whose socket went silent works again once its tab is shown", async () => {
    // A throwaway page on terminal 2 carries the fixture; the first page is
    // not touched.
    const third = await context.newPage();
    try {
      await installSocketFixture(third);
      await third.goto(`${BASE}/p/${PID}/2`);
      await waitPrompt(third);
      await typeIn(third, "echo frz-before-$((40+1))");
      await waitTermText(third, "frz-before-41");
      // The page only trusts its liveness check on a connection that has
      // answered a ping, so wait (bounded) for the first pong — then freeze
      // WHATEVER happened: against a build without the fix no pong arrives,
      // and the check must fail at its last step, not here.
      await third
        .waitForFunction(
          () =>
            window.__termSockets().some(
              (s) => s.readyState === 1 && s.__texts.some((t) => t.includes('"pong"')),
            ),
          null,
          { timeout: 5000 },
        )
        .catch(() => {});
      const frozen = await third.evaluate(() => {
        const open = window.__termSockets().filter((s) => s.readyState === 1);
        for (const s of open) s.__frozen = true;
        return open.length;
      });
      if (frozen !== 1) throw new Error(`expected one open terminal socket to freeze, froze ${frozen}`);
      // The fixture must produce the condition it claims: nothing typed now
      // reaches the shell.
      await typeIn(third, "echo frz-lost-$((50+2))");
      await third.waitForTimeout(2000);
      if (await termHas(third, "frz-lost-52")) {
        throw new Error("the frozen socket still carried input: the fixture froze nothing");
      }
      await setVisibility(third, "hidden");
      await setVisibility(third, "visible");
      // The page notices (a probe, 5 s unanswered) and reconnects in place: a
      // NEW, unfrozen socket, and the same page — no reload.
      await waitLiveSocket(third);
      await typeIn(third, "echo frz-after-$((60+3))");
      await waitTermText(third, "frz-after-63");
    } finally {
      await third.close().catch(() => {});
    }
  });

  await check("keys typed before a pane gave up are not replayed when it comes back", async () => {
    // The arrangement the security review of 2026-10-02 named: keystrokes
    // typed while the pane is RECONNECTING are queued; if the pane then gives
    // up ("Disconnected") and is brought back by a wake event — possibly
    // hours later — the queue must not be flushed into the shell. Staged on a
    // throwaway page: the live socket is dropped as a transient close (the
    // pane starts reconnecting), a line is typed (queued), the reconnect
    // attempt is failed with a non-transient code (the pane gives up), and
    // the tab is shown again (the pane reconnects for real).
    const fourth = await context.newPage();
    try {
      await installSocketFixture(fourth);
      await fourth.goto(`${BASE}/p/${PID}/2`);
      await waitPrompt(fourth);
      await typeIn(fourth, "echo ob-before-$((70+1))");
      await waitTermText(fourth, "ob-before-71");
      const dropped = await fourth.evaluate(() => {
        window.__failNextTermSocket = 1011;
        const open = window.__termSockets().filter((s) => s.readyState === 1 && !s.__frozen);
        for (const s of open) s.__dropNow(1006);
        return open.length;
      });
      if (dropped !== 1) throw new Error(`expected one open terminal socket to drop, dropped ${dropped}`);
      // Typed at once: the pane's first retry comes 2 s after the drop, and
      // until it fails the pane is not closed for good, so this line is queued.
      await typeIn(fourth, "echo ob-stale-$((80+2))");
      await fourth.getByText("Disconnected", { exact: true }).waitFor({ timeout: 15000 });
      await setVisibility(fourth, "hidden");
      await setVisibility(fourth, "visible");
      await waitLiveSocket(fourth);
      await typeIn(fourth, "echo ob-after-$((90+3))");
      await waitTermText(fourth, "ob-after-93");
      // The fresh line ran, so anything the reconnect flushed has run too.
      if (await termHas(fourth, "ob-stale-82")) {
        throw new Error("the line typed before the pane gave up was replayed into the shell");
      }
    } finally {
      await fourth.close().catch(() => {});
    }
  });

  // ── Settings ────────────────────────────────────────────────────────────
  await page.click("aside >> text=Settings");
  await check("settings page renders sections", async () => {
    await page.waitForURL("**/settings");
    await page.waitForSelector("text=Terminal username");
    await page.waitForSelector("text=State dir");
  });
  await check("settings shows runtime info", () =>
    page.waitForSelector("text=Kata microVM"),
  );

  // ── Appearance: fonts (v1.22.0) ─────────────────────────────────────────
  // NOTE ON SCOPE: /settings is a different route, so opening it UNMOUNTS the
  // terminal page and its editor (see the comment at p.$project.$term.tsx:30).
  // There is therefore no in-tab path where a font change reaches a surface
  // that is already mounted — what these checks prove is that the preference
  // persists and is applied on the next mount. CodeEditor's fontComp
  // compartment and TerminalPane's live-apply effect are correctness/robustness
  // measures for that store, not behaviour a user can currently observe.
  const APPEARANCE_KEY = "styrman.appearance.v1";
  const readPref = (field) =>
    page.evaluate(
      ([k, f]) => JSON.parse(localStorage.getItem(k) ?? "{}")[f],
      [APPEARANCE_KEY, field],
    );

  await check("appearance exposes terminal and editor font controls", async () => {
    for (const label of [
      "Terminal font size",
      "Terminal font family",
      "Editor font size",
      "Editor font family",
    ]) {
      await page.waitForSelector(`[aria-label="${label}"]`, { timeout: 10000 });
    }
  });

  await check("terminal font size persists", async () => {
    await page.locator('input[aria-label="Terminal font size"]').fill("18");
    const v = await readPref("termFontSize");
    if (v !== 18) throw new Error(`stored termFontSize=${v}`);
  });

  await check("editor font family persists", async () => {
    await page
      .locator('[aria-label="Editor font family"]')
      .getByRole("radio", { name: "JetBrains Mono" })
      .click();
    const v = await readPref("editorFontFamily");
    if (v !== "jetbrains-mono") throw new Error(`stored editorFontFamily=${v}`);
  });

  await check("the chosen terminal font size reaches a live xterm", async () => {
    await page.goto(`${BASE}/p/${PID}/1`);
    await page.waitForSelector(".xterm-rows", { timeout: 30000 });
    // xterm applies the size to its own elements; read whichever reports one
    // rather than pinning an internal class that could move between versions.
    const size = await page.evaluate(() => {
      for (const sel of [".xterm-rows", ".xterm-screen", ".xterm"]) {
        const el = document.querySelector(sel);
        const fs = el && getComputedStyle(el).fontSize;
        if (fs && fs !== "0px") return `${sel}=${fs}`;
      }
      return null;
    });
    if (!size || !size.endsWith("=18px"))
      throw new Error(`xterm font-size ${size}`);
  });

  await check("the chosen editor font reaches a freshly opened buffer", async () => {
    // The goto above was a FULL page load, which resets the per-terminal pane
    // store (module-level, deliberately not persisted — editorPane.ts:19), so
    // the Editor view would show its "No file open" empty state. Open a file
    // through the Files panel first, exactly as the editor checks above do.
    await panel().getByText("Files", { exact: true }).click();
    await page.waitForTimeout(500);
    // one.txt, at the workspace ROOT, is the file still standing this late in
    // the run: subdir/nested.txt is deleted by the scoped-delete check above
    // and Dockerfile by the stale-path check, so neither can be used here.
    await fileList().getByText("one.txt", { exact: true }).click();
    await page.waitForSelector(".cm-content", { timeout: 15000 });
    const fam = await page.evaluate(
      () =>
        getComputedStyle(document.querySelector(".cm-scroller")).fontFamily,
    );
    if (!/JetBrains Mono Variable/.test(fam))
      throw new Error(`cm-scroller fontFamily=${fam}`);
  });

  // ORDERING: this overwrites the stored appearance and RELOADS, so it runs
  // last in this block and puts the defaults back when it is done.
  await check("a corrupt appearance value falls back to defaults", async () => {
    await page.goto(`${BASE}/settings`);
    await page.waitForSelector('input[aria-label="Terminal font size"]');
    await page.evaluate(
      (k) => localStorage.setItem(k, "{{{ not json at all"),
      APPEARANCE_KEY,
    );
    await page.reload();
    const slider = page.locator('input[aria-label="Terminal font size"]');
    await slider.waitFor({ timeout: 15000 });
    const v = await slider.inputValue();
    if (v !== "13") throw new Error(`slider showed ${v}, expected the 13 default`);
    // Leave storage clean for whatever runs after this.
    await page.evaluate((k) => localStorage.removeItem(k), APPEARANCE_KEY);
  });

  await shot("07-settings");

  // ── Setup page ──────────────────────────────────────────────────────────
  // Navigate directly: the banner link only exists pre-completion. The page
  // shows the username picker on a fresh account, or auto-boots the setup
  // terminal once a username was chosen (checkpointed, never re-asked).
  await page.goto(`${BASE}/setup`);
  await check("setup page shows picker or boots the VM", async () => {
    // Third positional arg is options — passing it second silently makes it
    // the pageFunction arg and the 15s default timeout applies.
    await page.waitForFunction(
      () =>
        document.querySelector("#setup-username") !== null ||
        (document.querySelector(".xterm-rows")?.textContent ?? "").trim()
          .length > 40,
      null,
      { timeout: 60000 },
    );
  });
  await shot("08-setup");

  // ── Theme (3-state radio: light / dark / system) ────────────────────────
  await page.click('aside button[aria-label="Light theme"]');
  await check("light theme applies", async () => {
    const theme = await page.evaluate(
      () => document.documentElement.dataset.theme,
    );
    if (theme !== "light") throw new Error(`theme=${theme}`);
  });
  await shot("09-light-theme");
  await page.click('aside button[aria-label="Dark theme"]');

  // ── Network mode: the toggle (ADR-067) ─────────────────────────────────
  // LAST, deliberately. A mode change rebuilds the project container, so every
  // check that depends on guest state — the ports fixture, terminals, the git
  // client, history — has to have run already. The first draft of this section
  // sat next to the read-only checks up in the Ports block and took 17 checks
  // down with it: the rebuild killed the :4321 server, the firewall probe then
  // read connection-refused as "NOT the firewall", and the git panel lost the
  // session it was driving.
  await page.goto(`${BASE}/p/${PID}/1`);
  // The Network block lives INSIDE the Ports tab, so the tab has to be opened
  // before anything in it can be waited for — up in the Ports block that had
  // already happened, and moving the checks down here lost it.
  await panel().getByText("Ports", { exact: true }).click();
  await panel().getByText("Network", { exact: true }).waitFor({ timeout: 20000 });

  await check("enabling rebuilds the project and flags it", async () => {
    // 210 s, pairing with the SERVER's lanRebuildTimeout (3 min), not with
    // start/stop's client budget: Playwright's 30 s default cut the rebuild
    // off, and 150 s would still be 30 s short of what the server allows.
    const res = await page.request.put(`${BASE}/api/projects/${PID}/network`, {
      data: { lanAccess: true },
      headers: { "content-type": "application/json" },
      timeout: 210000,
    });
    if (!res.ok()) throw new Error(`enable failed: ${res.status()} ${await res.text()}`);
    const net = await res.json();
    if (!net.lanAccess) throw new Error("the enable did not persist");
    // The project was running, so the PUT rebuilt it synchronously.
    if (!net.rebuilt) throw new Error("a running project must be rebuilt by the PUT");
    await page.reload();
    await page.locator('[data-lan="on"]').first().waitFor({ timeout: 20000 });
  });

  // ── Browser view (ADR-068) — deliberately INSIDE the LAN window. A server
  // running on the host cannot reach a sandbox project's bridge on a
  // firewall-hardened host (the same reason the preview checks auto-SKIP),
  // and the relay to the in-guest Chromium has exactly that shape; the LAN
  // rebuild just handed us a fresh container with a reachable bridge. The
  // rebuild also killed the :4321 fixture, so the browser gets its own.
  const browserFramesOf = async () =>
    Number(await page.locator("canvas[data-frames]").getAttribute("data-frames"));
  const waitBrowserFrames = async (min, timeout = 30000) => {
    const t = Date.now();
    for (;;) {
      if ((await page.locator("canvas[data-frames]").count()) && (await browserFramesOf()) >= min) {
        return browserFramesOf();
      }
      if (Date.now() - t > timeout) {
        const phase = await page.locator("[data-phase]").getAttribute("data-phase");
        throw new Error(`no screencast frame >= ${min} within ${timeout} ms (pane phase ${phase})`);
      }
      await page.waitForTimeout(250);
    }
  };
  // The reload above reconnects the terminal into the REBUILT container:
  // wait for a shell prompt before typing, or the command lands nowhere
  // (first e2e run of these checks, 2026-09-11).
  await page.getByRole("button", { name: "Terminal", exact: true }).click();
  await page.waitForFunction(
    () => /[$❯]/.test(document.querySelector(".xterm-rows")?.textContent ?? ""),
    null,
    { timeout: 60000 },
  );
  await page.locator(".xterm textarea").first().focus();
  await page.keyboard.type(
    // No "!" anywhere in this line: the terminal is an INTERACTIVE bash, and
    // "<!doctype" is a history expansion ("event not found") that silently
    // killed the fixture on the first two runs (2026-09-11).
    'mkdir -p /tmp/bx && cd /tmp/bx && printf "<title>Browser fixture</title><h1>hello from the guest</h1>" > index.html && python3 -m http.server 4100 >/dev/null 2>&1 &',
    { delay: 5 },
  );
  await page.keyboard.press("Enter");
  await mainContains("/tmp/bx");

  await check("Browser pill opens the browser view and a frame arrives", async () => {
    await page.getByRole("button", { name: "Browser", exact: true }).click();
    await waitBrowserFrames(1);
  });

  await check("typing a localhost URL renders the fixture app", async () => {
    const before = await browserFramesOf();
    await page.getByLabel("Address").fill("localhost:4100");
    await page.getByLabel("Address").press("Enter");
    await page.waitForFunction(
      () => (document.querySelector('input[aria-label="Address"]')?.value ?? "").includes("localhost:4100"),
      null,
      { timeout: 20000 },
    );
    // The tab strip carries the page's TITLE, refreshed after the load.
    await page.getByRole("tab", { name: /Browser fixture/ }).waitFor({ timeout: 20000 });
    await waitBrowserFrames(before + 1);
    await shot("browser-fixture");
  });

  await check("Open in Browser from the Ports panel switches the view and navigates", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await panel().getByText("Ports", { exact: true }).click();
    await page.waitForSelector("aside >> text=:4100", { timeout: 25000 });
    await panel().getByRole("button", { name: "browser", exact: true }).first().click();
    await page.waitForFunction(
      () => document.querySelector('button[aria-label="Browser"]')?.getAttribute("aria-pressed") === "true",
      null,
      { timeout: 10000 },
    );
    await page.waitForFunction(
      () => (document.querySelector('input[aria-label="Address"]')?.value ?? "").startsWith("http://localhost:4100/"),
      null,
      { timeout: 20000 },
    );
  });

  await check("switching to the terminal and back resumes the screencast", async () => {
    // Hidden, the pane stops its screencast (the box is 0×0 and no viewport
    // is pushed onto the agents' page); shown again it restarts it, and
    // Page.startScreencast always emits a first frame — so the count rises
    // only if the show path really restarted it (review, 2026-09-13).
    const before = await browserFramesOf();
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.waitForFunction(
      () => document.querySelector('button[aria-label="Browser"]')?.getAttribute("aria-pressed") !== "true",
      null,
      { timeout: 10000 },
    );
    await page.getByRole("button", { name: "Browser", exact: true }).click();
    await waitBrowserFrames(before + 1);
  });

  await check("terminal session survived the browser round-trip", async () => {
    await page.getByRole("button", { name: "Terminal", exact: true }).click();
    await page.locator(".xterm textarea").first().focus();
    await page.keyboard.type("echo browser-rt-$((40+2))", { delay: 5 });
    await page.keyboard.press("Enter");
    await mainContains("browser-rt-42");
  });

  await check("disabling returns it to sandbox", async () => {
    const res = await page.request.put(`${BASE}/api/projects/${PID}/network`, {
      data: { lanAccess: false },
      headers: { "content-type": "application/json" },
      timeout: 210000,
    });
    if (!res.ok()) throw new Error(`disable failed: ${res.status()}`);
    const net = await res.json();
    if (net.lanAccess) throw new Error("still on after disable");
    await page.reload();
    await panel().getByText("Ports", { exact: true }).click();
    await panel().getByText("Sandboxed.", { exact: false }).waitFor({ timeout: 20000 });
  });

  // ── Cleanup: delete terminal + project via UI ───────────────────────────
  await page.goto(`${BASE}/p/${PID}/2`);
  // Any NATIVE browser dialog is a v1.3 regression (everything moved to
  // styled dialogs/toasts) — fail loudly instead of silently accepting.
  page.on("dialog", (d) => {
    bad("native browser dialog appeared", `${d.type()}: ${d.message().slice(0, 80)}`);
    d.dismiss();
  });
  // Terminal 2's title is generated — read it from the header first (wait
  // for the record poll to fill it in; the placeholder is "Terminal 2").
  await page.waitForFunction(() => {
    const b = document.querySelector('main header button[title="Rename terminal"]');
    const t = b?.textContent?.trim() ?? "";
    return t !== "" && t !== "Terminal 2";
  });
  const term2Title = (
    await page
      .locator('main header button[title="Rename terminal"]')
      .textContent()
  )?.trim();
  const termRow = page.locator("aside .group", { hasText: term2Title }).last();
  await termRow.hover();
  await termRow.locator('button[aria-label^="Delete"]').click({ force: true });
  // Styled confirm dialog (no native popups since v1.3).
  await page.locator('div[role="dialog"] button:has-text("Delete")').click();
  await check("terminal deleted from sidebar", () =>
    page.waitForFunction(
      (t) => !document.querySelector("aside")?.textContent?.includes(t),
      term2Title,
    ),
  );
  const projRow = page.locator("aside .group", { hasText: PROJECT }).first();
  await projRow.hover();
  await projRow.locator('button[aria-label*="actions"]').click({ force: true });
  const menuDelete = page.locator("button.text-danger", { hasText: "Delete" });
  await menuDelete.waitFor();
  await menuDelete.click();
  // Typed-confirmation dialog (no browser popups): type "delete" to arm.
  await check('delete dialog requires typing "delete"', async () => {
    await page.waitForSelector("#del-confirm");
    const armed = await page
      .locator('button:has-text("Delete project")')
      .isEnabled();
    if (armed) throw new Error("delete button armed before typing");
  });
  await page.fill("#del-confirm", "delete");
  await page.click('button:has-text("Delete project")');
  await check("project deleted from sidebar", () =>
    page.waitForFunction(
      (name) => !document.querySelector("nav")?.textContent?.includes(name),
      PROJECT,
      { timeout: 90000 },
    ),
  );
  await shot("10-after-cleanup");

  // ── A clone that fails must SAY SO, not present as an empty project ─────
  // The 2026-09-02 outage was silent: provisioning ran before the server had
  // joined the project's bridge, so every clone failed for want of
  // credentials and the UI showed a normal, empty project. The badge is the
  // only channel back to the user (POST /api/projects answers 201 long before
  // the clone runs), so it is worth a real microVM boot to prove it appears.
  const BADPROJ = "e2e-badclone";
  await fetch(`${BASE}/api/projects/${BADPROJ}?wipe=true`, { method: "DELETE" })
    .catch(() => {});
  const badCreate = await fetch(`${BASE}/api/projects`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      name: BADPROJ,
      // Unreachable by construction: no credentials and no such repo, so this
      // fails the same way with or without network access.
      cloneUrl: "https://github.com/kesokaj/e2e-no-such-repo-styrman.git",
    }),
  });
  const BADID = badCreate.ok ? (await badCreate.json()).id : "";
  if (!BADID) {
    skip("failed clone reports provisionError", "project create failed");
    skip("failed clone still leaves a git repo (R13)", "project create failed");
  } else {
    await check("failed clone reports provisionError", async () => {
      const deadline = Date.now() + 120000;
      for (;;) {
        const r = await (await fetch(`${BASE}/api/projects`)).json();
        const p = r.projects.find((x) => x.id === BADID);
        if (p?.provisionError) {
          if (!/clone/i.test(p.provisionError))
            throw new Error(`unexpected message: ${p.provisionError}`);
          return;
        }
        if (Date.now() > deadline)
          throw new Error("no provisionError after 120s");
        await page.waitForTimeout(2000);
      }
    });
    await check("sidebar shows the repo setup failed badge", async () => {
      await page.goto(BASE + "/");
      await page.waitForSelector('nav >> text=repo setup failed', { timeout: 30000 });
    });
    // R13: the workspace is a git repo from second zero even when the clone
    // could not fill it — the fallback init has to have run.
    await check("failed clone still leaves a git repo (R13)", async () => {
      const r = await fetch(`${BASE}/api/projects/${BADID}/files?path=`);
      if (!r.ok) throw new Error(`files listing ${r.status}`);
      const names = ((await r.json()).entries ?? []).map((e) => e.name);
      if (!names.includes(".git"))
        throw new Error(`no .git in workspace: ${JSON.stringify(names)}`);
    });
    await shot("11-clone-failure-badge");
    await fetch(`${BASE}/api/projects/${BADID}?wipe=true`, { method: "DELETE" })
      .catch(() => {});
  }

  // ── And the happy path: a clone that WORKS actually fills the workspace ──
  // This is the fix itself. The repo MUST be public: this harness runs a
  // host-mode server, which has no credential broker by design, so a private
  // repo fails here for want of credentials (it did, on the first attempt —
  // the suite caught it). A public https clone needs none, which makes this a
  // clean test of the ORDER (attach → broker wait → login-shell clone) rather
  // than of credential delivery.
  const OKPROJ = "e2e-clone-ok";
  await fetch(`${BASE}/api/projects/${OKPROJ}?wipe=true`, { method: "DELETE" })
    .catch(() => {});
  const okCreate = await fetch(`${BASE}/api/projects`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      name: OKPROJ,
      cloneUrl: "https://github.com/octocat/Hello-World.git",
    }),
  });
  const OKID = okCreate.ok ? (await okCreate.json()).id : "";
  if (!OKID) {
    skip("successful clone fills the workspace", "project create failed");
  } else {
    // A host with no route to github cannot test this; that is a SKIP, not a
    // failure. Anything else is a real regression.
    const OFFLINE = /could not resolve host|unable to access|connection (refused|timed out)/i;
    let outcome = null;
    const deadline = Date.now() + 180000;
    while (outcome === null) {
      const r = await fetch(`${BASE}/api/projects/${OKID}/files?path=`);
      if (r.ok) {
        const names = ((await r.json()).entries ?? []).map((e) => e.name);
        // The R13 fallback leaves ONLY .git; a real clone brings payload with it.
        if (names.some((n) => n !== ".git")) outcome = { ok: true, names };
      }
      if (outcome === null) {
        const list = await (await fetch(`${BASE}/api/projects`)).json();
        const err = list.projects.find((x) => x.id === OKID)?.provisionError;
        if (err) outcome = { ok: false, err };
        else if (Date.now() > deadline)
          outcome = { ok: false, err: "clone did not finish in 180s" };
        else await page.waitForTimeout(3000);
      }
    }
    if (!outcome.ok && OFFLINE.test(outcome.err)) {
      skip("successful clone fills the workspace", "no route to github from the guest");
    } else {
      await check("successful clone fills the workspace", () => {
        if (!outcome.ok) throw new Error(outcome.err);
      });
    }
    await fetch(`${BASE}/api/projects/${OKID}?wipe=true`, { method: "DELETE" })
      .catch(() => {});
  }
} catch (e) {
  bad("unexpected error", e.message.split("\n")[0]);
  await shot("99-failure");
} finally {
  await browser.close();
  if (hydrationWarns.length)
    console.log(
      `  (note: ${hydrationWarns.length} recoverable hydration warning(s) — known-benign, ADR-045)`,
    );
  if (pageErrors.length) {
    failed++;
    console.log(`FAIL no uncaught client errors — saw ${pageErrors.length}:`);
    // 160 chars cut the URL mid-UUID, which is the one part of the message
    // that makes a recurrence diagnosable — the whole reason the URL is
    // recorded here at all. These entries are one-liners; 400 fits them.
    for (const e of pageErrors.slice(0, 5)) console.log(`  ${e.slice(0, 400)}`);
  } else {
    passed++;
    console.log("PASS no uncaught client errors");
  }
  console.log(`\npassed=${passed} failed=${failed} skipped=${skipped}`);
  process.exit(failed === 0 ? 0 : 1);
}
