# Component Patterns

Reference file for the frontend skill. Read this when building interactive UI components in TanStack Start.
All patterns use React and `motion/react` — they work identically in TanStack Start.

## Navigation

### Sticky Header with Scroll-Aware Transparency

The header overlays the hero and gains a blurred background on scroll. Avoids consuming viewport budget.

```tsx
import { motion, useScroll, useTransform, useMotionTemplate } from "motion/react"

function Header() {
  const { scrollY } = useScroll()
  const bgOpacity = useTransform(scrollY, [0, 100], [0, 0.85])
  const blur = useTransform(scrollY, [0, 100], [0, 12])

  return (
    <motion.header
      className="fixed top-0 inset-x-0 z-50 border-b border-white/5"
      style={{
        backgroundColor: useMotionTemplate`rgba(9,9,11,${bgOpacity})`,
        backdropFilter: useMotionTemplate`blur(${blur}px)`,
      }}
    >
      <nav className="mx-auto flex max-w-7xl items-center justify-between px-6 py-4">
        <a href="/" className="text-lg font-semibold text-zinc-50">Brand</a>
        <div className="hidden md:flex items-center gap-8">
          <a href="#features" className="text-sm text-zinc-400 hover:text-zinc-50 transition-colors">Features</a>
          <button className="rounded-lg bg-amber-400 px-4 py-2 text-sm font-medium text-zinc-950">
            Get Started
          </button>
        </div>
      </nav>
    </motion.header>
  )
}
```

### Mobile Off-Canvas Navigation

Slide-in panel with focus trapping, ESC to close, backdrop click to close.

```tsx
import { AnimatePresence, motion } from "motion/react"
import { useEffect, useRef, useState } from "react"

function MobileNav() {
  const [open, setOpen] = useState(false)
  const closeRef = useRef<HTMLButtonElement>(null)

  useEffect(() => { if (open) closeRef.current?.focus() }, [open])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false) }
    document.addEventListener("keydown", h)
    return () => document.removeEventListener("keydown", h)
  }, [])

  return (
    <>
      <button onClick={() => setOpen(true)} aria-label="Open menu" className="md:hidden p-2">☰</button>
      <AnimatePresence>
        {open && (
          <>
            <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
              onClick={() => setOpen(false)} className="fixed inset-0 z-40 bg-black/60" aria-hidden="true" />
            <motion.nav initial={{ x: "100%" }} animate={{ x: 0 }} exit={{ x: "100%" }}
              transition={{ type: "spring", damping: 25, stiffness: 200 }}
              className="fixed right-0 top-0 bottom-0 z-50 w-72 bg-zinc-950 p-6"
              role="dialog" aria-modal="true" aria-label="Navigation">
              <button ref={closeRef} onClick={() => setOpen(false)} aria-label="Close menu"
                className="mb-8 p-2 text-zinc-400 hover:text-zinc-50">✕</button>
              <ul className="space-y-4">
                <li><a href="#features" className="text-lg text-zinc-200">Features</a></li>
                <li><a href="#pricing" className="text-lg text-zinc-200">Pricing</a></li>
              </ul>
            </motion.nav>
          </>
        )}
      </AnimatePresence>
    </>
  )
}
```

---

## Modals & Dialogs

Use React Portals or the native `<dialog>` element. Compound Component pattern for flexibility.

