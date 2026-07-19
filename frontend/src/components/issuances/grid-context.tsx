"use client"

import { createContext, useContext } from "react"
import { IssuanceRow } from "@/components/issuances/types"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { Airline } from "@/schemas/airline"
import { Account } from "@/schemas/account"
import { Agency } from "@/schemas/agency"
import { User } from "@/schemas/user"
import { Person } from "@/schemas/person"
import { Direction } from "@/lib/grid-navigation"

export interface GridReferenceData {
  loyaltyPrograms: LoyaltyProgram[]
  airlines: Airline[]
  accounts: Account[]
  agencies: Agency[]
  users: User[]
  people: Person[]
}

export interface GridContextValue extends GridReferenceData {
  rows: IssuanceRow[]
  sel: { r: number; c: number }
  editing: boolean
  sticky: boolean
  pendingChar: string | null
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
  // Creates a Person, an Account on the row's loyalty program and a nickname, all from
  // a name typed into the account cell that matched nothing, then points the row at the
  // new account. Rows are addressed by key, not index: the creation is async and a row
  // can move (or be deleted) while it is in flight.
  createAccountForRow: (rowKey: string, name: string) => void
  // Names typed into the account cell whose account is still being created, by row key.
  // The cell renders it greyed so the value the operator just typed doesn't blink back
  // to an em dash while the three requests run.
  pendingAccountLabels: Record<string, string>
}

const GridContext = createContext<GridContextValue | null>(null)

export const GridProvider = GridContext.Provider

export function useGridContext(): GridContextValue {
  const ctx = useContext(GridContext)
  if (!ctx) throw new Error("useGridContext must be used within GridProvider")
  return ctx
}
