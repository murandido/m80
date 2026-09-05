"use client"

import { createContext, useContext } from "react"
import { IssuanceRow } from "@/components/issuances/types"
import { CancelIssuanceInput } from "@/schemas/issuance"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { Airline } from "@/schemas/airline"
import { AccountOption } from "@/schemas/account"
import { Agency } from "@/schemas/agency"
import { User } from "@/schemas/user"
import { PersonOption } from "@/schemas/person"
import { Airport } from "@/schemas/airport"
import { Direction } from "@/lib/grid-navigation"

export interface GridReferenceData {
  loyaltyPrograms: LoyaltyProgram[]
  airlines: Airline[]
  accounts: AccountOption[]
  agencies: Agency[]
  users: User[]
  people: PersonOption[]
}

// Volatile per-cell state passed down as props (not through the context) so that moving
// the selection or editing one cell doesn't invalidate the context for every other cell.
// The context below holds only referentially-stable data (reference lists + memoized
// callbacks + pendingAccountLabels), which is what lets React.memo skip untouched cells.
export interface CellStateProps {
  row: IssuanceRow
  isEditing: boolean
  // Carries the replace-on-type / paste seed, and only for the cell that is editing;
  // every other cell receives null, so this prop stays stable for them.
  pendingChar: string | null
}

export interface GridContextValue extends GridReferenceData {
  // Not part of GridReferenceData because it does not arrive with the page: the grid
  // loads the airport table from the browser (see hooks/use-airports) and puts it here,
  // so the cells still read it the same way every other reference list is read.
  airports: Airport[]
  onCellClick: (r: number, c: number) => void
  // patch: the changed fields for row r, in backend field-name shape (e.g.
  // {program_locator}, {loyalty_program_id, airline_id}, {status}, {flight_date}).
  // direction null means "just save, don't navigate" (blur-only case).
  onSaveAndMove: (r: number, patch: Partial<IssuanceRow>, direction: Direction | null) => void
  onExitEdit: () => void
  // The currently-editing cell registers a save callback here so the grid can
  // force an explicit save before switching cells, per the spec's requirement
  // that cell-switch saves don't rely solely on native blur.
  registerActiveCellSave: (save: (() => void) | null) => void
  requestSaveActiveCell: () => void
  onDeleteRow: (r: number) => void
  // Cancels the issuance at row r (POST /issuances/{id}/cancel) with optional refund
  // amounts, then flips the row to "cancelled" in local state. Returns the action result
  // so the cancel dialog can toast + stay open on error, close on success.
  onCancelRow: (
    r: number,
    data: CancelIssuanceInput,
  ) => Promise<{ success: true } | { error: string }>
  // Reverts the cancellation at row r (POST /issuances/{id}/uncancel), flipping it back to
  // its derived status (complete/incomplete) and clearing the cancellation metadata locally.
  onUncancelRow: (r: number) => Promise<{ success: true } | { error: string }>
  // Pulls the itinerary from Traveos for row r (POST /issuances/{id}/traveos-sync),
  // issuing the ticket there first if it does not exist yet. Available on any saved,
  // non-cancelled row - including one already synced, because that is how a changed
  // flight gets picked up.
  onSyncRowWithTraveos: (r: number) => Promise<{ success: true } | { error: string }>
  // Row keys with a Traveos sync in flight. The call can take minutes, so the row is
  // locked and marked while it runs rather than left looking idle.
  syncingRowKeys: Record<string, true>

  // Turns what was typed into the account cell into an account the row can point at. Plain text
  // creates the Person/Account/nickname trio on the row's loyalty program; "conta - administrador"
  // also links the administrator, reusing the account when that alias already exists. Rows are
  // addressed by key, not index: this is async and a row can move (or be deleted) meanwhile.
  resolveAccountEntryForRow: (rowKey: string, typed: string) => void
  // Text typed into the account cell whose account is still being resolved, by row key.
  // The cell renders it greyed so the value the operator just typed doesn't blink back
  // to an em dash while the requests run.
  pendingAccountLabels: Record<string, string>
}

const GridContext = createContext<GridContextValue | null>(null)

export const GridProvider = GridContext.Provider

export function useGridContext(): GridContextValue {
  const ctx = useContext(GridContext)
  if (!ctx) throw new Error("useGridContext must be used within GridProvider")
  return ctx
}
