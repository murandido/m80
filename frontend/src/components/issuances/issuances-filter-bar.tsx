"use client"

import { useRef, useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { CalendarDays, Info, RotateCcw } from "lucide-react"
import type { DateRange } from "react-day-picker"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { DateInput } from "@/components/ui/date-input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Switch } from "@/components/ui/switch"
import { cn } from "@/lib/utils"
import { clearStickyFilters } from "@/lib/sticky-filters"
import { useAutocompletePreference } from "@/hooks/use-autocomplete-preference"
import { useStickyFilters } from "@/hooks/use-sticky-filters"
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
import { AgencyCombobox } from "@/components/agencies/agency-combobox"
import { UserCombobox } from "@/components/users/user-combobox"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import type { Airline } from "@/schemas/airline"
import type { AccountOption } from "@/schemas/account"
import type { Agency } from "@/schemas/agency"
import type { PersonOption } from "@/schemas/person"
import type { User } from "@/schemas/user"

// The seven filter dimensions carried in the URL. Empty string means "not filtered"
// on that dimension; the page treats a URL with any of these keys present as
// "user-controlled" and stops applying the today + current-user defaults.
const FILTER_KEYS = [
  "date_from",
  "date_to",
  "loyalty_program_id",
  "airline_id",
  "account_id",
  "agency_id",
  "issuer_id",
] as const

type FilterState = Record<(typeof FILTER_KEYS)[number], string>

interface IssuancesFilterBarProps {
  loyaltyPrograms: LoyaltyProgram[]
  airlines: Airline[]
  accounts: AccountOption[]
  agencies: Agency[]
  people: PersonOption[]
  users: User[]
  // Effective filters resolved by the server (defaults on a clean load, or the
  // exact URL values otherwise). Seed the controls from these.
  initialFilters: FilterState
  // True while the search box has a term. The backend drops the date and issuer
  // filters in that case, so the two controls are greyed out rather than left
  // looking like they still apply.
  hasSearch: boolean
}

export function IssuancesFilterBar({
  loyaltyPrograms,
  airlines,
  accounts,
  agencies,
  people,
  users,
  initialFilters,
  hasSearch,
}: IssuancesFilterBarProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  useStickyFilters()

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

  // Writes ALL the keys (empty when cleared) so clearing one dimension never
  // drops the URL back to "no keys" and re-triggers the defaults. The search term
  // is carried over by hand: this builds a fresh URLSearchParams, so anything not
  // written here is erased - and erasing it would clear the box mid-search.
  function pushFilters(next: FilterState) {
    const params = new URLSearchParams()
    for (const key of FILTER_KEYS) params.set(key, next[key])
    const search = searchParams.get("q")
    if (search) params.set("q", search)
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
  const hasAnyFilter = hasSearch || FILTER_KEYS.some((key) => filters[key] !== "")
  const searchOverrideTitle =
    "A busca procura em todas as emissões, ignorando data e emissor. Limpe a busca para voltar a usar estes filtros."

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Popover open={dateOpen} onOpenChange={handleDateOpenChange}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            disabled={hasSearch}
            title={hasSearch ? searchOverrideTitle : undefined}
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
                <DateInput
                  picker={false}
                  value={filters.date_from}
                  onChange={(value) => handleManualDate("date_from", value)}
                  className="[&_input]:bg-slate-100"
                />
              </div>
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <Label className="text-xs text-slate-500">Até</Label>
                <DateInput
                  picker={false}
                  value={filters.date_to}
                  onChange={(value) => handleManualDate("date_to", value)}
                  className="[&_input]:bg-slate-100"
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
          loyaltyPrograms={loyaltyPrograms}
          value={filters.account_id}
          onChange={(id) => update({ account_id: id })}
          placeholder="Todas as contas"
        />
      </div>

      <div className="w-44">
        <AgencyCombobox
          agencies={agencies}
          value={filters.agency_id}
          onChange={(id) => update({ agency_id: id })}
          placeholder="Todas as agências"
        />
      </div>

      <div className="w-44" title={hasSearch ? searchOverrideTitle : undefined}>
        <UserCombobox
          users={users}
          value={filters.issuer_id}
          onChange={(id) => update({ issuer_id: id })}
          placeholder="Todos os emissores"
          disabled={hasSearch}
        />
      </div>

      {hasSearch && (
        // Without this the two greyed-out controls read as a broken screen rather
        // than as a deliberate consequence of searching.
        <span
          className="flex items-center gap-1 text-xs text-muted-foreground"
          title={searchOverrideTitle}
        >
          <Info aria-hidden className="h-3.5 w-3.5" />
          Busca ignora data e emissor
        </span>
      )}

      {hasAnyFilter && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          title="Limpar filtros"
          aria-label="Limpar filtros"
          className="text-slate-500 hover:text-foreground"
          // Forgetting comes first: without it the bare URL would immediately be
          // repopulated from the cookie the operator just asked to drop.
          onClick={() => {
            clearStickyFilters(pathname)
            router.replace(pathname)
          }}
        >
          <RotateCcw className="h-4 w-4" />
        </Button>
      )}
    </div>
  )
}

// Rendered by the grid, beside "Nova emissão" under the table: with it on, that button
// starts the Traveos-first flow, so the two read as one control.
//
// Opt-in, and off by default. With it on, a row fetches from Traveos by itself as
// soon as it has the minimum fields the airline needs - which is a real emission
// at the vendor, so it must never happen because someone opened the page.
//
// The product doc describes this as fully automatic with no control at all. The
// toggle is the deliberate difference: the operator fixing yesterday's numbers is
// editing the same fields as the one entering a new booking, and the vendor cannot
// tell those two apart.
export function AutocompleteToggle() {
  const [enabled, setEnabled] = useAutocompletePreference()

  return (
    <label className="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
      <Switch
        checked={enabled}
        onCheckedChange={setEnabled}
        aria-label="Autocompletar dados"
      />
      <span title="Puxa os dados da Traveos sozinho assim que a linha tiver o mínimo que a companhia exige">
        Autocompletar dados
      </span>
    </label>
  )
}
