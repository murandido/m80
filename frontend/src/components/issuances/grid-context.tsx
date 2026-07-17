"use client"

import { createContext, useContext } from "react"
import { IssuanceRow } from "@/components/issuances/types"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { Airline } from "@/schemas/airline"
import { Account } from "@/schemas/account"
import { Agency } from "@/schemas/agency"
import { User } from "@/schemas/user"
import { Direction } from "@/lib/grid-navigation"

export interface GridReferenceData {
  loyaltyPrograms: LoyaltyProgram[]
  airlines: Airline[]
  accounts: Account[]
  agencies: Agency[]
  users: User[]
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
}

const GridContext = createContext<GridContextValue | null>(null)

export const GridProvider = GridContext.Provider

export function useGridContext(): GridContextValue {
  const ctx = useContext(GridContext)
  if (!ctx) throw new Error("useGridContext must be used within GridProvider")
  return ctx
}
