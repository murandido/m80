import { Issuance } from "@/schemas/issuance"
import { IssuanceRow, issuanceToRow } from "@/components/issuances/types"

// A row the reconcile must never rebuild from server data. This mirrors the rule the
// autosave already follows (see the PATCH branch of `persist`): server data is not
// merged over local state, because a reply that lands after the user moved on would
// clobber whatever they typed since. A live update from another session is the same
// hazard, only more likely.
function isProtected(row: IssuanceRow, editingRowKey: string | null): boolean {
  return (
    row.isDraft ||
    row.saveStatus === "saving" ||
    row.hasError ||
    row.key === editingRowKey
  )
}

// Folds a freshly fetched list into the rows currently on screen. `fresh` is the
// authoritative answer for the current filter - the server owns the filter rules, so a
// row missing from it genuinely does not belong on this screen anymore.
export function reconcileRows(
  prev: IssuanceRow[],
  fresh: Issuance[],
  editingRowKey: string | null,
): IssuanceRow[] {
  const previousByKey = new Map(prev.map((row) => [row.key, row]))
  const freshIds = new Set(fresh.map((issuance) => issuance.id))

  // issuanceToRow keys rows by the backend id, so a persisted row keeps its key across
  // reconciles and React never remounts it (which would drop the cell selection).
  // Iterating `fresh` also adopts the backend's created_at desc ordering for free.
  const reconciled = fresh.map((issuance) => {
    const existing = previousByKey.get(issuance.id)
    if (!existing) return issuanceToRow(issuance)
    if (isProtected(existing, editingRowKey)) return existing

    // saveStatus and hasError are UI-only; rebuilding them from the server would cut the
    // 900ms save flash short on a row that just saved.
    return {
      ...issuanceToRow(issuance),
      saveStatus: existing.saveStatus,
      hasError: existing.hasError,
    }
  })

  // Protected rows the refreshed list no longer contains - an edit may have pushed one
  // out of the current filter while the user is still in it. Dropping it would yank the
  // row out from under the cursor, so it stays until the next full page load.
  const orphans = prev.filter(
    (row) => !row.isDraft && !freshIds.has(row.key) && isProtected(row, editingRowKey),
  )

  // Drafts are always the tail: they have no id to match on, and the grid appends new
  // ones at the bottom.
  const drafts = prev.filter((row) => row.isDraft)

  return [...reconciled, ...orphans, ...drafts]
}
