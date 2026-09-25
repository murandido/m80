"use client"

import { useUrlQuery } from "@/hooks/use-url-query"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { Button } from "@/components/ui/button"

interface ListPaginationProps {
  total: number
  page: number
  limit: number
  /** Singular and plural of what is being counted, e.g. ["aeroporto", "aeroportos"]. */
  noun: [string, string]
}

export function ListPagination({
  total,
  page: landedPage,
  limit,
  noun,
}: ListPaginationProps) {
  const { params: urlParams, isPending, update } = useUrlQuery()
  // Clicking "next" twice before the first page arrives has to mean page 3, not page 2
  // twice - so the page shown and counted from is the one already asked for.
  const page = isPending ? Math.max(1, Number(urlParams.get("page")) || 1) : landedPage

  const totalPages = Math.max(1, Math.ceil(total / limit))
  const hasPrev = page > 1
  const hasNext = page < totalPages

  function goToPage(target: number) {
    update((params) => {
      if (target <= 1) {
        // Keeps the URL canonical: page 1 is the bare path, so a bookmark of the
        // first page is the same string as a fresh arrival.
        params.delete("page")
      } else {
        params.set("page", String(target))
      }
    })
  }

  return (
    <div className="flex items-center justify-between">
      {/* aria-live: paging replaces the whole list, and without an announcement a
          screen reader user gets no signal that anything happened. */}
      <span role="status" aria-live="polite" className="text-xs text-muted-foreground">
        {total} {total === 1 ? noun[0] : noun[1]} · página {page} de {totalPages}
      </span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="icon"
          disabled={!hasPrev}
          onClick={() => goToPage(page - 1)}
          aria-label="Página anterior"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <Button
          variant="outline"
          size="icon"
          disabled={!hasNext}
          onClick={() => goToPage(page + 1)}
          aria-label="Próxima página"
        >
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
