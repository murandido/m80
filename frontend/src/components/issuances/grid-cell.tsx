"use client"

import { memo } from "react"
import { CellStateProps, useGridContext } from "@/components/issuances/grid-context"
// Shared with the copied EMISSAO message: an account with no alias has to be named the
// same way on line 4 as it is in this combobox.
import { accountFallbackLabel } from "@/components/issuances/emissao-message"
import { Account } from "@/schemas/account"
import { Person } from "@/schemas/person"
import { TextCell } from "@/components/issuances/cells/text-cell"
import { LocatorCell } from "@/components/issuances/cells/locator-cell"
import { DateMaskCell } from "@/components/issuances/cells/date-mask-cell"
import { NumberCell } from "@/components/issuances/cells/number-cell"
import { CalculatedCell } from "@/components/issuances/cells/calculated-cell"
import { ComboboxCell, ComboOption } from "@/components/issuances/cells/combobox-cell"
import { AirportAutocompleteCell } from "@/components/issuances/cells/airport-autocomplete-cell"
import { CiaAereaCell } from "@/components/issuances/cells/cia-aerea-cell"
import { StatusCell } from "@/components/issuances/cells/status-cell"
import { acceptsRedemptionFee } from "@/components/issuances/redemption-fee"
import { formatCurrencyBRL } from "@/lib/currency"

// A currency cell that selects and copies but never edits. Mirrors NumberCell's
// non-editing branch so a locked column lines up with the editable ones beside it.
function ReadOnlyCurrencyCell({ r, c, value }: { r: number; c: number; value: number | null }) {
  const { onCellClick } = useGridContext()

  return (
    <button
      type="button"
      onClick={() => onCellClick(r, c)}
      className="block w-full truncate px-2 py-1.5 text-right"
    >
      {value !== null ? formatCurrencyBRL(value) : <span className="text-muted-foreground">—</span>}
    </button>
  )
}

// One option per alias, so the operator picks a specific alias rather than an account
// labelled with all of its aliases at once. Aliases belong to a loyalty program, so the
// list is narrowed to the program the row is issuing on; before a program is chosen there
// is nothing to narrow by and the full list stays available.
function buildAccountOptions(
  accounts: Account[],
  people: Person[],
  loyaltyProgramId: string | null,
): ComboOption[] {
  // Archived accounts are in this list only because they came embedded in a past
  // issuance, so an old row could still be labelled with them. They are history, not
  // a choice - offering one would let somebody move an issuance onto an account that
  // no longer exists anywhere else in the panel.
  const live = accounts.filter((account) => !account.deleted_at)

  const selectable = loyaltyProgramId
    ? live.filter((account) => account.loyalty_program_id === loyaltyProgramId)
    : live

  return selectable.flatMap((account): ComboOption[] =>
    account.nicknames.length > 0
      ? account.nicknames.map((nickname) => ({
          key: nickname.id,
          label: nickname.nickname,
          patch: { account_id: account.id, account_nickname_id: nickname.id },
        }))
      : [
          {
            key: account.id,
            label: accountFallbackLabel(account, people),
            patch: { account_id: account.id, account_nickname_id: null },
          },
        ],
  )
}

