"use client"

import { useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { CalendarDays, Info } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import {
  PERIOD_LABELS,
  PERIOD_PRESETS,
  type PeriodPreset,
  dateRangeLabel,
  formatYmd,
  parseYmd,
  resolvePeriodRange,
  startOfToday,
} from "@/lib/date-range"

interface SalesPeriodFilterProps {
  // The range the server actually applied, so the label never drifts from the data.
  dateFrom: string
  dateTo: string
  // Preset and reference day are carried in the URL too: without them a "Mensal" range
  // would be indistinguishable from a hand-picked one after a reload.
  preset: PeriodPreset
  reference: string
}

export function SalesPeriodFilter({
  dateFrom,
  dateTo,
  preset,
  reference,
}: SalesPeriodFilterProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const [pending, setPending] = useState<{ preset: PeriodPreset; reference: string }>({
    preset,
    reference,
  })

  function apply(nextPreset: PeriodPreset, nextReference: string) {
    setPending({ preset: nextPreset, reference: nextReference })

    const referenceDate = parseYmd(nextReference) ?? startOfToday()
    const range = resolvePeriodRange(nextPreset, referenceDate)

    // Merged into the existing URL so changing the period keeps whatever the filter bar
    // (which owns the other keys on this page) has already selected.
    const params = new URLSearchParams(searchParams.toString())
    params.set("period", nextPreset)
    params.set("reference", formatYmd(referenceDate))
    params.set("date_from", range.date_from)
    params.set("date_to", range.date_to)
    router.replace(`${pathname}?${params.toString()}`)
  }

  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="flex flex-col gap-1">
        <Label className="text-xs text-slate-500">Período</Label>
        <div className="flex gap-1">
          {PERIOD_PRESETS.map((option) => (
            <Button
              key={option}
              type="button"
              size="sm"
              variant="outline"
              onClick={() => apply(option, pending.reference)}
              className={cn(
                "font-normal",
                pending.preset === option
                  ? "bg-slate-800 text-slate-50 hover:bg-slate-900 hover:text-slate-50"
                  : "bg-slate-100 text-slate-600 hover:bg-slate-200",
              )}
            >
              {PERIOD_LABELS[option]}
            </Button>
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-1">
        <Label className="flex items-center gap-1 text-xs text-slate-500">
          Data de referência
          <Tooltip>
            <TooltipTrigger asChild>
              <Info className="h-3 w-3 cursor-help" />
            </TooltipTrigger>
            <TooltipContent className="max-w-64">
              O período é calculado em volta desta data. Com &quot;Mensal&quot;, por
              exemplo, escolher 10/03 mostra março inteiro — é assim que se olha um mês
              passado sem digitar as duas pontas.
            </TooltipContent>
          </Tooltip>
        </Label>
        <Input
          type="date"
          value={pending.reference}
          onChange={(e) => apply(pending.preset, e.target.value)}
          className="w-44 bg-slate-100"
        />
      </div>

      <div className="flex items-center gap-2 pb-2 text-sm text-slate-500">
        <CalendarDays className="h-4 w-4 shrink-0" />
        <span>{dateRangeLabel(dateFrom, dateTo)}</span>
      </div>
    </div>
  )
}
