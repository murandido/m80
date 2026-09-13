"use client"

import { useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { RotateCcw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { clearStickyFilters } from "@/lib/sticky-filters"
import { useStickyFilters } from "@/hooks/use-sticky-filters"
import { AgencyCombobox } from "@/components/agencies/agency-combobox"
import { AgencyMultiCombobox } from "@/components/agencies/agency-multi-combobox"
import { PersonCombobox } from "@/components/people/person-combobox"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import { AccountCombobox } from "@/components/accounts/account-combobox"
import type { Agency } from "@/schemas/agency"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import type { AccountOption } from "@/schemas/account"
import type { PersonOption } from "@/schemas/person"
import {
  parseAgencyIds,
  SALES_FILTER_KEYS,
  serializeAgencyIds,
  type SalesFilterState,
} from "@/app/dashboard/sales/period-params"

interface SalesFilterBarProps {
  agencies: Agency[]
  loyaltyPrograms: LoyaltyProgram[]
  accounts: AccountOption[]
  people: PersonOption[]
  // Filters resolved by the server: the exact URL values, or the defaults on a clean load.
  initialFilters: SalesFilterState
  // The period control, which owns its own URL keys and is rendered by the page. It comes
  // in as children rather than as a prop of its own: an element passed through any other
  // prop from a Server Component to a Client Component trips React's missing-key warning.
  children: React.ReactNode
  // The clients view filters on the agency alone; client and payment status are filtered
  // from its own column headers (see SalesClientsTable).
  variant: "clients" | "suppliers"
}

export function SalesFilterBar({
  agencies,
  loyaltyPrograms,
  accounts,
  people,
  initialFilters,
  children,
  variant,
}: SalesFilterBarProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  useStickyFilters()
  const [filters, setFilters] = useState<SalesFilterState>(initialFilters)

  // Writes only the keys that changed, merged into the URL that is already there: the
  // period control and the clients table's column filters own other keys on the same page,
  // and this bar's state is a copy taken at mount - rewriting every key from it would put
  // back a column filter somebody changed since. The URL is authoritative (useStickyFilters
  // restores remembered filters into it on arrival), so nothing is lost by not rewriting.
  function update(partial: Partial<SalesFilterState>) {
    setFilters({ ...filters, ...partial })

    const params = new URLSearchParams(searchParams.toString())
    for (const key of SALES_FILTER_KEYS) {
      if (!(key in partial)) continue
      if (partial[key]) params.set(key, partial[key])
      else params.delete(key)
    }
    router.replace(`${pathname}?${params.toString()}`)
  }

  const isClients = variant === "clients"

  return (
    <div className="flex flex-col gap-3">
      {/* The clients view has a single row - agency, period, clear - so the period sits
          inline there; the suppliers view keeps its preset buttons on a row of their own. */}
      {!isClients && children}

      <div className="flex flex-wrap items-end gap-3">
        <div className={isClients ? "flex w-full flex-col gap-1 sm:w-52" : "flex w-52 flex-col gap-1"}>
          <Label className="text-xs text-slate-500">Agência</Label>
          {isClients ? (
            <AgencyCombobox
              agencies={agencies}
              value={filters.agency_id}
              onChange={(agency_id) => update({ agency_id })}
              placeholder="Agência principal"
              clearable={false}
            />
          ) : (
            <AgencyMultiCombobox
              agencies={agencies}
              value={parseAgencyIds(filters.agency_ids)}
              onChange={(ids) => update({ agency_ids: serializeAgencyIds(ids) })}
            />
          )}
        </div>

        {isClients && children}

        {!isClients && (
          <>
            <div className="flex w-52 flex-col gap-1">
              <Label className="text-xs text-slate-500">Programa</Label>
              <LoyaltyProgramCombobox
                loyaltyPrograms={loyaltyPrograms}
                value={filters.loyalty_program_id}
                onChange={(loyalty_program_id) => update({ loyalty_program_id })}
                placeholder="Todos"
              />
            </div>

            <div className="flex w-52 flex-col gap-1">
              <Label className="text-xs text-slate-500">Conta</Label>
              <AccountCombobox
                accounts={accounts}
                people={people}
                loyaltyPrograms={loyaltyPrograms}
                value={filters.account_id}
                onChange={(account_id) => update({ account_id })}
                placeholder="Todas"
              />
            </div>

            <div className="flex w-52 flex-col gap-1">
              <Label className="text-xs text-slate-500">Administrador</Label>
              <PersonCombobox
                people={people}
                value={filters.administrator_id}
                onChange={(administrator_id) => update({ administrator_id })}
                placeholder="Todos"
              />
            </div>
          </>
        )}

        <Button
          type="button"
          variant="ghost"
          size="sm"
          // Forgetting comes first: without it the bare URL would immediately be
          // repopulated from the cookie the operator just asked to drop.
          onClick={() => {
            clearStickyFilters(pathname)
            router.replace(pathname)
          }}
          className="text-slate-500"
        >
          <RotateCcw className="h-4 w-4" />
          Limpar filtros
        </Button>
      </div>
    </div>
  )
}
