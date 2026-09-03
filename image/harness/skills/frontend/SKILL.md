---
name: frontend
description: |
  Frontend engineering and design for ALL frontend work: TanStack Start apps, dashboards,
  internal tools, admin surfaces, landing pages, and any TypeScript/Tailwind UI.
  Use when creating or modifying routes, components, hooks, server functions, forms,
  styling, or theming — even routine CRUD screens, small component edits, or "add a page"
  requests. Also use for visually-led work (landing pages, marketing sites, demos).
  Do NOT use for backend API logic (use golang), infrastructure-as-code (use terraform),
  CLI tools, or data processing pipelines.
---

Frontend standard: **TanStack Start + TypeScript + Tailwind CSS** for every frontend —
SPAs, dashboards, internal tools, and public-facing pages alike. When in doubt, use
TanStack Start.

## Tech Stack

- **Framework:** TanStack Start (full-stack TypeScript, type-safe routing, server functions).
- **SSR/SSG:** Built-in — configure per route via `ssr: true` or `prerender: true` in route options.
- **Styling:** Tailwind CSS. Raw CSS only for non-framework contexts (see Raw HTML/CSS Standards).
- **Animation:** Tailwind utilities first. Motion (`motion/react`) only when layout animations or scroll-linked effects are needed.
- **Theme:** Every frontend ships with dark/light theme support. Light mode is the default. Use CSS custom properties with `data-theme` attribute — see `./references/theme-system.md` for the token palette, FOWT prevention script, and ready-made `ThemeToggle` component.

## Project Structure

The frontend lives fully contained in one directory (`web/` or `ui/`) inside the Go
project — never scatter templates, assets, or styles across the Go package tree.

```
web/
├── app.config.ts        # The ONLY config file — do not create a separate vite.config.ts
├── src/
│   ├── routes/          # File-based routing (TanStack Router)
│   ├── components/      # Shared UI components
│   ├── lib/             # Utilities, API clients, shared types
│   └── hooks/           # Custom React hooks
└── ...
```

## TanStack Start Essentials

**File-based routing** — every route is a file in `routes/`; loaders co-locate data
fetching with the route:

```tsx
// src/routes/items.tsx
export const Route = createFileRoute('/items')({
  loader: () => getItems(),        // runs on server during SSR, type-safe on the client
  component: ItemsPage,
})
```

**Server-only logic** goes in `createServerFn` — DB access, secrets, internal API
calls. These never ship to the client and are co-located with the route or feature
that uses them:

```ts
// src/lib/items.ts
export const getItems = createServerFn({ method: 'GET' }).handler(async () => {
  return db.query(...)             // secrets/env stay server-side
})
```

Rules:

- Secrets and privileged calls **only** inside `createServerFn` handlers — never in
  components, loaders' client paths, or anything bundled to the browser.
- Type every prop and API payload. No `any` where a real type is feasible.
- **Deploy via Nitro presets** in `app.config.ts`: `node-server` for Cloud Run (GCP —
  the default target), `cloudflare-module` for Workers/V8 isolates.

## Apps & Dashboards (the default surface)

Default to Linear-style restraint: calm surface hierarchy, strong typography and
spacing, few colors, dense but readable information, minimal chrome. Cards only when
the card is the interaction — if a panel can become plain layout without losing
meaning, remove the card treatment.

Organize around: primary workspace, navigation, secondary context/inspector, and one
clear accent for action or state.

Avoid: dashboard-card mosaics, thick borders on every region, decorative gradients
behind routine product UI, multiple competing accent colors, ornamental icons that
don't improve scanning.

**Utility copy over marketing copy.** Product surfaces prioritize orientation,
status, and action:

- Start with the working surface itself (KPIs, tables, filters, status) — no hero
  section unless explicitly requested.
- Headings say what the area is or does: "Plan status", "Top segments", "Last sync".
- Supporting text explains scope, behavior, freshness, or decision value in one sentence.
- If a sentence could appear in a homepage hero or ad, rewrite it until it sounds
  like product UI.
- Litmus check: scanning only headings, labels, and numbers, can an operator
  understand the page immediately?

## Landing Pages & Visually-Led Work

For landing pages, marketing sites, brand pages, demos, and anything where art
direction carries the quality, read `./references/design.md` **before writing code**
— it defines the composition model, hero rules, imagery, copy, motion standards,
and the hard rules (no card grids, full-bleed heroes, brand-first hierarchy).

## Raw HTML/CSS Standards

These rules apply to **raw HTML/CSS files** outside of Tailwind-styled TanStack Start
components — email templates, static HTML pages, raw CSS files, and non-framework contexts.

> [!NOTE]
> Tailwind utility classes in TanStack Start components are **explicitly excluded** from
> these rules. Tailwind intentionally mixes style into markup, which is the correct
> pattern for component-based development.

### HTML

- Always use `<!doctype html>` (no-quirks mode)
- Use HTTPS for all embedded resources (`<script src="https://...">`)
- Use semantic HTML elements for their intended purpose (`<a>` not `<div onclick>`, `<button>` not `<span>`)
- Provide `alt` text for all meaningful images. Use `alt=""` for purely decorative images.
- Separate concerns: structure in HTML, style in CSS, behavior in JavaScript
- Validate HTML where possible
- 2-space indentation, no tabs
- Lowercase for all element and attribute names

### CSS (Non-Tailwind)

- 2-space indentation
- Lowercase for all selectors, properties, and values (except strings)
- Prefer shorthand properties where possible (`margin: 0 auto` not `margin-top: 0; margin-right: auto; ...`)
- Avoid ID selectors for styling — use classes
- Use meaningful class names that describe purpose, not appearance (`warning` not `big-red-text`)
- Omit units on zero values (`margin: 0` not `margin: 0px`)
- Omit leading zeros in decimals (`font-size: .8em` not `font-size: 0.8em`)

## Reference Library

For detailed patterns, examples, and implementation guidance, consult the reference files:

* **Design Standards (Composition, heroes, imagery, copy, motion, hard rules — for visually-led work)**
  Read → `./references/design.md`

* **Accessibility (WCAG 2.2 AA, Semantic HTML, ARIA, Focus Management, Color Contrast)**
  Read → `./references/accessibility.md`

* **Animation Recipes (Motion patterns, scroll-linked effects, layout transitions)**
  Read → `./references/animation-recipes.md`

* **Component Patterns (Navigation, Modals, Forms, Cards, Interactive UI)**
  Read → `./references/component-patterns.md`

* **Theme System (Dark/Light tokens, ThemeToggle component, Tailwind integration, FOWT prevention)**
  Read → `./references/theme-system.md`

> [!TIP]
> Building a landing page or brand surface? Read `design.md` first. Building
> interactive components or adding motion? Read the matching reference for
> copy-paste-ready patterns before writing code.

## Further Reading

- [Google HTML/CSS Style Guide](https://google.github.io/styleguide/htmlcssguide.html) — Google's conventions for raw HTML/CSS (non-conflicting principles adopted above)
