"use client"

import { createContext, useContext, useMemo, type ReactNode } from "react"
import type { GridReferenceData } from "@/components/issuances/grid-context"
import { IssuanceBoardScreen } from "@/components/issuances/board/issuance-board-screen"
import type { FilterState } from "@/components/issuances/board/board-toolbar"
import type { Issuance } from "@/schemas/issuance"

// The pickers' reference lists, loaded by the issuances layout rather than by the page.
//
// A layout is not re-rendered when only the search params change, so a filter change now
// costs the server one query - the issuances - instead of seven, and the answer no longer
// carries every account and person again just to rebuild the comboboxes. Those lists do
// not depend on the filters; they used to travel with every change anyway.
interface IssuancesReference {
  data: GridReferenceData
  // Some list failed to load; the screen says so rather than show a short picker silently.
  failed: boolean
}

const IssuancesReferenceContext = createContext<IssuancesReference | null>(null)

export function IssuancesReferenceProvider({
  data,
  failed,
  children,
}: IssuancesReference & { children: ReactNode }) {
  // The layout hands down the same arrays until it re-renders, so this stays referentially
  // stable across filter changes - which the grid's memoized cells rely on.
  const value = useMemo(() => ({ data, failed }), [data, failed])
  return (
    <IssuancesReferenceContext.Provider value={value}>
      {children}
    </IssuancesReferenceContext.Provider>
  )
}

interface IssuancesScreenProps {
  initialIssuances: Issuance[]
  filterQuery: string
  initialFilters: FilterState
  hasSearch: boolean
  currentUserId: string | null
}

// The issuances are a board of cards with a side panel (see components/issuances/board/).
// The spreadsheet's files (issuances-grid.tsx, cells/, the filter bar) are no longer
// rendered; they stay until the board has settled in production, then go in a cleanup.
export function IssuancesScreen({
  initialIssuances,
  filterQuery,
  initialFilters,
  hasSearch,
  currentUserId,
}: IssuancesScreenProps) {
  const reference = useContext(IssuancesReferenceContext)
  if (!reference) {
    throw new Error(
      "IssuancesScreen must be rendered inside IssuancesReferenceProvider",
    )
  }

  return (
    <IssuanceBoardScreen
      initialIssuances={initialIssuances}
      filterQuery={filterQuery}
      initialFilters={initialFilters}
      hasSearch={hasSearch}
      currentUserId={currentUserId}
      referenceData={reference.data}
      referenceListsFailed={reference.failed}
    />
  )
}
