"use client"

import { useCallback, useEffect, useRef } from "react"
import { useRouter } from "next/navigation"
import { useSSE } from "@/hooks/use-sse"

// Keeps both sales views current without a reload. It renders nothing: the tables are fed
// by Server Components, so refreshing the route is what brings fresh rows - and it does so
// with the filters already in the URL, instead of merging event payloads in the browser
// (the filter rules live in the Go handlers and must not be duplicated here).
//
// Every issuance event matters: a created issuance is a new sale, a cancelled one reopens
// its sale and refunds miles to the account, a deleted one takes its sale with it, and an
// edited one changes the amount owed. sale.updated covers what only this screen can do -
// linking a client and marking a sale as paid.
const REFRESH_DEBOUNCE_MS = 300

export function SalesLiveRefresh() {
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

  useSSE("/api/events", {
    "issuance.created": scheduleRefresh,
    "issuance.updated": scheduleRefresh,
    "issuance.cancelled": scheduleRefresh,
    "issuance.deleted": scheduleRefresh,
    "sale.updated": scheduleRefresh,
  })

  return null
}
