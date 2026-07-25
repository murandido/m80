"use client"

import { useRef, useState } from "react"
import { usePathname, useRouter } from "next/navigation"
import { CalendarDays, RotateCcw } from "lucide-react"
import type { DateRange } from "react-day-picker"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { cn } from "@/lib/utils"
import {
  addDays,
  dateRangeLabel,
  formatYmd,
  parseYmd,
  startOfToday,
} from "@/lib/date-range"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import { AirlineCombobox } from "@/components/airlines/airline-combobox"
import { AccountCombobox } from "@/components/accounts/account-combobox"
import { UserCombobox } from "@/components/users/user-combobox"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import type { Airline } from "@/schemas/airline"
import type { Account } from "@/schemas/account"
import type { Person } from "@/schemas/person"
import type { User } from "@/schemas/user"

// The six filter dimensions carried in the URL. Empty string means "not filtered"
// on that dimension; the page treats a URL with any of these keys present as
// "user-controlled" and stops applying the today + current-user defaults.
const FILTER_KEYS = [
  "date_from",
  "date_to",
  "loyalty_program_id",
  "airline_id",
  "account_id",
  "issuer_id",
] as const

type FilterState = Record<(typeof FILTER_KEYS)[number], string>

interface IssuancesFilterBarProps {
  loyaltyPrograms: LoyaltyProgram[]
  airlines: Airline[]
  accounts: Account[]
  people: Person[]
  users: User[]
  // Effective filters resolved by the server (defaults on a clean load, or the
  // exact URL values otherwise). Seed the controls from these.
  initialFilters: FilterState
}

export function IssuancesFilterBar({
  loyaltyPrograms,
  airlines,
  accounts,
  people,
  users,
  initialFilters,
}: IssuancesFilterBarProps) {
  const router = useRouter()
  const pathname = usePathname()

  const [filters, setFilters] = useState<FilterState>(initialFilters)
  const [dateOpen, setDateOpen] = useState(false)
  const [calendarRange, setCalendarRange] = useState<DateRange | undefined>(
    () => ({
      from: parseYmd(initialFilters.date_from),
      to: parseYmd(initialFilters.date_to),
    }),
  )
  // Read inside the popover-close handler to avoid a stale-closure range.
  const pendingRangeRef = useRef<DateRange | undefined>(calendarRange)

  // Note: this component is keyed on the resolved filters by its parent, so it
  // remounts (re-seeding state from initialFilters) whenever the server hands down
  // new effective filters - e.g. after "Limpar filtros" or browser back/forward.

  // Writes ALL six keys (empty when cleared) so clearing one dimension never
  // drops the URL back to "no keys" and re-triggers the defaults.
  function pushFilters(next: FilterState) {
    const params = new URLSearchParams()
    for (const key of FILTER_KEYS) params.set(key, next[key])
    router.replace(`${pathname}?${params.toString()}`)
  }

  function update(partial: Partial<FilterState>) {
    const next = { ...filters, ...partial }
    setFilters(next)
    pushFilters(next)
  }

  function setRange(range: DateRange | undefined) {
    setCalendarRange(range)
    pendingRangeRef.current = range
  }

  function applyPreset(from: Date, to: Date) {
    setRange({ from, to })
    setDateOpen(false)
    update({ date_from: formatYmd(from), date_to: formatYmd(to) })
  }

  function handleDateOpenChange(next: boolean) {
    setDateOpen(next)
    if (next) {
      const range = {
        from: parseYmd(filters.date_from),
        to: parseYmd(filters.date_to),
      }
      setRange(range)
      return
    }

    // Closing: commit whatever range was picked via the calendar or manual inputs.
    const range = pendingRangeRef.current
    const from = range?.from ? formatYmd(range.from) : ""
    const to = range?.to ? formatYmd(range.to) : from
    if (from !== filters.date_from || to !== filters.date_to) {
      update({ date_from: from, date_to: to })
    }
  }

  function handleManualDate(field: "date_from" | "date_to", value: string) {
    const next = { ...filters, [field]: value }
    setRange({ from: parseYmd(next.date_from), to: parseYmd(next.date_to) })
  }

  const today = startOfToday()
  const hasAnyFilter = FILTER_KEYS.some((key) => filters[key] !== "")

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Popover open={dateOpen} onOpenChange={handleDateOpenChange}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            className={cn(
              "w-44 justify-start gap-2 bg-slate-200 font-normal hover:bg-slate-200",
              !filters.date_from && !filters.date_to && "text-muted-foreground",
            )}
          >
            <CalendarDays className="h-4 w-4 shrink-0" />
            <span className="truncate">
              {dateRangeLabel(filters.date_from, filters.date_to)}
            </span>
          </Button>
        </PopoverTrigger>
          <PopoverContent className="w-72 p-3" align="start">
            {/* Single scrollable row: the presets would otherwise widen the whole popover
                well past the calendar. flex-nowrap + overflow-x-auto keeps them on one line
                with a horizontal scrollbar; the buttons never shrink below their label. */}
            <div className="mb-3 flex gap-2 overflow-x-auto pb-1 [&>button]:shrink-0">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => applyPreset(today, today)}
              >
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
                  applyPreset(
                    new Date(today.getFullYear(), today.getMonth(), 1),
                    today,
                  )
                }
              >
                Este mês
              </Button>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() =>
                  applyPreset(new Date(today.getFullYear(), 0, 1), today)
                }
              >
                Este ano
              </Button>
            </div>
            <Calendar
              mode="range"
              autoFocus
              selected={calendarRange}
              onSelect={setRange}
              defaultMonth={calendarRange?.from ?? today}
            />
            <div className="mt-3 flex items-end gap-2">
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <Label className="text-xs text-slate-500">De</Label>
                <Input
                  type="date"
                  value={filters.date_from}
                  onChange={(e) => handleManualDate("date_from", e.target.value)}
                  className="bg-slate-100"
                />
              </div>
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <Label className="text-xs text-slate-500">Até</Label>
                <Input
                  type="date"
                  value={filters.date_to}
                  onChange={(e) => handleManualDate("date_to", e.target.value)}
                  className="bg-slate-100"
                />
              </div>
            </div>
        </PopoverContent>
      </Popover>

      <div className="w-44">
        <LoyaltyProgramCombobox
          loyaltyPrograms={loyaltyPrograms}
          value={filters.loyalty_program_id}
          onChange={(id) => update({ loyalty_program_id: id })}
          placeholder="Todos os programas"
        />
      </div>

      <div className="w-44">
        <AirlineCombobox
          airlines={airlines}
          value={filters.airline_id}
          onChange={(id) => update({ airline_id: id })}
          placeholder="Todas as cias"
        />
      </div>

      <div className="w-44">
        <AccountCombobox
          accounts={accounts}
          people={people}
          value={filters.account_id}
          onChange={(id) => update({ account_id: id })}
          placeholder="Todas as contas"
        />
      </div>

      <div className="w-44">
        <UserCombobox
          users={users}
          value={filters.issuer_id}
          onChange={(id) => update({ issuer_id: id })}
          placeholder="Todos os emissores"
        />
      </div>

      {hasAnyFilter && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          title="Limpar filtros"
          aria-label="Limpar filtros"
          className="text-slate-500 hover:text-foreground"
          onClick={() => router.replace(pathname)}
        >
          <RotateCcw className="h-4 w-4" />
        </Button>
      )}
    </div>
  )
}
