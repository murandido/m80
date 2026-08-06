"use client"

import { Suspense } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { RotateCcw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { ListSearchBar } from "@/components/shared/list-search-bar"
import { ListSortSelect } from "@/components/shared/list-sort-select"
import { useStickyFilters } from "@/hooks/use-sticky-filters"
import { clearStickyFilters } from "@/lib/sticky-filters"
import type { ListOrder } from "@/lib/list-params"

interface ListFilterBarProps {
  searchPlaceholder: string
  /** Omit on a listing whose backend does not accept `order` — a select that
      silently does nothing is worse than no select. Airports is the one case. */
  order?: ListOrder
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
    // Forget first, then navigate. The other way round, useStickyFilters would see
    // the bare URL and immediately restore the cookie that was just dropped.
    clearStickyFilters(pathname)
    router.replace(pathname)
  }

  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="min-w-64 flex-1">
        <ListSearchBar placeholder={searchPlaceholder} />
      </div>
      {children}
      {order && <ListSortSelect order={order} />}
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
