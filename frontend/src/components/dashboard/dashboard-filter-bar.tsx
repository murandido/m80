"use client"

import { useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { RotateCcw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { AgencyCombobox } from "@/components/agencies/agency-combobox"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import { AirlineCombobox } from "@/components/airlines/airline-combobox"
import { UserCombobox } from "@/components/users/user-combobox"
import type { Agency } from "@/schemas/agency"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import type { Airline } from "@/schemas/airline"
import type { User } from "@/schemas/user"
import {
  DASHBOARD_FILTER_KEYS,
  type DashboardFilterState,
} from "@/app/dashboard/dashboard-params"

interface DashboardFilterBarProps {
  agencies: Agency[]
  loyaltyPrograms: LoyaltyProgram[]
  airlines: Airline[]
  users: User[]
  initialFilters: DashboardFilterState
  // The period control and the metric toggle, which own their own URL keys and are
  // rendered by the page. They come in as children rather than as props of their own: an
  // element passed through any other prop from a Server Component to a Client Component
  // trips React's missing-key warning.
  children: React.ReactNode
}

// One filter row, above everything it scopes. Every tile and chart on the page re-renders
// against the same slice, so the numbers can never disagree with each other.
export function DashboardFilterBar({
  agencies,
  loyaltyPrograms,
  airlines,
  users,
  initialFilters,
  children,
}: DashboardFilterBarProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  // A mirror of the server-resolved filters, for instant feedback on the controls. The URL
  // stays the source of truth.
  const [filters, setFilters] = useState<DashboardFilterState>(initialFilters)

  // Merges into the URL that is already there instead of rewriting it, because the period
  // control and the metric toggle own their own keys on the same page - a full rewrite
  // would reset them every time a filter changed.
  function update(partial: Partial<DashboardFilterState>) {
    const next = { ...filters, ...partial }
    setFilters(next)

    const params = new URLSearchParams(searchParams.toString())
    for (const key of DASHBOARD_FILTER_KEYS) {
      if (next[key]) params.set(key, next[key])
      else params.delete(key)
    }
    router.replace(`${pathname}?${params.toString()}`)
  }

  return (
    <div className="flex flex-col gap-3">
      {children}

      <div className="flex flex-wrap items-end gap-3">
        <div className="flex w-52 flex-col gap-1">
          <Label className="text-xs text-slate-500">Agência</Label>
          <AgencyCombobox
            agencies={agencies}
            value={filters.agency_id}
            onChange={(agency_id) => update({ agency_id })}
            placeholder="Agência principal"
            clearable={false}
          />
        </div>

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
          <Label className="text-xs text-slate-500">Cia aérea</Label>
          <AirlineCombobox
            airlines={airlines}
            value={filters.airline_id}
            onChange={(airline_id) => update({ airline_id })}
            placeholder="Todas"
          />
        </div>

        <div className="flex w-52 flex-col gap-1">
          <Label className="text-xs text-slate-500">Emissor</Label>
          <UserCombobox
            users={users}
            value={filters.issuer_id}
            onChange={(issuer_id) => update({ issuer_id })}
            placeholder="Todos"
          />
        </div>

        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => router.replace(pathname)}
          className="text-slate-500"
        >
          <RotateCcw className="h-4 w-4" />
          Limpar filtros
        </Button>
      </div>
    </div>
  )
}
