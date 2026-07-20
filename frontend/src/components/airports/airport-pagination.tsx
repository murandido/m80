"use client"

import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { Button } from "@/components/ui/button"

interface AirportPaginationProps {
  total: number
  page: number
  limit: number
}

export function AirportPagination({ total, page, limit }: AirportPaginationProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  const totalPages = Math.max(1, Math.ceil(total / limit))
  const hasPrev = page > 1
  const hasNext = page < totalPages

  function goToPage(target: number) {
    const params = new URLSearchParams(searchParams.toString())
    if (target <= 1) {
      params.delete("page")
    } else {
      params.set("page", String(target))
    }
    router.replace(`${pathname}?${params.toString()}`)
  }

  return (
    <div className="flex items-center justify-between">
      <span className="text-xs text-slate-500">
        {total} {total === 1 ? "aeroporto" : "aeroportos"} · página {page} de{" "}
        {totalPages}
      </span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="icon"
          disabled={!hasPrev}
          onClick={() => goToPage(page - 1)}
          className="border-slate-800 text-slate-800 disabled:opacity-40"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <Button
          variant="outline"
          size="icon"
          disabled={!hasNext}
          onClick={() => goToPage(page + 1)}
          className="border-slate-800 text-slate-800 disabled:opacity-40"
        >
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
