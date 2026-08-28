# Animation Recipes

Reference file for the frontend skill. Copy-paste-ready Motion patterns in TypeScript + TanStack Start.
All patterns use `motion/react` and work identically in TanStack Start (which is React-based).

Import all Motion APIs from `"motion/react"`:
```tsx
import { motion, AnimatePresence, useScroll, useTransform, useSpring, useReducedMotion, useMotionTemplate } from "motion/react"
```

---

## 1. Staggered Entrance

Parent orchestrates children with `staggerChildren`. Used for hero sequences, list reveals, and feature sections.

```tsx
const containerVariants = {
  hidden: {},
  visible: {
    transition: { staggerChildren: 0.08, delayChildren: 0.1 },
  },
}

const itemVariants = {
  hidden: { opacity: 0, y: 20 },
  visible: {
    opacity: 1,
    y: 0,
    transition: { duration: 0.5, ease: [0.25, 0.4, 0.25, 1] },
  },
}

function HeroContent() {
  return (
    <motion.div variants={containerVariants} initial="hidden" animate="visible">
      <motion.span variants={itemVariants} className="text-sm text-amber-400">Brand</motion.span>
      <motion.h1 variants={itemVariants} className="text-5xl font-bold text-zinc-50">Ship faster.</motion.h1>
      <motion.p variants={itemVariants} className="text-zinc-400">The command center for teams that build.</motion.p>
      <motion.button variants={itemVariants} className="bg-amber-400 px-6 py-3 rounded-lg text-zinc-950">
        Get Started
      </motion.button>
    </motion.div>
  )
}
```

---

## 2. Scroll-Triggered Reveal

Elements fade and slide up when they enter the viewport. Fire once with `viewport={{ once: true }}`.

```tsx
function ScrollReveal({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.2 }}
      transition={{ duration: 0.6, ease: [0.25, 0.4, 0.25, 1] }}
      className={className}
    >
      {children}
    </motion.div>
  )
}

// Usage — wrap any section
<ScrollReveal>
  <FeatureSection />
</ScrollReveal>
```

### With Stagger (multiple items)

```tsx
function StaggeredScrollReveal({ children }: { children: ReactNode }) {
  return (
    <motion.div
      initial="hidden"
      whileInView="visible"
      viewport={{ once: true, amount: 0.15 }}
      variants={{
        hidden: {},
        visible: { transition: { staggerChildren: 0.1 } },
      }}
    >
      {children}
    </motion.div>
  )
}

// Children use itemVariants from Recipe 1
```

---

## 3. Parallax Section

Background moves slower than foreground as you scroll. Uses `useScroll` with a target element and `useTransform` to map progress to Y offset.

```tsx
function ParallaxSection() {
  const ref = useRef<HTMLDivElement>(null)
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start end", "end start"],
  })

  const bgY = useTransform(scrollYProgress, [0, 1], ["-10%", "10%"])
  const fgY = useTransform(scrollYProgress, [0, 1], ["5%", "-5%"])

  return (
    <section ref={ref} className="relative overflow-hidden h-[80vh]">
      {/* Background layer — moves slow */}
      <motion.div style={{ y: bgY }} className="absolute inset-0">
        <img src="/bg.jpg" alt="" className="w-full h-[120%] object-cover" aria-hidden="true" />
      </motion.div>

      {/* Foreground content — moves opposite */}
      <motion.div style={{ y: fgY }} className="relative z-10 flex items-center justify-center h-full">
        <h2 className="text-4xl font-bold text-white">Depth in motion</h2>
      </motion.div>
    </section>
  )
}
```

---

## 4. Scroll Progress Bar

Fixed bar at the top of the page. `scaleX` linked to page scroll progress, smoothed with `useSpring`.

```tsx
function ScrollProgressBar() {
  const { scrollYProgress } = useScroll()
  const scaleX = useSpring(scrollYProgress, {
    stiffness: 100,
    damping: 30,
    restDelta: 0.001,
  })

  return (
    <motion.div
      style={{ scaleX, transformOrigin: "0%" }}
      className="fixed top-0 left-0 right-0 z-[100] h-0.5 bg-amber-400"
    />
  )
}
```

---

## 5. Hover Lift

Subtle lift + shadow on hover. For pricing cards, CTAs, and interactive elements.

```tsx
function HoverCard({ children }: { children: ReactNode }) {
  return (
    <motion.div
      whileHover={{ y: -4, scale: 1.02 }}
      transition={{ type: "spring", stiffness: 300, damping: 20 }}
      className="rounded-xl border border-zinc-800 bg-zinc-900 p-6
        hover:shadow-lg hover:shadow-amber-400/5 transition-shadow"
    >
      {children}
    </motion.div>
  )
}
```

### Button Hover

