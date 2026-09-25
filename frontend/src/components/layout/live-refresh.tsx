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
  // Whether an event arrived while this tab was in the background. The refresh is still
  // attempted there - it often lands - but Chrome throttles a hidden tab's timers and
  // React holds its rendering back, so it cannot be relied on: without the flag the
  // operator would come back to a screen that looks current and is not, which is the
  // exact failure this component exists to prevent.
  const missedWhileHiddenRef = useRef(false)

  // Coalesces a burst - issuances registered back to back - into a single refetch.
  const scheduleRefresh = useCallback(() => {
    if (document.hidden) missedWhileHiddenRef.current = true

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
