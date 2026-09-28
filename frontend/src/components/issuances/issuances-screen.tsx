"use client"

import { createContext, useContext, useMemo, type ReactNode } from "react"
import { AlertTriangle } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import type { GridReferenceData } from "@/components/issuances/grid-context"
import { IssuancesGrid } from "@/components/issuances/issuances-grid"
import { IssuancesFilterBar } from "@/components/issuances/issuances-filter-bar"
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
    <IssuancesReferenceContext.Provider value={value}>{children}</IssuancesReferenceContext.Provider>
  )
}

interface IssuancesScreenProps {
  initialIssuances: Issuance[]
  filterQuery: string
  initialFilters: Parameters<typeof IssuancesFilterBar>[0]["initialFilters"]
  hasSearch: boolean
  currentUserId: string | null
}

export function IssuancesScreen({
  initialIssuances,
  filterQuery,
  initialFilters,
  hasSearch,
  currentUserId,
}: IssuancesScreenProps) {
  const reference = useContext(IssuancesReferenceContext)
  if (!reference) {
    throw new Error("IssuancesScreen must be rendered inside IssuancesReferenceProvider")
  }
  const { data, failed } = reference

  return (
    <>
      {failed && (
        <Alert>
          <AlertTriangle />
          <AlertTitle>Algumas listas não carregaram</AlertTitle>
          <AlertDescription>
            Companhias, contas, agências ou emissores podem aparecer incompletos nos campos de
            seleção. Recarregue a página para tentar de novo.
          </AlertDescription>
        </Alert>
      )}

      <IssuancesGrid
        filters={
          <IssuancesFilterBar
            loyaltyPrograms={data.loyaltyPrograms}
            airlines={data.airlines}
            accounts={data.accounts}
            people={data.people}
            agencies={data.agencies}
            users={data.users}
            initialFilters={initialFilters}
            hasSearch={hasSearch}
          />
        }
        initialIssuances={initialIssuances}
        filterQuery={filterQuery}
        currentUserId={currentUserId}
        {...data}
      />
    </>
  )
}
