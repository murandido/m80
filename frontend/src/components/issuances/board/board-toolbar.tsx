"use client"

import { useState } from "react"
import { usePathname } from "next/navigation"
import {
  ChevronDown,
  FileSpreadsheet,
  FileText,
  Filter,
  Keyboard,
  RotateCcw,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { DateRangeFilter } from "@/components/filters/date-range-filter"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import { AirlineCombobox } from "@/components/airlines/airline-combobox"
import { AccountCombobox } from "@/components/accounts/account-combobox"
import { AgencyCombobox } from "@/components/agencies/agency-combobox"
import { UserMultiCombobox } from "@/components/users/user-multi-combobox"
import { SimulateIssuanceDialog } from "@/components/issuances/board/simulate-issuance-dialog"
import { clearStickyFilters } from "@/lib/sticky-filters"
import { useStickyFilters } from "@/hooks/use-sticky-filters"
import { useUrlQuery } from "@/hooks/use-url-query"
import { cn } from "@/lib/utils"
import type { GridReferenceData } from "@/components/issuances/grid-context"

// The same seven URL keys the grid's filter bar writes (issuances-filter-bar.tsx), so the
// page's resolveFilters and the sticky-filter cookie read the board exactly as they read
// the grid. Duplicated rather than imported for the reason use-issuance-board.ts gives.
const FILTER_KEYS = [
  "date_from",
  "date_to",
  "loyalty_program_id",
  "airline_id",
  "account_id",
  "agency_id",
  "issuer_id",
] as const

export type FilterState = Record<(typeof FILTER_KEYS)[number], string>

const ISSUERS_ALL = "all"
const ISSUERS_NONE = "none"

function parseIssuerParam(
  param: string,
  users: GridReferenceData["users"],
): string[] {
  if (param === "" || param === ISSUERS_ALL) return users.map((user) => user.id)
  if (param === ISSUERS_NONE) return []
  return param.split(",").filter(Boolean)
}

function serializeIssuerSelection(
  ids: string[],
  users: GridReferenceData["users"],
): string {
  if (ids.length === 0) return ISSUERS_NONE
  if (users.length > 0 && ids.length >= users.length) return ISSUERS_ALL
  return ids.join(",")
}

// Shown in the "Atalhos" popover, so the keyboard flow is discoverable.
export const BOARD_SHORTCUTS: { keys: string; label: string }[] = [
  { keys: "Ctrl+Enter", label: "Nova emissão" },
  { keys: "Setas", label: "Andar entre os cards" },
  { keys: "Enter", label: "Abrir o card" },
  { keys: "Tab / Shift+Tab", label: "Próximo / campo anterior" },
  { keys: "Enter (num campo)", label: "Confirmar e ir ao próximo" },
  { keys: "/", label: "Programa → Cia; localizador do programa → da cia" },
  { keys: "Esc", label: "Fechar a lista; de novo, minimizar" },
  { keys: "Ctrl+Alt+C", label: "Copiar a emissão como mensagem" },
  { keys: "Ctrl+Alt+X", label: "Copiar só o cálculo" },
  { keys: "Ctrl+Shift+⌫", label: "Excluir a emissão aberta" },
]

interface BoardFiltersProps extends Pick<
  GridReferenceData,
  "loyaltyPrograms" | "airlines" | "accounts" | "agencies" | "people" | "users"
> {
  initialFilters: FilterState
  hasSearch: boolean
  currentUserId: string | null
}

// "Período" and "Filtros". The period is its own control because it is the one changed
// every day; every other dimension sits behind the Filtros popover.
export function BoardFilters({
  loyaltyPrograms,
  airlines,
  accounts,
  agencies,
  people,
  users,
  initialFilters,
  hasSearch,
  currentUserId,
}: BoardFiltersProps) {
  const pathname = usePathname()
  const { replace, update: updateUrl } = useUrlQuery()
  useStickyFilters()
  const [filters, setFilters] = useState<FilterState>(initialFilters)

  // Writes every key, so clearing one dimension never drops the URL back to "no keys" and
  // re-triggers the today + me defaults.
  function update(partial: Partial<FilterState>) {
    const next = { ...filters, ...partial }
    setFilters(next)
    updateUrl((params) => {
      for (const key of FILTER_KEYS) params.set(key, next[key])
    })
  }

  const selectedIssuers = parseIssuerParam(filters.issuer_id, users)
  const summary: string[] = []
  const pick = <T extends { id: string; name: string }>(
    list: T[],
    id: string,
  ) => list.find((item) => item.id === id)?.name
  if (filters.loyalty_program_id)
    summary.push(
      pick(loyaltyPrograms, filters.loyalty_program_id) ?? "Programa",
    )
  if (filters.airline_id)
    summary.push(pick(airlines, filters.airline_id) ?? "Cia")
  if (filters.account_id) summary.push("Conta")
  if (filters.agency_id)
    summary.push(pick(agencies, filters.agency_id) ?? "Agência")
  if (filters.issuer_id && filters.issuer_id !== ISSUERS_ALL) {
    summary.push(
      selectedIssuers.length === 1 && selectedIssuers[0] === currentUserId
        ? "Emitidas por mim"
        : `${selectedIssuers.length} emissores`,
    )
  }
  const hasAnyFilter =
    hasSearch || FILTER_KEYS.some((key) => filters[key] !== "")
  const searchOverrideTitle =
    "A busca procura em todas as emissões, ignorando data e emissor. Limpe a busca para voltar a usar estes filtros."

  return (
    <div className="grid grid-cols-2 gap-3 lg:flex lg:items-end">
      <div
        className="flex min-w-0 flex-col gap-1.5"
        title={hasSearch ? searchOverrideTitle : undefined}
      >
        <Label className="hidden text-slate-600 lg:block">Período</Label>
        <DateRangeFilter
          from={filters.date_from}
          to={filters.date_to}
          onChange={({ from, to }) => update({ date_from: from, date_to: to })}
          className={cn(
            "h-10 w-full flex-row-reverse justify-end border-0 bg-slate-200 hover:bg-slate-300/70 lg:w-48",
            hasSearch && "pointer-events-none opacity-50",
          )}
        />
      </div>

      <div className="flex min-w-0 flex-col gap-1.5">
        <Label className="hidden text-slate-600 lg:block">Filtros</Label>
        <Popover>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="outline"
              className="h-10 w-full justify-between gap-2 border-0 bg-slate-200 px-3 font-normal hover:bg-slate-300/70 lg:w-48"
            >
              <span
                className={cn(
                  "truncate",
                  summary.length === 0 && "text-muted-foreground",
                )}
              >
                {summary.length > 0 ? summary.join(", ") : "Filtros"}
              </span>
              <Filter className="h-4 w-4 shrink-0 text-slate-500" />
            </Button>
          </PopoverTrigger>
          <PopoverContent
            align="start"
            className="flex w-72 flex-col gap-3 rounded-xl p-3"
          >
            <FilterField label="Programa">
              <LoyaltyProgramCombobox
                loyaltyPrograms={loyaltyPrograms}
                value={filters.loyalty_program_id}
                onChange={(id) => update({ loyalty_program_id: id })}
                placeholder="Todos os programas"
              />
            </FilterField>
            <FilterField label="Cia aérea">
              <AirlineCombobox
                airlines={airlines}
                value={filters.airline_id}
                onChange={(id) => update({ airline_id: id })}
                placeholder="Todas as cias"
              />
            </FilterField>
            <FilterField label="Conta">
              <AccountCombobox
                accounts={accounts}
                people={people}
                loyaltyPrograms={loyaltyPrograms}
                value={filters.account_id}
                onChange={(id) => update({ account_id: id })}
                placeholder="Todas as contas"
              />
            </FilterField>
            <FilterField label="Agência">
              <AgencyCombobox
                agencies={agencies}
                value={filters.agency_id}
                onChange={(id) => update({ agency_id: id })}
                placeholder="Todas as agências"
              />
            </FilterField>
            <FilterField label="Emissor">
              <div title={hasSearch ? searchOverrideTitle : undefined}>
                <UserMultiCombobox
                  users={users}
                  value={selectedIssuers}
                  onChange={(ids) =>
                    update({ issuer_id: serializeIssuerSelection(ids, users) })
                  }
                  disabled={hasSearch}
                />
              </div>
            </FilterField>
            {hasAnyFilter && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="self-end text-slate-500"
                // Forgetting comes first: without it the bare URL would be repopulated
                // from the cookie the operator just asked to drop.
                onClick={() => {
                  clearStickyFilters(pathname)
                  replace(new URLSearchParams())
                }}
              >
                <RotateCcw className="h-3.5 w-3.5" />
                Limpar filtros
              </Button>
            )}
          </PopoverContent>
        </Popover>
      </div>
    </div>
  )
}

