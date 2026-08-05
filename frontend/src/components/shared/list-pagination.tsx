"use client"

import { usePathname, useRouter, useSearchParams } from "next/navigation"
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
  page,
  limit,
  noun,
}: ListPaginationProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  const totalPages = Math.max(1, Math.ceil(total / limit))
  const hasPrev = page > 1
  const hasNext = page < totalPages

  function goToPage(target: number) {
    const params = new URLSearchParams(searchParams.toString())
    if (target <= 1) {
      // Keeps the URL canonical: page 1 is the bare path, so a bookmark of the
      // first page is the same string as a fresh arrival.
      params.delete("page")
    } else {
      params.set("page", String(target))
    }
    router.replace(`${pathname}?${params.toString()}`)
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
