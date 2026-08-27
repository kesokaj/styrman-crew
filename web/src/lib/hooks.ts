import { useCallback, useEffect, useRef, useState } from 'react'

/** Poll an async fetcher on an interval; exposes an immediate refresh. */
export function usePolled<T>(fetcher: () => Promise<T>, intervalMs: number): {
  data: T | undefined
  refresh: () => void
  error: string | undefined
} {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<string>()
  const fetcherRef = useRef(fetcher)
  useEffect(() => {
    fetcherRef.current = fetcher
  })
  // Guards: skip if a request is still in flight (slow VM boots stacked polls
  // otherwise), drop stale responses (a slow earlier reply must not overwrite
  // a newer one), and ignore results after unmount.
  const inFlight = useRef(false)
  const gen = useRef(0)
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const refresh = useCallback(() => {
    if (inFlight.current) return
    inFlight.current = true
    const myGen = ++gen.current
    fetcherRef
      .current()
      .then((d) => {
        if (!mounted.current || myGen !== gen.current) return
        setData(d)
        setError(undefined)
      })
      .catch((e: Error) => {
        if (!mounted.current || myGen !== gen.current) return
        setError(e.message)
      })
      .finally(() => {
        inFlight.current = false
      })
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, intervalMs)
    return () => clearInterval(t)
  }, [refresh, intervalMs])

  return { data, refresh, error }
}

const THEME_KEY = 'styrman-theme'

/** Dark-default theme with persisted toggle (data-theme on <html>). */
export function useTheme(): { theme: 'dark' | 'light'; toggle: () => void } {
  const [theme, setTheme] = useState<'dark' | 'light'>(() => {
    try {
      return localStorage.getItem(THEME_KEY) === 'light' ? 'light' : 'dark'
    } catch {
      return 'dark'
    }
  })
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    try {
      localStorage.setItem(THEME_KEY, theme)
    } catch {
      /* private mode */
    }
  }, [theme])
  return { theme, toggle: () => setTheme((t) => (t === 'dark' ? 'light' : 'dark')) }
}