```tsx
import { AnimatePresence, motion } from "motion/react"
import { createPortal } from "react-dom"
import { createContext, useContext, useEffect, type ReactNode } from "react"

const ModalCtx = createContext<{ close: () => void }>({ close: () => {} })

function Modal({ open, onClose, children }: { open: boolean; onClose: () => void; children: ReactNode }) {
  useEffect(() => {
    if (open) document.body.style.overflow = "hidden"
    return () => { document.body.style.overflow = "" }
  }, [open])

  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === "Escape") onClose() }
    if (open) document.addEventListener("keydown", h)
    return () => document.removeEventListener("keydown", h)
  }, [open, onClose])

  return createPortal(
    <AnimatePresence>
      {open && (
        <ModalCtx.Provider value={{ close: onClose }}>
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            onClick={onClose} className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center">
            <motion.div initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }} onClick={(e) => e.stopPropagation()}
              role="dialog" aria-modal="true"
              className="relative w-full max-w-lg rounded-xl bg-zinc-900 p-6 shadow-2xl">
              {children}
            </motion.div>
          </motion.div>
        </ModalCtx.Provider>
      )}
    </AnimatePresence>,
    document.body
  )
}

function ModalHeader({ children }: { children: ReactNode }) {
  const { close } = useContext(ModalCtx)
  return (
    <div className="mb-4 flex items-center justify-between">
      <h2 className="text-lg font-semibold text-zinc-50">{children}</h2>
      <button onClick={close} aria-label="Close" className="text-zinc-400 hover:text-zinc-50">✕</button>
    </div>
  )
}
Modal.Header = ModalHeader
Modal.Body = ({ children }: { children: ReactNode }) => <div className="text-zinc-300">{children}</div>
Modal.Footer = ({ children }: { children: ReactNode }) => <div className="mt-6 flex justify-end gap-3">{children}</div>
```

Key rules:
- Lock body scroll when open
- ESC to close, backdrop click to close
- `role="dialog"` and `aria-modal="true"`
- Stop click propagation on the panel

---

## Toast Notifications

Context/Provider pattern for global access. Non-intrusive — use modals for critical errors.

```tsx
import { AnimatePresence, motion } from "motion/react"
import { createContext, useCallback, useContext, useState, type ReactNode } from "react"

type Toast = { id: string; message: string; type: "success" | "error" | "info" }
const ToastCtx = createContext<{ toast: (msg: string, type?: Toast["type"]) => void }>({ toast: () => {} })
export const useToast = () => useContext(ToastCtx)

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const toast = useCallback((message: string, type: Toast["type"] = "info") => {
    const id = crypto.randomUUID()
    setToasts((p) => [...p, { id, message, type }])
    setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 4000)
  }, [])

  return (
    <ToastCtx.Provider value={{ toast }}>
      {children}
      <div className="fixed bottom-6 right-6 z-[100] flex flex-col gap-2">
        <AnimatePresence>
          {toasts.map((t) => (
            <motion.div key={t.id} initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
              className={`rounded-lg px-4 py-3 text-sm shadow-lg ${
                t.type === "success" ? "bg-emerald-600 text-white" :
                t.type === "error" ? "bg-red-600 text-white" : "bg-zinc-800 text-zinc-200"
              }`}>{t.message}</motion.div>
          ))}
        </AnimatePresence>
      </div>
    </ToastCtx.Provider>
  )
}
```

---

## Forms

Prefer React Hook Form + Zod for client-side forms. Use `register` (uncontrolled) to minimise re-renders.
For forms that submit to server functions, pair with `createServerFn` for type-safe server-side handling.

```tsx
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"

const schema = z.object({
  email: z.string().email("Enter a valid email"),
  name: z.string().min(2, "Name is required"),
})
type FormData = z.infer<typeof schema>

function SignupForm() {
  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm<FormData>({
    resolver: zodResolver(schema),
  })

  return (
    <form onSubmit={handleSubmit(async (d) => fetch("/api/signup", { method: "POST", body: JSON.stringify(d) }))}
      className="space-y-4" noValidate>
      <div>
        <label htmlFor="name" className="block text-sm text-zinc-400 mb-1">Name</label>
        <input id="name" {...register("name")}
          className="w-full rounded-lg bg-zinc-800 px-4 py-2.5 text-zinc-50 border border-zinc-700
            focus:border-amber-400 focus:outline-none focus:ring-2 focus:ring-amber-400/20"
          aria-invalid={errors.name ? "true" : undefined}
          aria-describedby={errors.name ? "name-err" : undefined} />
        {errors.name && <p id="name-err" role="alert" className="mt-1 text-sm text-red-400">{errors.name.message}</p>}
      </div>
      <button type="submit" disabled={isSubmitting}
        className="w-full rounded-lg bg-amber-400 py-2.5 text-sm font-medium text-zinc-950
          hover:bg-amber-300 disabled:opacity-50 transition-colors">
        {isSubmitting ? "Submitting…" : "Sign Up"}
      </button>
    </form>
  )
}
```