```tsx
<motion.button
  whileHover={{ scale: 1.03 }}
  whileTap={{ scale: 0.97 }}
  transition={{ type: "spring", stiffness: 400, damping: 15 }}
  className="rounded-lg bg-amber-400 px-6 py-3 text-sm font-medium text-zinc-950"
>
  Get Started
</motion.button>
```

---

## 6. Modal Enter/Exit

Use `AnimatePresence` for exit animations. Backdrop fades, panel scales up.

```tsx
function AnimatedModal({ open, onClose, children }: {
  open: boolean; onClose: () => void; children: ReactNode
}) {
  return (
    <AnimatePresence>
      {open && (
        <>
          {/* Backdrop */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            onClick={onClose}
            className="fixed inset-0 z-50 bg-black/70"
          />
          {/* Panel */}
          <motion.div
            initial={{ opacity: 0, scale: 0.95, y: 10 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95, y: 10 }}
            transition={{ duration: 0.2, ease: [0.25, 0.4, 0.25, 1] }}
            className="fixed inset-0 z-50 flex items-center justify-center"
          >
            <div
              onClick={(e) => e.stopPropagation()}
              role="dialog"
              aria-modal="true"
              className="w-full max-w-lg rounded-xl bg-zinc-900 p-6 shadow-2xl"
            >
              {children}
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  )
}
```

---

## 7. Sticky Header Blur

Header starts transparent, gains blur and opacity as you scroll.

```tsx
function StickyHeader() {
  const { scrollY } = useScroll()
  const bgOpacity = useTransform(scrollY, [0, 100], [0, 0.85])
  const blur = useTransform(scrollY, [0, 100], [0, 12])

  return (
    <motion.header
      className="fixed top-0 inset-x-0 z-50"
      style={{
        backgroundColor: useMotionTemplate`rgba(9,9,11,${bgOpacity})`,
        backdropFilter: useMotionTemplate`blur(${blur}px)`,
      }}
    >
      <nav className="mx-auto max-w-7xl flex items-center justify-between px-6 py-4">
        <span className="text-lg font-semibold text-zinc-50">Brand</span>
        <button className="rounded-lg bg-amber-400 px-4 py-2 text-sm text-zinc-950">CTA</button>
      </nav>
    </motion.header>
  )
}
```

---

## 8. Reduced Motion Wrapper

Wraps any animated component to respect `prefers-reduced-motion`. Disables entrance/scroll animations; simplifies hover effects.

```tsx
import { useReducedMotion } from "motion/react"

function SafeMotion({
  children,
  initial,
  animate,
  whileInView,
  transition,
  ...rest
}: React.ComponentProps<typeof motion.div>) {
  const reduced = useReducedMotion()

  return (
    <motion.div
      initial={reduced ? false : initial}
      animate={reduced ? animate : animate}
      whileInView={reduced ? undefined : whileInView}
      transition={reduced ? { duration: 0 } : transition}
      {...rest}
    >
      {children}
    </motion.div>
  )
}

// Usage
<SafeMotion
  initial={{ opacity: 0, y: 30 }}
  whileInView={{ opacity: 1, y: 0 }}
  viewport={{ once: true }}
  transition={{ duration: 0.6 }}
>
  <SectionContent />
</SafeMotion>
```

When `prefers-reduced-motion` is active:
- `initial` is skipped (element renders in final state)
- `whileInView` is disabled (no scroll triggers)
- All transitions are instant (`duration: 0`)

---

## Easing Reference

Recommended easing curves for the frontend skill's aesthetic:

| Name | Value | Use |
|------|-------|-----|
| Smooth | `[0.25, 0.4, 0.25, 1]` | Default for reveals, fades |
| Snappy | `[0.16, 1, 0.3, 1]` | Quick interactions, hovers |
| Bounce | `type: "spring", stiffness: 300, damping: 20` | Cards, buttons |
| Slow reveal | `{ duration: 0.8, ease: [0.25, 0.4, 0.25, 1] }` | Hero entrances |

---

## Performance Notes

- Animate `transform` and `opacity` only — these run on the compositor thread and don't trigger layout/paint.
- Avoid animating `width`, `height`, `top`, `left` when possible. Use `scale` and `translate` instead.
- Use `LazyMotion` + `domAnimation` to reduce Motion's bundle size (~5KB instead of ~30KB). In TanStack Start, add this to your root route component:

```tsx
import { LazyMotion, domAnimation, m } from "motion/react"

function App() {
  return (
    <LazyMotion features={domAnimation}>
      <m.div animate={{ opacity: 1 }} />
    </LazyMotion>
  )
}
```

- For scroll-linked animations, Motion uses native `ScrollTimeline` when available — no JS per frame.
- Audit with the Motion Score tool: https://motion.dev/tools/motion-score