function FilterField({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-slate-500">{label}</span>
      {children}
    </div>
  )
}

// Atalhos, Exportar and Simular Emissão. Three equal buttons on a phone, as in the design.
export function BoardActions({ filterQuery }: { filterQuery: string }) {
  const buttonClass = "h-10 w-full bg-white px-2 shadow-xs lg:w-auto lg:px-4"

  return (
    <div className="grid grid-cols-3 gap-2 lg:flex">
      <Popover>
        <PopoverTrigger asChild>
          <Button type="button" variant="outline" className={buttonClass}>
            <Keyboard className="h-4 w-4" />
            Atalhos
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-80">
          <p className="mb-2 text-sm font-medium">Atalhos de teclado</p>
          <dl className="space-y-1">
            {BOARD_SHORTCUTS.map((shortcut) => (
              <div
                key={shortcut.keys}
                className="flex items-baseline justify-between gap-3 text-xs"
              >
                <dt className="text-muted-foreground">{shortcut.label}</dt>
                <dd className="shrink-0 font-mono text-[11px]">
                  {shortcut.keys}
                </dd>
              </div>
            ))}
          </dl>
        </PopoverContent>
      </Popover>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="outline" className={buttonClass}>
            <FileText className="h-4 w-4" />
            Exportar
            <ChevronDown className="hidden h-4 w-4 sm:block" />
          </Button>
        </DropdownMenuTrigger>
        {/* Native downloads with the filters the board was rendered with, so the sheet
            matches what is on screen. */}
        <DropdownMenuContent align="end">
          <DropdownMenuItem asChild>
            <a href={`/api/issuances/export?${filterQuery}`} download>
              <FileSpreadsheet />
              Planilha de controle
            </a>
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <a
              href={`/api/issuances/export?${filterQuery ? `${filterQuery}&` : ""}layout=legacy`}
              download
            >
              <FileSpreadsheet />
              Planilha Emissões Gerais (antiga)
            </a>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <SimulateIssuanceDialog triggerClassName={buttonClass} />
    </div>
  )
}
