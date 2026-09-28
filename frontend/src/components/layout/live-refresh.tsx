"use client"

import { useCallback, useEffect, useRef } from "react"
import { useRouter } from "next/navigation"
import { useSSE } from "@/hooks/use-sse"

// Keeps a period-scoped report current without a reload - the sales views and the
// dashboard. It renders nothing: those screens are fed by Server Components, so refreshing
// the route is what brings fresh numbers, and it does so with the filters already in the
// URL instead of merging event payloads in the browser (the filter rules live in the Go
// handlers and must not be duplicated here).
//
// It subscribes to every event rather than taking a list: each one moves at least one of
// these screens. A created issuance is a new sale and new volume, a cancelled one reopens
// its sale and refunds miles, a deleted one takes its sale with it, an edited one changes
// the amount owed, and sale.updated carries linking a client or marking a sale as paid.
const REFRESH_DEBOUNCE_MS = 300

export function LiveRefresh() {
  const router = useRouter()
  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Whether an event arrived while this tab was in the background. A hidden tab does not
  // refresh at all: it only remembers, and catches up once when it is looked at again.
  //
  // It used to refresh anyway. Each refresh re-renders the whole route - the report plus
  // every picker list, around nine calls to the API - and the API rate-limits per user,
  // across all of that user's tabs. A Vendas tab left open behind the grid spent the
  // operator's budget on every issuance a colleague registered, and the 429s surfaced
  // on the screen they were actually using. Nobody reads a hidden tab; one refresh on
  // return gives them the same screen for a fraction of the calls.
  const missedWhileHiddenRef = useRef(false)

  // Coalesces a burst - issuances registered back to back - into a single refetch.
  const scheduleRefresh = useCallback(() => {
    if (document.hidden) {
      missedWhileHiddenRef.current = true
      return
    }

    if (refreshTimeoutRef.current) clearTimeout(refreshTimeoutRef.current)
    refreshTimeoutRef.current = setTimeout(() => router.refresh(), REFRESH_DEBOUNCE_MS)
  }, [router])

  useEffect(() => {
    function catchUpOnReturn() {
      if (document.hidden || !missedWhileHiddenRef.current) return
      missedWhileHiddenRef.current = false
      router.refresh()
    }

    document.addEventListener("visibilitychange", catchUpOnReturn)
    return () => document.removeEventListener("visibilitychange", catchUpOnReturn)
  }, [router])

  useEffect(
    () => () => {
      if (refreshTimeoutRef.current) clearTimeout(refreshTimeoutRef.current)
    },
    [],
  )

  useSSE(
    "/api/events",
    {
      "issuance.created": scheduleRefresh,
      "issuance.updated": scheduleRefresh,
      "issuance.cancelled": scheduleRefresh,
      "issuance.deleted": scheduleRefresh,
      "sale.updated": scheduleRefresh,
    },
    { onReconnect: scheduleRefresh },
  )

  return null
}
