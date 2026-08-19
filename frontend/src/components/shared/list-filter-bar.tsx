"use client"

import { Suspense } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { RotateCcw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { ListSearchBar } from "@/components/shared/list-search-bar"
import { ListSortSelect } from "@/components/shared/list-sort-select"
import { useStickyFilters } from "@/hooks/use-sticky-filters"
import { clearStickyFilters } from "@/lib/sticky-filters"

interface ListFilterBarProps {
  searchPlaceholder: string
  /** Omit on a listing whose backend does not accept `order` — a select that
      silently does nothing is worse than no select. */
  order?: string
  /** Override the sort options for a listing that does not use the shared four. */
  orderOptions?: readonly { value: string; label: string }[]
  /** The screen's own filter controls, if it has a dimension worth filtering on. */
  children?: React.ReactNode
}

// Every control in the bar reads useSearchParams, which Next requires to sit under
// a Suspense boundary. Wrapping here rather than in each page means no screen can
// forget it - and forgetting it is not a visible failure, it is a hydration
// mismatch that only shows up in the console.
export function ListFilterBar(props: ListFilterBarProps) {
  return (
    <Suspense fallback={<div className="h-16" />}>
      <FilterBar {...props} />
    </Suspense>
  )
}

function FilterBar({
  searchPlaceholder,
  order,
  orderOptions,
  children,
}: ListFilterBarProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  // One call per screen, here rather than in each page: this is what writes the
  // per-route cookie that brings the filters back when the operator returns.
  useStickyFilters()

  const hasAnyFilter = searchParams.toString() !== ""

  function clearAll() {
    // useStickyFilters also drops the cookie when the URL goes bare on a screen the
    // operator is already on, so this is belt and braces - but it happens now
    // rather than on the next effect pass, and it says out loud that "Limpar
    // filtros" is meant to be forgotten, not just navigated away from.
    clearStickyFilters(pathname)
    router.replace(pathname)
  }

  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="min-w-64 flex-1">
        <ListSearchBar placeholder={searchPlaceholder} />
      </div>
      {children}
      {order && <ListSortSelect order={order} options={orderOptions} />}
      {hasAnyFilter && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={clearAll}
          className="mb-0.5 text-muted-foreground"
        >
          <RotateCcw className="h-4 w-4" />
          Limpar filtros
        </Button>
      )}
    </div>
  )
}
