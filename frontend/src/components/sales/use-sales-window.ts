"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { useSSE } from "@/hooks/use-sse"
import { getSalesPageSchema, type Sale } from "@/schemas/sale"

// How many sales one request brings. Pages are aligned on multiples of this from the oldest
// sale, so a page is always the same rows.
export const SALES_PAGE_SIZE = 100

// How many pages are held at once. The scroll covers only these - like a chat, the bar is
// the stretch in memory, not the whole history - and reaching either end of it brings the
// next page in and lets the one at the far end go.
const MAX_PAGES = 4

const REFRESH_DEBOUNCE_MS = 300

interface WindowState {
  // How many sales the filter matches. Null until the first answer arrives.
  total: number | null
  // What the one filtered client owes over the whole list; zero without that filter.
  owedCents: number
  // The loaded stretch: consecutive pages starting at firstPage.
  firstPage: number
  pages: Sale[][]
  // When the page at the top arrived, so the rows that came in above can fade in.
  arrivedAt: number
  error: boolean
}

const EMPTY: WindowState = {
  total: null,
  owedCents: 0,
  firstPage: 0,
  pages: [],
  arrivedAt: 0,
  error: false,
}

async function fetchWindow(query: string, offset: number, limit: number) {
  const params = new URLSearchParams(query)
  params.set("offset", String(offset))
  params.set("limit", String(limit))
  const res = await fetch(`/api/sales/page?${params.toString()}`, { cache: "no-store" })
  const parsed = getSalesPageSchema.response.safeParse(await res.json())
  if (!res.ok || !parsed.success) throw new Error("sales page")
  return parsed.data.data
}

function fetchPage(query: string, page: number) {
  return fetchWindow(query, page * SALES_PAGE_SIZE, SALES_PAGE_SIZE)
}

