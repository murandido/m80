"use client"

import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"

// The query string a filter control asked for and the server has not answered yet.
//
// useSearchParams only moves once the RSC payload for a router.replace comes back, which
// is instant on localhost and takes a second or two in production. Every control on a
// screen merges its own keys into "the current URL", so with useSearchParams as the base
// a second change made inside that window was built on the old URL and silently erased
// the first - pick a programme, start typing a search, and the programme filter was gone.
// This store is the one place that knows what the URL is about to be.
//
// Module-level on purpose: the controls on a screen are siblings with no common client
// parent to hold a context, and there is only ever one address bar.
interface InFlight {
  pathname: string
  // The URL this chain of writes started from, then every query written since. While the
  // landed URL is any of these, the chain is still settling; anything else means the
  // operator navigated some other way (back/forward, a link) and the chain is abandoned.
  known: string[]
  latest: string
}

let inFlight: InFlight | null = null
const listeners = new Set<() => void>()

function notify() {
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function settle(pathname: string, landed: string) {
  if (!inFlight) return
  const stillSettling =
    inFlight.pathname === pathname &&
    landed !== inFlight.latest &&
    inFlight.known.includes(landed)
  if (stillSettling) return
  inFlight = null
  notify()
}

// Whether any filter change is still travelling. Also settles the chain against the URL
// that landed, which is what keeps it from staying pending forever when the operator
// leaves for a screen with no filter control on it to do that.
export function useIsQueryNavigationPending(): boolean {
  const pathname = usePathname()
  const landed = useSearchParams().toString()
  useEffect(() => {
    settle(pathname, landed)
  }, [pathname, landed])

  return useSyncExternalStore(
    subscribe,
    () => inFlight !== null,
    () => false,
  )
}

// The URL a screen's filter controls should read from and write to. `params` is the query
// the operator last asked for - the landed one when nothing is travelling - so a control
// renders its new value immediately instead of snapping back until the server answers.
export function useUrlQuery() {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const landed = searchParams.toString()

  const pending = useSyncExternalStore(
    subscribe,
    () => (inFlight?.pathname === pathname ? inFlight.latest : null),
    () => null,
  )

  useEffect(() => {
    settle(pathname, landed)
  }, [pathname, landed])

  const current = pending ?? landed
  const params = useMemo(() => new URLSearchParams(current), [current])

  // Replaces the whole query. `push` adds a history entry instead, for the one screen
  // whose filters are meant to be walked back with the browser's back button.
  const replace = useCallback(
    (next: URLSearchParams, { push = false }: { push?: boolean } = {}) => {
      const query = next.toString()
      if (query === landed) {
        // Back to where the address bar already is: nothing left to wait for.
        inFlight = null
      } else if (inFlight?.pathname === pathname) {
        inFlight = { ...inFlight, known: [...inFlight.known, query], latest: query }
      } else {
        inFlight = { pathname, known: [landed, query], latest: query }
      }
      notify()
      const href = query ? `${pathname}?${query}` : pathname
      if (push) router.push(href)
      else router.replace(href)
    },
    [landed, pathname, router],
  )

  // Merges into the freshest query - the one the last write asked for, not the one on
  // screen - and writes it back.
  const update = useCallback(
    (mutate: (params: URLSearchParams) => void, options?: { push?: boolean }) => {
      const base =
        inFlight?.pathname === pathname ? inFlight.latest : landed
      const next = new URLSearchParams(base)
      mutate(next)
      replace(next, options)
    },
    [landed, pathname, replace],
  )

  return { params, isPending: pending !== null, replace, update }
}

// Local mirror of a value the server resolved from the URL, for controls that need to
// show a pick instantly. It re-seeds whenever the server hands down something different
// - "Limpar filtros", back/forward, a default the server filled in - but only once no
// write is travelling, so an intermediate answer never overwrites a newer pick. It also
// re-seeds every time a chain of writes settles, even onto an unchanged value: "Limpar
// filtros" on a screen that was already bare lands on the same answer, and the pick made
// just before it must still go.
//
// Without the re-seed the mirror was only ever read on mount: after "Limpar filtros" the
// controls kept showing the old picks, and the next change wrote them all back.
export function useServerSeededState<T>(serverValue: T, isPending: boolean) {
  const [state, setState] = useState(serverValue)
  const seed = JSON.stringify(serverValue)
  const [seenSeed, setSeenSeed] = useState(seed)
  const [wasPending, setWasPending] = useState(isPending)
  if (isPending !== wasPending) setWasPending(isPending)
  if (!isPending && (seed !== seenSeed || wasPending)) {
    setSeenSeed(seed)
    setState(serverValue)
  }
  return [state, setState] as const
}
