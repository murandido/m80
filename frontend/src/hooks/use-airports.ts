"use client"

import { useEffect, useState } from "react"
import { getAirportsSchema, type Airport } from "@/schemas/airport"

// The airport table no longer rides along in the page payload - see
// app/api/airports/route.ts for why - so the grid asks for it once and every consumer
// in the tab shares that one answer. Module scope rather than a context because the
// list is immutable reference data: there is nothing to invalidate and no provider to
// thread through.
let cached: Airport[] | null = null
let inflight: Promise<Airport[]> | null = null

function loadAirports(): Promise<Airport[]> {
  if (cached) return Promise.resolve(cached)

  // A failed load is deliberately not cached: only `cached` short-circuits, so the next
  // mount tries again. The cell still commits whatever IATA code is typed, so an empty
  // dropdown costs the operator convenience, not the ability to work.
  inflight ??= fetch("/api/airports")
    .then(async (res) => {
      const parsed = getAirportsSchema.response.safeParse(await res.json())
      if (!res.ok || !parsed.success) return []
      cached = parsed.data.data ?? []
      return cached
    })
    .catch(() => [])
    .finally(() => {
      inflight = null
    })

  return inflight
}

export function useAirports(): Airport[] {
  const [airports, setAirports] = useState<Airport[]>(cached ?? [])

  useEffect(() => {
    if (cached) return

    let active = true
    void loadAirports().then((list) => {
      if (active) setAirports(list)
    })
    return () => {
      active = false
    }
  }, [])

  return airports
}