// The payments list the way a chat history works: it opens on the newest sales, scrolling
// up brings older pages in above, and only a few pages are ever held - the far end is let
// go as the other end grows. The filter rules stay on the server; this only asks for pages.
export function useSalesWindow(query: string) {
  const [state, setState] = useState<WindowState>(EMPTY)
  const stateRef = useRef(state)
  useEffect(() => {
    stateRef.current = state
  }, [state])
  const busy = useRef(false)
  const generation = useRef(0)
  // Sales that must not be let go - a row being edited.
  const protectedIds = useRef(new Set<string>())

  // A new filter is a new list: start over, during render, so the old rows are never
  // painted under the new filter.
  const [stateQuery, setStateQuery] = useState(query)
  if (stateQuery !== query) {
    setStateQuery(query)
    setState(EMPTY)
  }

  // Open on the newest page. When it is short, the one before it comes too, so the screen
  // is never a handful of rows with the rest of the history one scroll away.
  useEffect(() => {
    const current = ++generation.current
    busy.current = true
    ;(async () => {
      const head = await fetchWindow(query, 0, 1)
      if (head.total === 0) return { head, firstPage: 0, pages: [] as Sale[][] }
      const lastPage = Math.floor((head.total - 1) / SALES_PAGE_SIZE)
      const last = await fetchPage(query, lastPage)
      if (lastPage > 0 && last.sales.length < SALES_PAGE_SIZE / 2) {
        const before = await fetchPage(query, lastPage - 1)
        return { head: last, firstPage: lastPage - 1, pages: [before.sales, last.sales] }
      }
      return { head: last, firstPage: lastPage, pages: [last.sales] }
    })()
      .then(({ head, firstPage, pages }) => {
        if (generation.current !== current) return
        setState({
          total: head.total,
          owedCents: head.owed_cents,
          firstPage,
          pages,
          arrivedAt: 0,
          error: false,
        })
      })
      .catch(() => {
        if (generation.current !== current) return
        setState((prev) => ({ ...prev, error: true }))
      })
      .finally(() => {
        if (generation.current === current) busy.current = false
      })
  }, [query])

  // One page older, above the stretch; the newest page goes if the stretch is full.
  const loadOlder = useCallback(() => {
    const { firstPage, pages, total } = stateRef.current
    if (busy.current || total === null || firstPage === 0 || pages.length === 0) return
    busy.current = true
    const current = generation.current
    fetchPage(query, firstPage - 1)
      .then((data) => {
        if (generation.current !== current) return
        setState((prev) => {
          const next = [data.sales, ...prev.pages]
          const last = next[next.length - 1]
          if (next.length > MAX_PAGES && !last.some((s) => protectedIds.current.has(s.id))) {
            next.pop()
          }
          return {
            ...prev,
            pages: next,
            firstPage: prev.firstPage - 1,
            total: data.total,
            owedCents: data.owed_cents,
            arrivedAt: Date.now(),
          }
        })
      })
      .catch(() => {})
      .finally(() => {
        busy.current = false
      })
  }, [query])

  // One page newer, below the stretch; the oldest page goes if the stretch is full.
  const loadNewer = useCallback(() => {
    const { firstPage, pages, total } = stateRef.current
    if (busy.current || total === null || pages.length === 0) return
    const nextPage = firstPage + pages.length
    if (nextPage * SALES_PAGE_SIZE >= total) return
    busy.current = true
    const current = generation.current
    fetchPage(query, nextPage)
      .then((data) => {
        if (generation.current !== current) return
        setState((prev) => {
          const next = [...prev.pages, data.sales]
          let first = prev.firstPage
          if (next.length > MAX_PAGES && !next[0].some((s) => protectedIds.current.has(s.id))) {
            next.shift()
            first += 1
          }
          return { ...prev, pages: next, firstPage: first, total: data.total, owedCents: data.owed_cents }
        })
      })
      .catch(() => {})
      .finally(() => {
        busy.current = false
      })
  }, [query])

  // Refetches the pages held, and the length - after an edit (a payment can move money
  // between a client's sales) and on every server event.
  const refresh = useCallback(() => {
    const { firstPage, pages } = stateRef.current
    if (pages.length === 0) return
    const current = generation.current
    Promise.all(pages.map((_, i) => fetchPage(query, firstPage + i)))
      .then((fresh) => {
        if (generation.current !== current) return
        setState((prev) =>
          prev.firstPage !== firstPage || prev.pages.length !== fresh.length
            ? prev
            : {
                ...prev,
                pages: fresh.map((page) => page.sales),
                total: fresh[fresh.length - 1].total,
                owedCents: fresh[fresh.length - 1].owed_cents,
              },
        )
      })
      .catch(() => {})
  }, [query])

  // Writes a sale's new values into the page that holds it - the optimistic half of an
  // edit, and the server's answer to it.
  const patchSale = useCallback((saleId: string, patch: Partial<Sale>) => {
    setState((prev) => {
      let changed = false
      const pages = prev.pages.map((sales) => {
        const at = sales.findIndex((sale) => sale.id === saleId)
        if (at === -1) return sales
        changed = true
        const next = [...sales]
        next[at] = { ...next[at], ...patch }
        return next
      })
      return changed ? { ...prev, pages } : prev
    })
  }, [])

  const protect = useCallback((saleIds: string[]) => {
    protectedIds.current = new Set(saleIds)
  }, [])

  // Live updates. A hidden tab only remembers that it missed something and refreshes once
  // when it is looked at, the same budget rule LiveRefresh follows: the API rate-limits
  // per user, across every tab.
  const timeout = useRef<ReturnType<typeof setTimeout> | null>(null)
  const missed = useRef(false)
  const scheduleRefresh = useCallback(() => {
    if (document.hidden) {
      missed.current = true
      return
    }
    if (timeout.current) clearTimeout(timeout.current)
    timeout.current = setTimeout(refresh, REFRESH_DEBOUNCE_MS)
  }, [refresh])

  useEffect(() => {
    function onVisible() {
      if (!document.hidden && missed.current) {
        missed.current = false
        refresh()
      }
    }
    document.addEventListener("visibilitychange", onVisible)
    return () => {
      document.removeEventListener("visibilitychange", onVisible)
      if (timeout.current) clearTimeout(timeout.current)
    }
  }, [refresh])

  useSSE(
    "/api/events",
    {
      "sale.updated": scheduleRefresh,
      "issuance.created": scheduleRefresh,
      "issuance.updated": scheduleRefresh,
      "issuance.cancelled": scheduleRefresh,
      "issuance.deleted": scheduleRefresh,
    },
    { onReconnect: scheduleRefresh },
  )

  const sales = state.pages.flat()
  const firstIndex = state.firstPage * SALES_PAGE_SIZE
  const total = state.total
  return {
    total,
    owedCents: state.owedCents,
    error: state.error,
    sales,
    // The position of sales[0] in the whole list.
    firstIndex,
    // When the page on top arrived, for the fade of rows that came in above.
    arrivedAt: state.arrivedAt,
    hasOlder: state.firstPage > 0,
    hasNewer: total !== null && firstIndex + sales.length < total,
    loadOlder,
    loadNewer,
    refresh,
    patchSale,
    protect,
  }
}
