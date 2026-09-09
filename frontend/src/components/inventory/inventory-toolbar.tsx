"use client"

import { useRouter, useSearchParams } from "next/navigation"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { DateRangeFilter } from "@/components/filters/date-range-filter"
import type { AccountOption } from "@/schemas/account"

interface Props {
  accounts: AccountOption[]
  selected: string[]
  from: string
  to: string
}

// The label an account goes by here: its first nickname, then its login, then the head of
// the id. Same order the suppliers screen uses, so an account does not answer to two
// different names in two places.
function labelOf(account: AccountOption): string {
  return account.nicknames?.[0]?.nickname ?? account.login ?? account.id.slice(0, 8)
}

export function InventoryToolbar({ accounts, selected, from, to }: Props) {
  const router = useRouter()
  const searchParams = useSearchParams()

  // Only accounts on the inventory regime. An ordinary account has no batches and never
  // will, so offering it would produce an empty screen with no way to tell why.
  const eligible = accounts
    .filter((account) => account.owns_point_inventory)
    .sort((a, b) => labelOf(a).localeCompare(labelOf(b), "pt-BR"))

  function push(changes: Record<string, string>) {
    const params = new URLSearchParams(searchParams.toString())
    for (const [key, value] of Object.entries(changes)) {
      if (value) params.set(key, value)
      else params.delete(key)
    }
    router.push(`?${params.toString()}`)
  }

  if (eligible.length === 0) {
    return (
      <p className="text-sm text-slate-500">
        Nenhuma conta com estoque próprio. Marque a conta em Contas para começar.
      </p>
    )
  }

  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="inventory-account" className="text-xs text-slate-500">
          Conta
        </Label>
        <Select value={selected[0] ?? ""} onValueChange={(id) => push({ account_ids: id })}>
          <SelectTrigger id="inventory-account" className="w-56">
            <SelectValue placeholder="Escolha uma conta" />
          </SelectTrigger>
          <SelectContent>
            {eligible.map((account) => (
              <SelectItem key={account.id} value={account.id}>
                {labelOf(account)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label className="text-xs text-slate-500">Período</Label>
        <DateRangeFilter
          from={from}
          to={to}
          onChange={(range) => push({ date_from: range.from, date_to: range.to })}
        />
      </div>
    </div>
  )
}
