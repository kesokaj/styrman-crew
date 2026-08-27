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
page.setDefaultTimeout(15000);
const shot = (name) => page.screenshot({ path: `${SHOTS}/${name}.png` });
const panel = () => page.locator("aside").last();

try {
  // ── Home ────────────────────────────────────────────────────────────────
  await page.goto(BASE + "/");
  await check("app shell renders", () =>
    page.waitForSelector('aside:has-text("styrman")'),
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
  await page.click("text=+ New Project");
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
  await check("terminal header shows title", () =>
    page.waitForSelector(
      'header button:has-text("Terminal 1"), header button:has-text("Conversation 1")',
    ),
  );
  await check("project listed running (green dot)", () =>
    page.waitForSelector(
      `aside div:has-text("${PROJECT}") span[title="running"]`,
    ),
  );

  // ── The terminal is a shell; typing reaches the microVM ─────────────────
  await check("shell prompt appears", () =>
    page.waitForFunction(
      () => /\$/.test(document.querySelector(".xterm-rows")?.textContent ?? ""),
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
  // Link bar: a long URL that soft-wraps across terminal lines must still be
  // reassembled and surfaced as an Open/Copy chip (auth-flow rescue).
  await check("wrapped URL appears in the link bar", async () => {
    await page.keyboard.type(
      'echo "https://auth.example.com/oauth2/authorize?client_id=styrman-e2e&scope=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&state=wrapcheck"',
      { delay: 5 },
    );
    await page.keyboard.press("Enter");
    await page.waitForFunction(() =>
      [...document.querySelectorAll("a")].some((a) =>
        (a.href ?? "").includes("state=wrapcheck"),
      ),
    );
  });

  await check("both agents are installed", async () => {
    await page.keyboard.type("command -v claude agy | wc -l", { delay: 10 });
    await page.keyboard.press("Enter");
    await page.waitForFunction(() =>
      [...document.querySelectorAll(".xterm-rows")].some((r) =>
        (r.textContent ?? "").includes("command -v claude agy | wc -l"),
      ),
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

  // ── Terminals: rename + second terminal ─────────────────────────────────
  await page.locator("header button").first().click();
  await page.fill("header input", "Renamed by e2e");
  await page.keyboard.press("Enter");
  await check("rename sticks in header", () =>
    page.waitForSelector('header button:has-text("Renamed by e2e")'),
  );
  await check("rename visible in sidebar", () =>
    page.waitForSelector("aside >> text=Renamed by e2e"),
  );
  await page.click("text=+ New terminal");
  await check("second terminal opens", () => page.waitForURL(`**/p/${PID}/2`));

  // ── Deep link ───────────────────────────────────────────────────────────
  await page.goto(`${BASE}/p/${PID}/1`);
  await check("deep link reattaches terminal 1", () =>
    page.waitForSelector('header button:has-text("Renamed by e2e")'),
  );

  // ── Settings ────────────────────────────────────────────────────────────
  await page.click("text=⚙ Settings");
  await check("settings page renders sections", async () => {
    await page.waitForURL("**/settings");
    await page.waitForSelector("text=Allowed accounts");
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

  // ── Theme ───────────────────────────────────────────────────────────────
  await page.click("text=☀ Light");
  await check("light theme applies", async () => {
    const theme = await page.evaluate(
      () => document.documentElement.dataset.theme,
    );
    if (theme !== "light") throw new Error(`theme=${theme}`);
  });
  await shot("09-light-theme");
  await page.click("text=☾ Dark");

  // ── Cleanup: delete terminal + project via UI ───────────────────────────
  await page.goto(`${BASE}/p/${PID}/2`);
  page.on("dialog", (d) => {
    console.log(`  [dialog:${d.type()}] ${d.message().slice(0, 80)}`);
    d.accept();
  });
  const termRow = page
    .locator("aside .group", { hasText: "Terminal 2" })
    .last();
  await termRow.hover();
  await termRow.locator('button[aria-label^="Delete"]').click({ force: true });
  await check("terminal deleted from sidebar", () =>
    page.waitForFunction(
      () =>
        !document.querySelector("aside")?.textContent?.includes("Terminal 2"),
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
  console.log(`\npassed=${passed} failed=${failed}`);
  process.exit(failed === 0 ? 0 : 1);
}
