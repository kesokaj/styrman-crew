# Theme System

Reference file for the frontend skill. Every frontend ships with dark/light theme support. **Light mode is the default.**

## Design Tokens

All UI colors are defined as CSS custom properties. Components reference tokens — never raw hex values.

### 🌞 Light Mode — VS Code Light Modern

Source: [microsoft/vscode `light_modern.json`](https://github.com/microsoft/vscode/blob/main/extensions/theme-defaults/themes/light_modern.json)

| Token | Hex | Usage |
|-------|-----|-------|
| `--bg-primary` | `#FFFFFF` | Page background |
| `--bg-secondary` | `#F8F8F8` | Cards, sidebar, panels |
| `--bg-tertiary` | `#F3F3F3` | Code blocks, wells |
| `--bg-hover` | `#F2F2F2` | List/row hover |
| `--bg-active` | `#E8E8E8` | Active selection |
| `--bg-input` | `#FFFFFF` | Input fields |
| `--fg-primary` | `#3B3B3B` | Body text |
| `--fg-secondary` | `#616161` | Muted text |
| `--fg-tertiary` | `#8B949E` | Placeholder, disabled |
| `--fg-heading` | `#1F1F1F` | Headings |
| `--fg-inverse` | `#FFFFFF` | Text on accent bg |
| `--accent` | `#005FB8` | Buttons, links, focus |
| `--accent-hover` | `#0258A8` | Button hover |
| `--border` | `#E5E5E5` | Dividers, card borders |
| `--border-input` | `#CECECE` | Input/dropdown borders |
| `--success` | `#2EA043` | Success states |
| `--error` | `#F85149` | Error states |
| `--warning` | `#BB8009` | Warning states |
| `--focus-ring` | `#005FB8` | Focus outline |

### 🌙 Dark Mode — One Dark Pro Night Flat

Source: [Binaryify/OneDark-Pro `OneDark-Pro-night-flat.json`](https://github.com/Binaryify/OneDark-Pro/blob/master/themes/OneDark-Pro-night-flat.json)

| Token | Hex | Usage |
|-------|-----|-------|
| `--bg-primary` | `#16191d` | Page background |
| `--bg-secondary` | `#1e2227` | Cards, dropdowns |
| `--bg-tertiary` | `#23272e` | Elevated surfaces |
| `--bg-hover` | `#2c313a` | List/row hover |
| `--bg-active` | `#2c313a` | Active selection |
| `--bg-input` | `#1d1f23` | Input fields |
| `--fg-primary` | `#abb2bf` | Body text |
| `--fg-secondary` | `#9da5b4` | Muted text |
| `--fg-tertiary` | `#5c6370` | Placeholder, disabled |
| `--fg-heading` | `#d7dae0` | Headings |
| `--fg-inverse` | `#16191d` | Text on accent bg |
| `--accent` | `#61afef` | Buttons, links, focus |
| `--accent-hover` | `#528bff` | Button hover |
| `--border` | `#37393d` | Dividers, card borders |
| `--border-input` | `#3e4452` | Input/panel borders |
| `--success` | `#98c379` | Success states |
| `--error` | `#e06c75` | Error states |
| `--warning` | `#d19a66` | Warning states |
| `--focus-ring` | `#3e4452` | Focus outline |

### Accent Palette (Dark mode decorative colors)

Use for tags, badges, syntax highlights, and decorative elements:

| Name | Hex | Role |
|------|-----|------|
| Coral | `#e06c75` | Destructive, alerts |
| Sand | `#d19a66` | Warnings, constants |
| Gold | `#e5c07b` | Types, highlights |
| Sage | `#98c379` | Success, confirmed |
| Sky | `#61afef` | Links, info |
| Lilac | `#c678dd` | Keywords, premium |
| Teal | `#56b6c2` | Info badges, metadata |

---

## Copy-Paste: `tokens.css`

Drop this file into your project. All components reference these variables.

```css
/* tokens.css — Theme design tokens
   Light: VS Code Light Modern
   Dark:  One Dark Pro Night Flat
   Default: light */

:root,
[data-theme="light"] {
  color-scheme: light;

  /* Backgrounds */
  --bg-primary: #FFFFFF;
  --bg-secondary: #F8F8F8;
  --bg-tertiary: #F3F3F3;
  --bg-hover: #F2F2F2;
  --bg-active: #E8E8E8;
  --bg-input: #FFFFFF;

  /* Foregrounds */
  --fg-primary: #3B3B3B;
  --fg-secondary: #616161;
  --fg-tertiary: #8B949E;
  --fg-heading: #1F1F1F;
  --fg-inverse: #FFFFFF;

  /* Accent */
  --accent: #005FB8;
  --accent-hover: #0258A8;

  /* Borders */
  --border: #E5E5E5;
  --border-input: #CECECE;

  /* Semantic */
  --success: #2EA043;
  --error: #F85149;
  --warning: #BB8009;

  /* Focus */
  --focus-ring: #005FB8;

  /* Shadows */
  --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.05);
  --shadow-md: 0 4px 6px rgba(0, 0, 0, 0.07);
  --shadow-lg: 0 10px 15px rgba(0, 0, 0, 0.1);

  /* Transitions */
  --transition-theme: background-color 0.2s ease, color 0.2s ease, border-color 0.2s ease;
}

[data-theme="dark"] {
  color-scheme: dark;

  /* Backgrounds */
  --bg-primary: #16191d;
  --bg-secondary: #1e2227;
  --bg-tertiary: #23272e;
  --bg-hover: #2c313a;
  --bg-active: #2c313a;
  --bg-input: #1d1f23;

  /* Foregrounds */
  --fg-primary: #abb2bf;
  --fg-secondary: #9da5b4;
  --fg-tertiary: #5c6370;
  --fg-heading: #d7dae0;
  --fg-inverse: #16191d;

  /* Accent */
  --accent: #61afef;
  --accent-hover: #528bff;

  /* Borders */
  --border: #37393d;
  --border-input: #3e4452;

  /* Semantic */
  --success: #98c379;
  --error: #e06c75;
  --warning: #d19a66;

  /* Focus */
  --focus-ring: #3e4452;

  /* Shadows */
  --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.3);
  --shadow-md: 0 4px 6px rgba(0, 0, 0, 0.4);
  --shadow-lg: 0 10px 15px rgba(0, 0, 0, 0.5);

  /* Transitions — same var, same timing */
  --transition-theme: background-color 0.2s ease, color 0.2s ease, border-color 0.2s ease;

  /* Dark accent palette (decorative) */
  --accent-coral: #e06c75;
  --accent-sand: #d19a66;
  --accent-gold: #e5c07b;
  --accent-sage: #98c379;
  --accent-sky: #61afef;
  --accent-lilac: #c678dd;
  --accent-teal: #56b6c2;
}

/* Base application styles */
body {
  background-color: var(--bg-primary);
  color: var(--fg-primary);
  transition: var(--transition-theme);
}
```

---

## FOWT Prevention (Flash of Wrong Theme)

Add this inline script to `<head>` **before** any stylesheets. It sets `data-theme` synchronously so the browser never renders the wrong theme.

```html
<script>
  (function() {
    var t = localStorage.getItem('theme');
    if (!t) {
      t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }
    document.documentElement.dataset.theme = t;
  })();
</script>
```

**Priority cascade:**

| Priority | Source | Behavior |
|----------|--------|----------|
| 1 | `localStorage` | User's explicit choice persists across sessions |
| 2 | `prefers-color-scheme` | Respects OS/browser dark mode setting |
| 3 | Fallback | **Light mode** |

For **TanStack Start**, add the FOWT script to your `index.html` in the `<head>` section:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <script>
      (function() {
        var t = localStorage.getItem('theme');
        if (!t) {
          t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
        }
        document.documentElement.dataset.theme = t;
      })();
    </script>
  </head>
  <body>
    <div id="root"></div>
  </body>
</html>
```

---

## Ready-Made Component: `ThemeToggle.tsx`

A self-contained React component. No external dependencies — uses inline SVG icons.
Works in TanStack Start without any special directives.

```tsx

import { useEffect, useState } from "react";

type Theme = "light" | "dark";

function getInitialTheme(): Theme {
  if (typeof window === "undefined") return "light";
  const stored = localStorage.getItem("theme") as Theme | null;
  if (stored === "light" || stored === "dark") return stored;
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(getInitialTheme);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("theme", theme);
  }, [theme]);

  const toggle = () => setTheme((prev) => (prev === "dark" ? "light" : "dark"));

  return (
    <button
      onClick={toggle}
      aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}
      title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}
      style={{
        background: "none",
        border: "1px solid var(--border)",
        borderRadius: "8px",
        padding: "8px",
        cursor: "pointer",
        color: "var(--fg-primary)",
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        transition: "var(--transition-theme)",
      }}
    >
      {theme === "dark" ? <SunIcon /> : <MoonIcon />}
    </button>
  );
}

