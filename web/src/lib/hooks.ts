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

  const refresh = useCallback(() => {
    fetcherRef
      .current()
      .then((d) => {
        setData(d)
        setError(undefined)
      })
      .catch((e: Error) => setError(e.message))
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
