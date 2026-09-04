import { AccountOption } from "@/schemas/account"
import { IssuanceRow } from "@/components/issuances/types"

// Whether the redemption fee column accepts a value on this row.
//
// The column holds what an exempt account did NOT have to pay, so on an ordinary account
// there is no amount for a correction to be about - the backend clears anything stored
// there on the next recalculation. Blocked in the grid so the operator finds that out
// while typing rather than after the number silently disappears.
//
// An unknown account (none picked yet, or one not in the loaded list) counts as not
// exempt: the safe answer while we cannot tell is the one that writes nothing.
export function acceptsRedemptionFee(row: IssuanceRow, accounts: AccountOption[]): boolean {
  if (!row.account_id) return false
  return accounts.some(
    (account) => account.id === row.account_id && account.waives_redemption_fee,
  )
}
