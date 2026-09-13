"use client"

import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { Info } from "lucide-react"
import { DateRangeFilter } from "@/components/filters/date-range-filter"
import { Label } from "@/components/ui/label"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

interface SalesPeriodFilterProps {
  // The range the server actually applied, so the field never drifts from the data.
  dateFrom: string
  dateTo: string
}

// The clients view's period, on the shared date picker. It writes the same URL keys the
// preset-based PeriodFilter does, as a "custom" period, so the server resolves it through
// the same resolvePeriod and the sticky-filter cookie remembers it like any other.
export function SalesPeriodFilter({ dateFrom, dateTo }: SalesPeriodFilterProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  function apply(range: { from: string; to: string }) {
    const params = new URLSearchParams(searchParams.toString())
    params.set("period", "custom")
    params.set("reference", range.from)
    params.set("date_from", range.from)
    params.set("date_to", range.to)
    router.replace(`${pathname}?${params.toString()}`)
  }

  return (
    <div className="flex flex-col gap-1">
      <Label className="flex items-center gap-1 text-xs text-slate-500">
        Período
        <Tooltip>
          <TooltipTrigger asChild>
            <Info className="h-3 w-3 cursor-help" />
          </TooltipTrigger>
          <TooltipContent className="max-w-64">
            Mostra as vendas das emissões feitas nesses dias.
          </TooltipContent>
        </Tooltip>
      </Label>
      <DateRangeFilter from={dateFrom} to={dateTo} onChange={apply} />
    </div>
  )
}
