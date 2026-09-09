"use client"

import { useRef, useState } from "react"
import type { DateRange } from "react-day-picker"
import { CalendarDays } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { addDays, dateRangeLabel, formatYmd, parseYmd, startOfToday } from "@/lib/date-range"
import { cn } from "@/lib/utils"

interface DateRangeFilterProps {
  from: string
  to: string
  onChange: (range: { from: string; to: string }) => void
  className?: string
}

// The date control the issuances grid uses, as its own component: a label saying the range
// in words, a row of presets, a range calendar, and two typed dates for the case none of the
// presets covers.
//
// Lifted here rather than reimplemented because "the same as the issuances screen" has to
// stay true - two copies drift, and the operator learns the control once. The grid's own
// bar keeps its inline copy for now; this is the same behaviour, ready for it to adopt.
export function DateRangeFilter({ from, to, onChange, className }: DateRangeFilterProps) {
  const [open, setOpen] = useState(false)
  const [range, setRange] = useState<DateRange | undefined>(() => ({
    from: parseYmd(from),
    to: parseYmd(to),
  }))
  // The calendar reports every click, including the half-made range between the first and
  // second date. The ref carries the latest one to the commit on close, where a range with
  // only a start still means something: a single day.
  const pending = useRef<DateRange | undefined>(range)

  function pick(next: DateRange | undefined) {
    setRange(next)
    pending.current = next
  }

  function applyPreset(start: Date, end: Date) {
    pick({ from: start, to: end })
    setOpen(false)
    onChange({ from: formatYmd(start), to: formatYmd(end) })
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      pick({ from: parseYmd(from), to: parseYmd(to) })
      return
    }

    const picked = pending.current
    const nextFrom = picked?.from ? formatYmd(picked.from) : ""
    const nextTo = picked?.to ? formatYmd(picked.to) : nextFrom
    if (nextFrom !== from || nextTo !== to) {
      onChange({ from: nextFrom, to: nextTo })
    }
  }

  function handleTyped(field: "from" | "to", value: string) {
    const next = field === "from" ? { from: value, to } : { from, to: value }
    pick({ from: parseYmd(next.from), to: parseYmd(next.to) })
  }

  const today = startOfToday()

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn(
            "w-52 justify-start gap-2 font-normal",
            !from && !to && "text-muted-foreground",
            className,
          )}
        >
          <CalendarDays className="h-4 w-4 shrink-0" />
          <span className="truncate">{dateRangeLabel(from, to)}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-3" align="start">
        {/* One scrollable row: the presets would otherwise widen the popover past the
            calendar underneath. */}
        <div className="mb-3 flex gap-2 overflow-x-auto pb-1 [&>button]:shrink-0">
          <Button type="button" variant="secondary" size="sm" onClick={() => applyPreset(today, today)}>
            Hoje
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => applyPreset(addDays(today, -1), addDays(today, -1))}
          >
            Ontem
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => applyPreset(addDays(today, -6), today)}
          >
            Últimos 7 dias
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() =>
              applyPreset(new Date(today.getFullYear(), today.getMonth(), 1), today)
            }
          >
            Este mês
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() =>
              applyPreset(
                new Date(today.getFullYear(), today.getMonth() - 1, 1),
                new Date(today.getFullYear(), today.getMonth(), 0),
              )
            }
          >
            Mês passado
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => applyPreset(new Date(today.getFullYear(), 0, 1), today)}
          >
            Este ano
          </Button>
        </div>
        <Calendar
          mode="range"
          autoFocus
          selected={range}
          onSelect={pick}
          defaultMonth={range?.from ?? today}
        />
        <div className="mt-3 flex items-end gap-2">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <Label className="text-xs text-slate-500">De</Label>
            <Input
              type="date"
              value={from}
              onChange={(e) => handleTyped("from", e.target.value)}
              className="bg-slate-100"
            />
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <Label className="text-xs text-slate-500">Até</Label>
            <Input
              type="date"
              value={to}
              onChange={(e) => handleTyped("to", e.target.value)}
              className="bg-slate-100"
            />
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