Form rules:
- Every input needs a visible `<label>` with `htmlFor`
- Use `aria-invalid` and `aria-describedby` for error association
- Show errors inline, not in toasts
- Disable the submit button during submission

---

## Loading States

### Skeleton Screens

Prefer skeletons over spinners. They communicate structure and prevent layout shift.

```tsx
function Skeleton({ className }: { className?: string }) {
  return <div className={`animate-pulse rounded-lg bg-zinc-800 ${className ?? ""}`} aria-hidden="true" />
}

function CardSkeleton() {
  return (
    <div className="space-y-3" aria-busy="true" aria-label="Loading">
      <Skeleton className="h-48 w-full" />
      <Skeleton className="h-4 w-3/4" />
      <Skeleton className="h-4 w-1/2" />
    </div>
  )
}
```

### Route-Based Code Splitting

TanStack Router automatically code-splits per route. For component-level splitting, use `lazy()`:

```tsx
import { lazy, Suspense } from "react"
const PricingTable = lazy(() => import("./PricingTable"))

function PricingPage() {
  return (
    <Suspense fallback={<PricingTableSkeleton />}>
      <PricingTable />
    </Suspense>
  )
}
```

For route-level lazy loading, define a `.lazy.tsx` file alongside the route:

```tsx
// routes/pricing.lazy.tsx — loaded on demand when the route is visited
import { createLazyFileRoute } from '@tanstack/react-router'

export const Route = createLazyFileRoute('/pricing')({
  component: PricingPage,
})
```

---

## Section Layout Patterns

### Full-Bleed Section

```tsx
function FullBleedSection({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <section className={`w-full ${className ?? ""}`}>
      <div className="mx-auto max-w-7xl px-6">{children}</div>
    </section>
  )
}
```

### Alternating Two-Column (Cardless Features)

```tsx
function Feature({ title, description, imageSrc, imageAlt, reverse }: {
  title: string; description: string; imageSrc: string; imageAlt: string; reverse?: boolean
}) {
  return (
    <div className={`flex flex-col gap-8 py-16 md:flex-row md:items-center md:gap-16
      ${reverse ? "md:flex-row-reverse" : ""}`}>
      <div className="flex-1">
        <h3 className="text-2xl font-semibold text-zinc-50">{title}</h3>
        <p className="mt-3 text-zinc-400 leading-relaxed">{description}</p>
      </div>
      <div className="flex-1">
        <img src={imageSrc} alt={imageAlt} className="rounded-xl" width={600} height={400} loading="lazy" />
      </div>
    </div>
  )
}
```

### Responsive Data Table

On mobile, stack rows vertically. On desktop, standard table.

```tsx
function ResponsiveTable({ headers, rows }: { headers: string[]; rows: string[][] }) {
  return (
    <>
      <table className="hidden w-full md:table">
        <thead>
          <tr className="border-b border-zinc-800">
            {headers.map((h) => <th key={h} className="py-3 text-left text-sm font-medium text-zinc-400">{h}</th>)}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} className="border-b border-zinc-800/50">
              {row.map((cell, j) => <td key={j} className="py-3 text-sm text-zinc-300">{cell}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
      <div className="space-y-4 md:hidden">
        {rows.map((row, i) => (
          <div key={i} className="rounded-lg border border-zinc-800 p-4 space-y-2">
            {row.map((cell, j) => (
              <div key={j} className="flex justify-between">
                <span className="text-sm text-zinc-500">{headers[j]}</span>
                <span className="text-sm text-zinc-200">{cell}</span>
              </div>
            ))}
          </div>
        ))}
      </div>
    </>
  )
}
```
