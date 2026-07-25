"use client"

import { useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { RotateCcw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import { AgencyCombobox } from "@/components/agencies/agency-combobox"
import { LoyaltyProgramCombobox } from "@/components/loyalty-programs/loyalty-program-combobox"
import { AccountCombobox } from "@/components/accounts/account-combobox"
import { UserCombobox } from "@/components/users/user-combobox"
import { ClientCombobox } from "@/components/clients/client-combobox"
import type { Agency } from "@/schemas/agency"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import type { Account } from "@/schemas/account"
import type { Person } from "@/schemas/person"
import type { User } from "@/schemas/user"
import type { Client } from "@/schemas/client"
import { SALES_FILTER_KEYS, type SalesFilterState } from "@/app/dashboard/sales/period-params"

interface SalesFilterBarProps {
  agencies: Agency[]
  loyaltyPrograms: LoyaltyProgram[]
  accounts: Account[]
  people: Person[]
  users: User[]
  clients: Client[]
  // Filters resolved by the server: the exact URL values, or the defaults on a clean load.
  initialFilters: SalesFilterState
  // The period control, which owns its own URL keys and is rendered by the page. It comes
  // in as children rather than as a prop of its own: an element passed through any other
  // prop from a Server Component to a Client Component trips React's missing-key warning.
  children: React.ReactNode
  // The suppliers view aggregates by account, so per-sale dimensions make no sense there.
  variant: "clients" | "suppliers"
}

const PAID_OPTIONS = [
  { value: "", label: "Todas" },
  { value: "false", label: "Em aberto" },
  { value: "true", label: "Pagas" },
] as const

export function SalesFilterBar({
  agencies,
  loyaltyPrograms,
  accounts,
  people,
  users,
  clients,
  initialFilters,
  children,
  variant,
}: SalesFilterBarProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const [filters, setFilters] = useState<SalesFilterState>(initialFilters)

  // Merges into the URL that is already there instead of rewriting it, because the period
  // control owns its own keys on the same page - a full rewrite here would reset the
  // period every time a filter changed.
  function update(partial: Partial<SalesFilterState>) {
    const next = { ...filters, ...partial }
    setFilters(next)

    const params = new URLSearchParams(searchParams.toString())
    for (const key of SALES_FILTER_KEYS) {
      if (next[key]) params.set(key, next[key])
      else params.delete(key)
    }
    router.replace(`${pathname}?${params.toString()}`)
  }

  const isClients = variant === "clients"

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
          <Label className="text-xs text-slate-500">Conta</Label>
          <AccountCombobox
            accounts={accounts}
            people={people}
            value={filters.account_id}
            onChange={(account_id) => update({ account_id })}
            placeholder="Todas"
          />
        </div>

        {isClients && (
          <>
            <div className="flex w-52 flex-col gap-1">
              <Label className="text-xs text-slate-500">Emissor</Label>
              <UserCombobox
                users={users}
                value={filters.issuer_id}
                onChange={(issuer_id) => update({ issuer_id })}
                placeholder="Todos"
              />
            </div>

            <div className="flex w-52 flex-col gap-1">
              <Label className="text-xs text-slate-500">Cliente</Label>
              <ClientCombobox
                clients={clients}
                value={filters.client_id}
                onChange={(client_id) => update({ client_id })}
                placeholder="Todos"
              />
            </div>

            <div className="flex flex-col gap-1">
              <Label className="text-xs text-slate-500">Pagamento</Label>
              <div className="flex gap-1">
                {PAID_OPTIONS.map((option) => (
                  <Button
                    key={option.value || "all"}
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => update({ paid: option.value })}
                    className={cn(
                      "font-normal",
                      filters.paid === option.value
                        ? "bg-slate-800 text-slate-50 hover:bg-slate-900 hover:text-slate-50"
                        : "bg-slate-100 text-slate-600 hover:bg-slate-200",
                    )}
                  >
                    {option.label}
                  </Button>
                ))}
              </div>
            </div>
          </>
        )}

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
