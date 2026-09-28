"use client"

import { useServerSeededState, useUrlQuery } from "@/hooks/use-url-query"
import { CalendarDays, Info } from "lucide-react"
import { Button } from "@/components/ui/button"
import { DateInput } from "@/components/ui/date-input"
import { Label } from "@/components/ui/label"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
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

interface PeriodFilterProps {
  // The range the server actually applied, so the label never drifts from the data.
  dateFrom: string
  dateTo: string
  // Preset and reference day are carried in the URL too: without them a "Mensal" range
  // would be indistinguishable from a hand-picked one after a reload.
  preset: PeriodPreset
  reference: string
}

// YYYY-MM-DD sorts lexicographically, so comparing the strings is comparing the days -
// no parsing, and no timezone to get wrong.
function min(a: string, b: string): string {
  return a && a < b ? a : b
}

function max(a: string, b: string): string {
  return a && a > b ? a : b
}

// Shared by every period-scoped report (Vendas and the Dashboard). It only ever writes
// period/reference/date_from/date_to, merging into whatever else is already in the URL, so
// each page can own its own filter keys alongside it.
export function PeriodFilter({
  dateFrom,
  dateTo,
  preset,
  reference,
}: PeriodFilterProps) {
  const { isPending, update } = useUrlQuery()
  const [pending, setPending] = useServerSeededState<{
    preset: PeriodPreset
    reference: string
    date_from: string
    date_to: string
  }>({ preset, reference, date_from: dateFrom, date_to: dateTo }, isPending)

  function push(
    nextPreset: PeriodPreset,
    nextReference: string,
    range: { date_from: string; date_to: string },
  ) {
    setPending({ preset: nextPreset, reference: nextReference, ...range })

    // Merged into the existing URL so changing the period keeps whatever the filter bar
    // (which owns the other keys on this page) has already selected.
    update((params) => {
      params.set("period", nextPreset)
      params.set("reference", nextReference)
      params.set("date_from", range.date_from)
      params.set("date_to", range.date_to)
    })
  }

  function apply(nextPreset: PeriodPreset, nextReference: string) {
    const referenceDate = parseYmd(nextReference) ?? startOfToday()

    // Switching into "Personalizado" keeps the range currently on screen as the starting
    // point, so the two ends are already filled in and only one of them usually moves.
    if (nextPreset === "custom") {
      push(nextPreset, formatYmd(referenceDate), {
        date_from: pending.date_from,
        date_to: pending.date_to,
      })
      return
    }

    push(
      nextPreset,
      formatYmd(referenceDate),
      resolvePeriodRange(nextPreset, referenceDate),
    )
  }

  // Either end of a custom range. The other end follows when it would be crossed, because
  // an inverted range filters nothing at all and reads as a screen with no issuances.
  function applyCustomEnd(end: "date_from" | "date_to", value: string) {
    if (!value) return
    const from = end === "date_from" ? value : min(pending.date_from, value)
    const to = end === "date_to" ? value : max(pending.date_to, value)
    push("custom", pending.reference, { date_from: from, date_to: to })
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

      {pending.preset === "custom" ? (
        <div className="flex flex-wrap items-end gap-3">
          <div className="flex flex-col gap-1">
            <Label
              className="text-xs text-slate-500"
              htmlFor="period-date-from"
            >
              De
            </Label>
            <DateInput
              id="period-date-from"
              value={pending.date_from}
              max={pending.date_to || undefined}
              onChange={(value) => applyCustomEnd("date_from", value)}
              className="w-44 [&_input]:bg-slate-100"
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label className="text-xs text-slate-500" htmlFor="period-date-to">
              Até
            </Label>
            <DateInput
              id="period-date-to"
              value={pending.date_to}
              min={pending.date_from || undefined}
              onChange={(value) => applyCustomEnd("date_to", value)}
              className="w-44 [&_input]:bg-slate-100"
            />
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-1">
          <Label className="flex items-center gap-1 text-xs text-slate-500">
            Data de referência
            <Tooltip>
              <TooltipTrigger asChild>
                <Info className="h-3 w-3 cursor-help" />
              </TooltipTrigger>
              <TooltipContent className="max-w-64">
                O período é calculado em volta desta data. Com
                &quot;Mensal&quot;, por exemplo, escolher 10/03 mostra março
                inteiro — é assim que se olha um mês passado sem digitar as duas
                pontas.
              </TooltipContent>
            </Tooltip>
          </Label>
          <DateInput
            value={pending.reference}
            onChange={(value) => apply(pending.preset, value)}
            className="w-44 [&_input]:bg-slate-100"
          />
        </div>
      )}

      <div className="flex items-center gap-2 pb-2 text-sm text-slate-500">
        <CalendarDays className="h-4 w-4 shrink-0" />
        <span>{dateRangeLabel(dateFrom, dateTo)}</span>
      </div>
    </div>
  )
}