// Volatile per-cell state (row/isEditing/pendingChar) arrives as props from the grid's
// render loop rather than through the context, so moving the selection re-renders only the
// cells whose props actually change. Memoized: with a stable context, an unchanged row and
// isEditing/pendingChar let React skip this cell (and its leaf) on a selection move.
export const GridCell = memo(function GridCell({
  r,
  c,
  row,
  isEditing,
  pendingChar,
}: { r: number; c: number } & CellStateProps) {
  const { accounts, users, agencies, people, createAccountForRow, pendingAccountLabels } =
    useGridContext()

  switch (c) {
    case 0:
      return <CiaAereaCell r={r} c={c} row={row} isEditing={isEditing} pendingChar={pendingChar} />
    case 1:
      return <LocatorCell r={r} c={c} row={row} isEditing={isEditing} pendingChar={pendingChar} />
    case 2:
      return <DateMaskCell r={r} c={c} row={row} isEditing={isEditing} pendingChar={pendingChar} />
    case 3:
      return (
        <TextCell
          r={r}
          c={c}
          row={row}
          isEditing={isEditing}
          pendingChar={pendingChar}
          field="main_passenger_name"
        />
      )
    case 4:
      return (
        <AirportAutocompleteCell
          r={r}
          c={c}
          row={row}
          isEditing={isEditing}
          pendingChar={pendingChar}
          field="origin_iata_code"
        />
      )
    case 5:
      return (
        <AirportAutocompleteCell
          r={r}
          c={c}
          row={row}
          isEditing={isEditing}
          pendingChar={pendingChar}
          field="destination_iata_code"
        />
      )
    case 6:
      return (
        <NumberCell
          r={r}
          c={c}
          row={row}
          isEditing={isEditing}
          pendingChar={pendingChar}
          field="points"
          mode="integer"
        />
      )
    case 7:
      return (
        <NumberCell
          r={r}
          c={c}
          row={row}
          isEditing={isEditing}
          pendingChar={pendingChar}
          field="value_of_the_thousand"
          mode="currency"
        />
      )
    case 8:
      return (
        <NumberCell
          r={r}
          c={c}
          row={row}
          isEditing={isEditing}
          pendingChar={pendingChar}
          field="taxes"
          mode="currency"
        />
      )
    case 9:
      return <CalculatedCell r={r} row={row} />
    case 10:
      return (
        <ComboboxCell
          r={r}
          c={c}
          isEditing={isEditing}
          pendingChar={pendingChar}
          options={buildAccountOptions(accounts, people, row.loyalty_program_id)}
          // Accounts without an alias are keyed by their own id, so this covers both cases.
          selectedKey={row.account_nickname_id ?? row.account_id}
          create={{
            label: (typed) => `Criar conta "${typed}"`,
            hint: "cria a pessoa, a conta no programa e o apelido",
            // An account cannot exist without a loyalty program, and the program comes
            // from the row - so the airline column has to be filled in first.
            disabledReason: row.loyalty_program_id
              ? null
              : "Escolha a Cia aérea antes de criar uma conta",
            onCreate: (typed) => createAccountForRow(row.key, typed),
          }}
          pendingLabel={pendingAccountLabels[row.key]}
        />
      )
    case 11:
      return (
        <ComboboxCell
          r={r}
          c={c}
          isEditing={isEditing}
          pendingChar={pendingChar}
          options={users.map((u) => ({ key: u.id, label: u.name, patch: { issuer_id: u.id } }))}
          selectedKey={row.issuer_id}
        />
      )
    case 12:
      return (
        <ComboboxCell
          r={r}
          c={c}
          isEditing={isEditing}
          pendingChar={pendingChar}
          options={agencies.map((a) => ({ key: a.id, label: a.name, patch: { agency_id: a.id } }))}
          selectedKey={row.agency_id}
        />
      )
    case 13:
      return <StatusCell r={r} c={c} row={row} isEditing={isEditing} />
    case 14:
      return (
        <TextCell
          r={r}
          c={c}
          row={row}
          isEditing={isEditing}
          pendingChar={pendingChar}
          field="observation"
        />
      )
    case 15:
      // Read-only on an ordinary account: the column measures an exemption this row does
      // not have, so there is nothing to type. Still a button, so the cell selects and
      // copies like any other - it is not editable, not inert.
      if (!acceptsRedemptionFee(row, accounts)) {
        return <ReadOnlyCurrencyCell r={r} c={c} value={row.redemption_fee_saved_cents} />
      }
      return (
        <NumberCell
          r={r}
          c={c}
          row={row}
          isEditing={isEditing}
          pendingChar={pendingChar}
          field="redemption_fee_saved_cents"
          mode="currency"
        />
      )
    default:
      return null
  }
})