function SunIcon() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <circle cx="12" cy="12" r="5" />
      <line x1="12" y1="1" x2="12" y2="3" />
      <line x1="12" y1="21" x2="12" y2="23" />
      <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
      <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
      <line x1="1" y1="12" x2="3" y2="12" />
      <line x1="21" y1="12" x2="23" y2="12" />
      <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
      <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  );
}
```

---

## Tailwind CSS Integration

Wire the CSS custom properties into Tailwind so you can use `bg-primary`, `text-heading`, etc.:

```ts
// tailwind.config.ts
import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        primary: "var(--bg-primary)",
        secondary: "var(--bg-secondary)",
        tertiary: "var(--bg-tertiary)",
        hover: "var(--bg-hover)",
        active: "var(--bg-active)",
        input: "var(--bg-input)",
        accent: {
          DEFAULT: "var(--accent)",
          hover: "var(--accent-hover)",
        },
        success: "var(--success)",
        error: "var(--error)",
        warning: "var(--warning)",
      },
      textColor: {
        primary: "var(--fg-primary)",
        secondary: "var(--fg-secondary)",
        tertiary: "var(--fg-tertiary)",
        heading: "var(--fg-heading)",
        inverse: "var(--fg-inverse)",
      },
      borderColor: {
        DEFAULT: "var(--border)",
        input: "var(--border-input)",
      },
      ringColor: {
        focus: "var(--focus-ring)",
      },
      boxShadow: {
        sm: "var(--shadow-sm)",
        md: "var(--shadow-md)",
        lg: "var(--shadow-lg)",
      },
    },
  },
  plugins: [],
};

export default config;
```

With this config, Tailwind classes like `bg-primary`, `text-heading`, `border-input`, `ring-focus` automatically adapt to the active theme.

> [!IMPORTANT]
> You do **not** need `darkMode: 'class'` or the `dark:` prefix. The CSS custom properties handle everything — one class, both themes.

---

## Usage Checklist

When starting a new frontend:

1. Copy `tokens.css` into your styles directory
2. Import it as the **first** stylesheet (`@import './tokens.css'`)
3. Add the FOWT prevention script to `index.html` `<head>`
4. Drop `<ThemeToggle />` into your header/navbar
5. Use `var(--token-name)` in CSS or Tailwind utility classes
6. **Never use raw hex values** — always reference tokens

### Quick Test

After setup, verify:
- Page loads in light mode (default)
- Toggle switches to dark mode
- Refresh preserves the choice
- Set OS to dark mode, clear `localStorage`, refresh → should pick dark
- No flash of wrong theme on any page load
