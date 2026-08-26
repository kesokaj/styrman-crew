// styrman UI end-to-end test (Playwright, headless Chromium).
// Drives the real UI against a running server: sidebar, setup banner,
// project creation (boots a microVM), agent + shell terminals (xterm uses
// the DOM renderer, so terminal text is assertable), rename/delete flows,
// deep links, and cleanup. Screenshots land in the --shots dir.
//
// Usage: node .agents/docs/testing/styrman/e2e.mjs [--base http://127.0.0.1:8080] [--shots /tmp/shots]
import { chromium } from 'playwright'

const arg = (name, def) => {
  const i = process.argv.indexOf(name)
  return i > -1 ? process.argv[i + 1] : def
}
const BASE = arg('--base', 'http://127.0.0.1:8080')
const SHOTS = arg('--shots', '/tmp/styrman-shots')
const PROJECT = 'e2e-ui'

let passed = 0
let failed = 0
const ok = (label) => {
  passed++
  console.log(`PASS ${label}`)
}
const bad = (label, extra = '') => {
  failed++
  console.log(`FAIL ${label} ${extra}`)
}
const check = async (label, fn) => {
  try {
    await fn()
    ok(label)
  } catch (e) {
    bad(label, `— ${e.message.split('\n')[0]}`)
  }
}

// Pre-clean any leftover project from an aborted previous run.
await fetch(`${BASE}/api/projects/${PROJECT}?wipe=true`, { method: 'DELETE' }).catch(() => {})

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1440, height: 860 } })
page.setDefaultTimeout(15000)
const shot = (name) => page.screenshot({ path: `${SHOTS}/${name}.png` })

// Terminal text lives in the DOM (no canvas addon loaded).
const termText = (scope) => scope.locator('.xterm-rows').innerText()

