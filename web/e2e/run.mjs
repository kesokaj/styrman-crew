// styrman UI end-to-end test (Playwright, headless Chromium).
// Drives the real UI against a running server: sidebar, setup banner,
// project creation (boots a microVM), terminals (xterm uses the DOM
// renderer, so terminal text is assertable), the side panel's Files / Ports
// / History / Diff tabs, settings, deep links, and cleanup.
//
// Usage: node web/e2e/run.mjs [--base http://127.0.0.1:8080] [--shots /tmp/shots]
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
const page = await browser.newPage({ viewport: { width: 1440, height: 860 } });
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
  if (m.type() === "error") record(m.text());
});
page.setDefaultTimeout(15000);
const shot = (name) => page.screenshot({ path: `${SHOTS}/${name}.png` });
const panel = () => page.locator("aside").last();
// File clicks are scoped to the listing: the Recent chips strip (2026-09-01)
// repeats file basenames inside the same aside, and an unscoped getByText
// would strict-mode-collide with them.
const fileList = () => panel().getByTestId("file-list");

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
    if (n !== 8) throw new Error(`recents strip rendered ${n} chips, cap is 8`);
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
    for (const e of pageErrors.slice(0, 5)) console.log(`  ${e.slice(0, 160)}`);
  } else {
    passed++;
    console.log("PASS no uncaught client errors");
  }
  console.log(`\npassed=${passed} failed=${failed} skipped=${skipped}`);
  process.exit(failed === 0 ? 0 : 1);
}
