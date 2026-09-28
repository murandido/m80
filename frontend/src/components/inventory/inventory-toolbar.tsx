"use client"

import { useUrlQuery } from "@/hooks/use-url-query"
import { Label } from "@/components/ui/label"
import { AccountCombobox } from "@/components/accounts/account-combobox"
import { DateRangeFilter } from "@/components/filters/date-range-filter"
import type { AccountOption, LoyaltyProgramSummary } from "@/schemas/account"
import type { PersonOption } from "@/schemas/person"

interface Props {
  accounts: AccountOption[]
  // An account carries only loyalty_program_id, and the holder's name lives on the person.
  // Both are needed to say which account this is: two accounts of the same holder differ
  // only by programme, and an alias is free text that need not mention it.
  loyaltyPrograms: LoyaltyProgramSummary[]
  people: PersonOption[]
  selected: string[]
  from: string
  to: string
}

export function InventoryToolbar({
  accounts,
  loyaltyPrograms,
  people,
  selected,
  from,
  to,
}: Props) {
  const { update } = useUrlQuery()

  // Only accounts on the inventory regime. An ordinary account has no batches and never
  // will, so offering it would produce an empty screen with no way to tell why.
  const eligible = accounts.filter((account) => account.owns_point_inventory)

  const chosen = eligible.find((account) => account.id === selected[0])
  const programOf = loyaltyPrograms.find((program) => program.id === chosen?.loyalty_program_id)

  function push(changes: Record<string, string>) {
    update(
      (params) => {
        for (const [key, value] of Object.entries(changes)) {
          if (value) params.set(key, value)
          else params.delete(key)
        }
      },
      { push: true },
    )
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
      <div className="flex w-72 flex-col gap-1.5">
        <Label className="flex items-center gap-2 text-xs text-slate-500">
          Conta
          {/* The programme spelled out, not only the colour dot the combobox shows when
              closed. Two accounts of one holder differ by nothing else - "Marlon" on Azul
              and "Marlon" on Smiles read identically otherwise, and an alias is free text
              that need not mention the programme. */}
          {programOf && (
            <span
              className="rounded-full px-1.5 py-0.5 text-[11px] font-medium text-white"
              style={{ backgroundColor: programOf.color }}
            >
              {programOf.name}
            </span>
          )}
        </Label>
        {/* The grid's own account picker rather than a plain select: it names the account
            by its holder and shows the programme beside it, which is the only thing telling
            two accounts of the same person apart - one Azul, one Smiles, both called
            "Marlon". A select showing the alias alone cannot say which is which. */}
        <AccountCombobox
          accounts={eligible}
          people={people}
          loyaltyPrograms={loyaltyPrograms}
          value={selected[0] ?? ""}
          onChange={(id) => push({ account_ids: id })}
          placeholder="Escolha uma conta"
        />
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
