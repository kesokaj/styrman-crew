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
// on the client — and pre-date this suite; track them separately (TODO: the
// SPA-prerender hydration mismatch) so a genuine runtime error still fails.
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
    // .bashrc is always in the fresh home skeleton (ensureHome); .claude only
    // exists after a real sign-in, so don't depend on it.
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
    await panel().getByText("subdir", { exact: true }).click();
    await page.waitForSelector("aside >> text=nested.txt");
  });
  await shot("04-files");

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
  await check("port link serves the app from inside the microVM", async () => {
    const res = await page.request.get(
      `${BASE}/p/${PID}/1/port/4321/index.html`,
    );
    const body = await res.text();
    if (!body.includes("e2e-app"))
      throw new Error("proxy did not serve the app");
  });
  // Per-port sharing: add an email then remove it (the remove button must be
  // visible + working — it was hover-hidden and undeletable before).
  await check("share add + remove on a port", async () => {
    await page.locator('aside button:has-text("share")').first().click();
    await page.fill('aside input[placeholder*="@"]', "e2e-guest@gmail.com");
    await panel().getByRole("button", { name: "add", exact: true }).click();
    await page.waitForSelector("aside >> text=e2e-guest@gmail.com");
    // Sharing mints a stable private link, surfaced in the panel…
    await page.waitForSelector("aside >> text=copy private link", {
      timeout: 5000,
    });
    // …and it actually SERVES the app (not just renders a button). NOTE:
    // auth mode is "none" here, so this exercises the OWNER branch of
    // previewAllowed — the shared-guest branch is covered by the Go tests.
    const shares = await (
      await page.request.get(`${BASE}/api/projects/${PID}/ports/4321/shares`)
    ).json();
    if (!shares.url?.startsWith("/shared/"))
      throw new Error(`share url missing: ${JSON.stringify(shares)}`);
    const viaShared = await page.request.get(`${BASE}${shares.url}index.html`);
    if (!(await viaShared.text()).includes("e2e-app"))
      throw new Error("private link did not serve the app");
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
    const afterRevoke = await page.request.get(
      `${BASE}${shares.url}index.html`,
    );
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

  // ── Side panel: Git (diff + checkpoints, R13) ───────────────────────────
  await check("git tab shows the repo", async () => {
    await panel().getByText("Git", { exact: true }).click();
    await page.waitForSelector("aside >> text=branch");
  });
  await check("manual checkpoint appears", async () => {
    await panel().getByRole("button", { name: "Checkpoint now" }).click();
    await page.waitForSelector("aside >> text=manual");
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
} catch (e) {
  bad("unexpected error", e.message.split("\n")[0]);
  await shot("99-failure");
} finally {
  await browser.close();
  if (hydrationWarns.length)
    console.log(
      `  (note: ${hydrationWarns.length} recoverable hydration warning(s) — see TODO)`,
    );
  if (pageErrors.length) {
    failed++;
    console.log(`FAIL no uncaught client errors — saw ${pageErrors.length}:`);
    for (const e of pageErrors.slice(0, 5)) console.log(`  ${e.slice(0, 160)}`);
  } else {
    passed++;
    console.log("PASS no uncaught client errors");
  }
  console.log(`\npassed=${passed} failed=${failed}`);
  process.exit(failed === 0 ? 0 : 1);
}
