# Accessibility

Reference file for the frontend skill. Read this when implementing accessible interfaces. Targets WCAG 2.2 Level AA.

## Quick Reference: Key Success Criteria

| SC | Name | Level | Requirement |
|----|------|-------|-------------|
| 1.4.3 | Contrast (Minimum) | AA | 4.5:1 normal text, 3:1 large text (≥18pt or ≥14pt bold) |
| 1.4.11 | Non-text Contrast | AA | 3:1 for UI controls and graphics |
| 2.4.7 | Focus Visible | AA | Focus indicator must be visible on all interactive elements |
| 2.4.11 | Focus Not Obscured | AA | Sticky/fixed elements must not hide the focused element |
| 2.5.8 | Target Size | AA | Interactive targets ≥ 24×24 CSS px (aim for 44×44) |
| 1.3.1 | Info and Relationships | A | Use semantic HTML; heading hierarchy; landmarks |
| 4.1.2 | Name, Role, Value | A | Interactive elements need accessible name + role |

---

## Color Contrast

### Dark Mode Safe Pairings

These combinations are pre-verified for WCAG AA compliance:

| Background | Foreground | Ratio | Use |
|-----------|-----------|-------|-----|
| `zinc-950` (#09090b) | `zinc-50` (#fafafa) | 19.4:1 | Primary text |
| `zinc-950` (#09090b) | `zinc-400` (#a1a1aa) | 7.5:1 | Secondary/muted text |
| `zinc-950` (#09090b) | `amber-400` (#fbbf24) | 11.3:1 | Accent text, links |
| `zinc-900` (#18181b) | `zinc-50` (#fafafa) | 17.1:1 | Primary text on card |
| `zinc-900` (#18181b) | `zinc-400` (#a1a1aa) | 6.6:1 | Muted text on card |
| `amber-400` (#fbbf24) | `zinc-950` (#09090b) | 11.3:1 | Button text on accent bg |

### Rules

- Never use text lighter than `zinc-400` on `zinc-950` backgrounds.
- Avoid `zinc-500` (#71717a) for body text — ratio drops to 4.7:1, barely passing for normal text and failing at smaller sizes.
- Do not rely on color alone to convey state. Pair with icons, labels, or shape changes.
- When placing text over images, add a gradient overlay or solid tonal area. Test contrast against both light and dark image regions.

---

## Focus States

### Default Focus Ring

Never remove `outline` without a replacement. Use `focus-visible` to show rings only for keyboard users.

```css
/* Base — applies to all interactive elements */
:focus-visible {
  outline: 2px solid var(--color-accent, #fbbf24);
  outline-offset: 2px;
  border-radius: 4px;
}

/* Remove default, replace with custom */
button:focus:not(:focus-visible),
a:focus:not(:focus-visible) {
  outline: none;
}
```

### Tailwind v4

```html
<button class="focus-visible:outline-2 focus-visible:outline-amber-400 focus-visible:outline-offset-2">
  Click me
</button>
```

### Focus Not Obscured

Sticky headers, floating CTAs, and cookie banners can hide the focused element. Ensure:

- `scroll-padding-top` equals the height of any fixed header
- Floating elements have `pointer-events: none` except on their interactive children
- Test by tabbing through the entire page with the keyboard

```css
html {
  scroll-padding-top: 5rem; /* match your fixed header height */
}
```

---

## Keyboard Navigation

### Principles

1. **Logical tab order.** Let DOM order drive focus order. Never use `tabindex > 0`.
2. **No keyboard traps.** Every modal, dropdown, or drawer must be closeable with `Escape`.
3. **Skip link.** Add a visually hidden "Skip to main content" link as the first focusable element.
4. **Use native elements.** `<button>`, `<a>`, `<input>`, `<select>` are keyboard-accessible by default. Custom `<div onClick>` requires `role`, `tabindex="0"`, and `onKeyDown` handling — avoid this.

### Skip Link

```tsx
function SkipLink() {
  return (
    <a
      href="#main-content"
      className="sr-only focus:not-sr-only focus:fixed focus:top-4 focus:left-4 focus:z-[999]
        focus:rounded-lg focus:bg-amber-400 focus:px-4 focus:py-2 focus:text-zinc-950 focus:text-sm"
    >
      Skip to main content
    </a>
  )
}

// In layout:
<SkipLink />
<Header />
<main id="main-content">{/* ... */}</main>
```

### Roving Tabindex for Tab Groups

When building tab bars or toolbars, only one item is tabbable. Arrow keys move focus.

```tsx
function TabBar({ tabs, active, onChange }: {
  tabs: string[]; active: number; onChange: (i: number) => void
}) {
  const handleKeyDown = (e: React.KeyboardEvent, i: number) => {
    if (e.key === "ArrowRight") onChange((i + 1) % tabs.length)
    if (e.key === "ArrowLeft") onChange((i - 1 + tabs.length) % tabs.length)
  }

  return (
    <div role="tablist" className="flex gap-1">
      {tabs.map((tab, i) => (
        <button key={tab} role="tab" aria-selected={i === active}
          tabIndex={i === active ? 0 : -1}
          onKeyDown={(e) => handleKeyDown(e, i)}
          onClick={() => onChange(i)}
          className={`px-4 py-2 text-sm rounded-lg transition-colors
            ${i === active ? "bg-zinc-800 text-zinc-50" : "text-zinc-400 hover:text-zinc-200"}`}>
          {tab}
        </button>
      ))}
    </div>
  )
}
```

---

## Semantic HTML Checklist

- One `<h1>` per page. Headings are hierarchical (`h1` > `h2` > `h3`). No skipping levels.
- Use `<main>`, `<nav>`, `<header>`, `<footer>`, `<aside>`, `<section>` for landmarks.
- Use `<button>` for actions, `<a>` for navigation. Never `<div onClick>`.
- Lists of items use `<ul>`/`<ol>` + `<li>`.
- Every `<img>` needs `alt`. Decorative images use `alt=""`.
- Form inputs need `<label htmlFor="id">`.
- Tables need `<thead>`, `<th scope="col">`.

---

## ARIA Patterns

### When to Use ARIA

The first rule of ARIA: **don't use ARIA if native HTML can do the job.** Only reach for ARIA when building custom widgets with no native equivalent.

### Common Patterns

| Widget | Key Attributes |
|--------|---------------|
| Modal | `role="dialog"`, `aria-modal="true"`, `aria-labelledby` |
| Toast/Alert | `role="alert"` or `aria-live="polite"` |
| Tab | `role="tablist"` + `role="tab"` + `role="tabpanel"`, `aria-selected` |
| Accordion | `aria-expanded`, `aria-controls` |
| Dropdown | `aria-haspopup`, `aria-expanded`, `role="menu"` + `role="menuitem"` |
| Toggle | `aria-pressed` (button) or `role="switch"` + `aria-checked` |
| Loading | `aria-busy="true"` on the container |

### Common Mistakes

- Adding `role="button"` to a `<button>` (redundant).
- Using `aria-label` when visible text exists (use `aria-labelledby` instead).
- Forgetting `aria-live="polite"` on dynamically inserted content.
- Using `aria-hidden="true"` on focusable elements (creates ghost focus).

---

## Reduced Motion

Respect `prefers-reduced-motion` to avoid triggering motion sickness or seizures.

### CSS Fallback

```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

### Motion Integration

Use the `useReducedMotion` hook to swap animated variants for instant ones.

```tsx
import { useReducedMotion, motion } from "motion/react"

function FadeIn({ children }: { children: ReactNode }) {
  const reduced = useReducedMotion()

  return (
    <motion.div
      initial={reduced ? false : { opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={reduced ? { duration: 0 } : { duration: 0.5 }}
    >
      {children}
    </motion.div>
  )
}
```

What to disable vs. simplify:
- **Disable:** parallax, scroll-hijacking, auto-playing carousels, large translate/scale animations
- **Simplify:** fade-in (keep but make instant), hover scale (reduce to opacity change)

---

## Testing

### Automated

- **axe-core:** Run via browser extension (axe DevTools) or in CI via `@axe-core/react`
- **Lighthouse:** Accessibility audit scores 90+ as baseline
- **eslint-plugin-jsx-a11y:** Catches common JSX accessibility issues at lint time

### Manual Checklist

1. Tab through entire page — can you reach and operate every interactive element?
2. Is focus always visible?
3. Can you close every modal/drawer with Escape?
4. Does content make sense with a screen reader? (test with VoiceOver on Mac: Cmd+F5)
5. Zoom to 200% — does layout remain usable?
6. Enable "Reduce motion" in OS settings — do animations respect it?

### Testing Tools

```bash
# Install axe for React (dev dependency)
npm install -D @axe-core/react

# In main.tsx (development only)
if (import.meta.env.DEV) {
  import("@axe-core/react").then((axe) => {
    axe.default(React, ReactDOM, 1000)
  })
}
```
