"use client"

import { useState } from "react"
import type { DateRange } from "react-day-picker"
import { CalendarDays } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { DateInput } from "@/components/ui/date-input"
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

interface Draft {
  from: string
  to: string
}

const DAY_MS = 24 * 60 * 60 * 1000

// Inclusive count of days in the draft, for the confirm button. Rounded because the two
// midnights can sit an hour apart across a DST change, and a range is a count of days.
function dayCount(draft: Draft): number {
  const from = parseYmd(draft.from)
  const to = parseYmd(draft.to || draft.from)
  if (!from || !to) return 0
  return Math.round((to.getTime() - from.getTime()) / DAY_MS) + 1
}

function presets(today: Date): { label: string; from: Date; to: Date }[] {
  return [
    { label: "Hoje", from: today, to: today },
    { label: "Ontem", from: addDays(today, -1), to: addDays(today, -1) },
    { label: "Últimos 7 dias", from: addDays(today, -6), to: today },
    {
      label: "Este mês",
      from: new Date(today.getFullYear(), today.getMonth(), 1),
      to: today,
    },
    {
      label: "Mês passado",
      from: new Date(today.getFullYear(), today.getMonth() - 1, 1),
      to: new Date(today.getFullYear(), today.getMonth(), 0),
    },
    { label: "Este ano", from: new Date(today.getFullYear(), 0, 1), to: today },
  ]
}

// The shared date control: a field saying the range, and a popover with presets, a range
// calendar and the two typed ends, in the design every screen is converging on.
//
// Everything in the popover edits a draft, and only the button at the bottom applies it.
// Each change here refetches the screen behind it, so clicking through a calendar should
// not fire one request per click - and closing without confirming is how an operator backs
// out of a range they did not mean.
export function DateRangeFilter({ from, to, onChange, className }: DateRangeFilterProps) {
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft>({ from, to })
  const [month, setMonth] = useState<Date>(() => parseYmd(from) ?? startOfToday())

  // Moves the calendar along with the draft, so a preset or a typed date lands on a month
  // where the operator can see what they picked.
  function edit(next: Draft) {
    setDraft(next)
    const start = parseYmd(next.from)
    if (start) setMonth(start)
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) edit({ from, to })
  }

  function handleCalendar(range: DateRange | undefined) {
    // Only the draft follows a click: the month stays put, or picking the end of a range
    // would jump the view back to where it started.
    setDraft({
      from: range?.from ? formatYmd(range.from) : "",
      to: range?.to ? formatYmd(range.to) : "",
    })
  }

  // Either typed end. The other follows when it would be crossed, because an inverted
  // range filters nothing at all and reads as a screen with no data.
  function handleTyped(end: "from" | "to", value: string) {
    if (!value) {
      setDraft({ ...draft, [end]: "" })
      return
    }
    if (end === "from") {
      edit({ from: value, to: draft.to && draft.to < value ? value : draft.to })
    } else {
      edit({ from: draft.from && draft.from > value ? value : draft.from, to: value })
    }
  }

  function confirm() {
    // A range with only a start is a single day, which is what a single click means.
    const next = { from: draft.from, to: draft.to || draft.from }
    setOpen(false)
    if (next.from !== from || next.to !== to) onChange(next)
  }

  const days = dayCount(draft)

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn(
            "min-w-44 justify-between gap-2 bg-white px-3 font-normal",
            !from && !to && "text-muted-foreground",
            className,
          )}
        >
          <span className="truncate">{dateRangeLabel(from, to)}</span>
          <CalendarDays className="h-4 w-4 shrink-0 text-slate-900" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 rounded-xl p-3" align="start">
        {/* One scrollable row: the presets would otherwise widen the popover past the
            calendar underneath. */}
        <div className="-mx-3 mb-3 flex gap-2 overflow-x-auto px-3 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {presets(startOfToday()).map((preset) => (
            <Button
              key={preset.label}
              type="button"
              variant="ghost"
              onClick={() => edit({ from: formatYmd(preset.from), to: formatYmd(preset.to) })}
              className="h-8 shrink-0 rounded-lg bg-neutral-100 px-3 font-normal hover:bg-neutral-200"
            >
              {preset.label}
            </Button>
          ))}
        </div>
        <Calendar
          mode="range"
          autoFocus
          selected={{ from: parseYmd(draft.from), to: parseYmd(draft.to) }}
          onSelect={handleCalendar}
          month={month}
          onMonthChange={setMonth}
          className="p-0 [--cell-size:--spacing(9)]"
          classNames={{ root: "w-full" }}
        />
        <div className="mt-3 flex items-end gap-2">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <Label className="text-xs text-slate-500">De</Label>
            <DateInput
              picker={false}
              value={draft.from}
              onChange={(value) => handleTyped("from", value)}
              className="[&_input]:bg-slate-100"
            />
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <Label className="text-xs text-slate-500">Até</Label>
            <DateInput
              picker={false}
              value={draft.to}
              onChange={(value) => handleTyped("to", value)}
              className="[&_input]:bg-slate-100"
            />
          </div>
        </div>
        <Button
          type="button"
          onClick={confirm}
          disabled={days === 0}
          className="mt-3 w-full rounded-lg bg-slate-500 text-white hover:bg-slate-600"
        >
          {days === 1 ? "Selecionar período de 1 dia" : `Selecionar período de ${days} dias`}
        </Button>
      </PopoverContent>
    </Popover>
  )
}