try {
  // ── Home ────────────────────────────────────────────────────────────────
  await page.goto(BASE + '/')
  await check('app shell renders', () => page.waitForSelector('aside:has-text("styrman")'))
  await check('projects section present', () => page.waitForSelector('nav:has-text("Projects")'))
  await check('setup banner shows checklist', async () => {
    await page.waitForSelector('text=First-time setup')
    await page.waitForSelector('text=claude')
  })
  await shot('01-home')

  // ── Create a project (boots a real microVM) ─────────────────────────────
  await page.click('text=+ New Project')
  await page.fill('#np-name', PROJECT)
  await page.click('text=Claude Code')
  await shot('02-new-project-modal')
  await page.click('button:has-text("Create")')
  await check('navigates into conversation after create', () =>
    page.waitForURL(`**/p/${PROJECT}/1`, { timeout: 60000 }),
  )
  await check('conversation header shows title', () =>
    page.waitForSelector('header button:has-text("Conversation 1")'),
  )
  await check('project listed running (green dot)', () =>
    page.waitForSelector(`aside div:has-text("${PROJECT}") span[title="running"]`),
  )

  // ── Agent terminal: claude actually launches ────────────────────────────
  await check('claude output reaches the terminal', async () => {
    await page.waitForFunction(
      () => {
        const rows = document.querySelector('.xterm-rows')
        return rows && /claude|Claude|Welcome|theme|sign/i.test(rows.textContent ?? '')
      },
      { timeout: 45000 },
    )
  })
  await shot('03-claude-conversation')

  // ── Right rail: plain shell terminal, typing works ──────────────────────
  // NB: selectors must be exact + rail-scoped ('+ New' substring-matches the
  // sidebar's '+ New Project' and opens the wrong dialog).
  const railToggle = page.locator('button[title="Terminals"]')
  if (await railToggle.count()) await railToggle.click()
  const rail = page.locator('aside').filter({ hasText: 'Terminals' }).last()
  await rail.getByText('+ New', { exact: true }).click()
  await check('shell terminal opens in rail', () => rail.locator('.xterm').waitFor())
  await check('shell prompt appears', async () => {
    await page.waitForFunction(() => {
      const rails = [...document.querySelectorAll('.xterm-rows')]
      return rails.some((r) => /\$/.test(r.textContent ?? ''))
    })
  })
  await rail.locator('.xterm textarea').last().focus()
  await page.keyboard.type('echo styrman-e2e-$((21*2))', { delay: 15 })
  await page.keyboard.press('Enter')
  await check('typed command executes in microVM', () =>
    page.waitForFunction(() => {
      const rails = [...document.querySelectorAll('.xterm-rows')]
      return rails.some((r) => (r.textContent ?? '').includes('styrman-e2e-42'))
    }),
  )
  await shot('04-right-rail-shell')

  // ── Rename + second conversation ────────────────────────────────────────
  await page.click('header button:has-text("Conversation 1")')
  await page.fill('header input', 'Renamed by e2e')
  await page.keyboard.press('Enter')
  await check('rename sticks in header', () =>
    page.waitForSelector('header button:has-text("Renamed by e2e")'),
  )
  await check('rename visible in sidebar', () =>
    page.waitForSelector('aside >> text=Renamed by e2e'),
  )
  await page.click('text=+ New conversation')
  await check('second conversation opens', () => page.waitForURL(`**/p/${PROJECT}/2`))

  // ── Deep link ───────────────────────────────────────────────────────────
  await page.goto(`${BASE}/p/${PROJECT}/1`)
  await check('deep link reattaches conversation 1', () =>
    page.waitForSelector('header button:has-text("Renamed by e2e")'),
  )

  // ── Setup page ──────────────────────────────────────────────────────────
  await page.click('text=Open setup')
  await check('setup page boots hidden VM and attaches', async () => {
    await page.waitForURL('**/setup')
    await page.waitForFunction(
      () => {
        const rows = document.querySelector('.xterm-rows')
        return rows && /styrman setup|Git identity/i.test(rows.textContent ?? '')
      },
      { timeout: 60000 },
    )
  })
  await shot('05-setup-page')

  // ── Theme toggle ────────────────────────────────────────────────────────
  await page.click('text=☀ Light')
  await check('light theme applies', async () => {
    const theme = await page.evaluate(() => document.documentElement.dataset.theme)
    if (theme !== 'light') throw new Error(`theme=${theme}`)
  })
  await shot('06-light-theme')
  await page.click('text=☾ Dark')

  // ── Cleanup: delete conversation + project via UI ───────────────────────
  await page.goto(`${BASE}/p/${PROJECT}/2`)
  page.on('dialog', (d) => {
    console.log(`  [dialog:${d.type()}] ${d.message().slice(0, 100)}`)
    d.accept()
  })
  // Hover-revealed controls need force:true (visibility:hidden until hover).
  const convRow = page.locator('aside .group', { hasText: 'Conversation 2' }).last()
  await convRow.hover()
  await convRow.locator('button[aria-label^="Delete"]').click({ force: true })
  await check('conversation deleted from sidebar', async () => {
    await page.waitForFunction(
      () => !document.querySelector('aside')?.textContent?.includes('Conversation 2'),
    )
  })
  const projRow = page.locator('aside .group', { hasText: PROJECT }).first()
  await projRow.hover()
  await projRow.locator('button[aria-label*="actions"]').click({ force: true })
  // The inline action menu renders Stop/Start + a red Delete; wait for it.
  const menuDelete = page.locator('button.text-danger', { hasText: 'Delete' })
  await menuDelete.waitFor()
  await menuDelete.click()
  await check('project deleted from sidebar', async () => {
    await page.waitForFunction(
      (name) => !document.querySelector('nav')?.textContent?.includes(name),
      PROJECT,
      { timeout: 90000 },
    )
  })
  await shot('07-after-cleanup')
} catch (e) {
  bad('unexpected error', e.message.split('\n')[0])
  await shot('99-failure')
} finally {
  await browser.close()
  console.log(`\npassed=${passed} failed=${failed}`)
  process.exit(failed === 0 ? 0 : 1)
}
