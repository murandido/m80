import { IssuanceStatus } from "@/schemas/issuance"

export interface NavigableRow {
  status: IssuanceStatus
}

export type Direction = "down" | "up" | "tab-right" | "tab-left"

// "Cia aérea", "Localizador", "Data do voo", "Passageiro principal", "Origem",
// "Destino", "Pontos", "Valor milheiro", "Taxas", "Valor total" (calculated),
// "Conta", "Emissor", "Agência", "Status", "Observação", "Taxa de resgate".
export const COLUMN_COUNT = 16
// "Data da emissão" sits after all of them, and is the one column the operator can hide.
// Hiding it is passing COLUMN_COUNT where the resolvers below take a count - which only
// works because it is last: no other index moves.
export const ISSUED_ON_COLUMN_INDEX = 16
export const COLUMN_COUNT_WITH_ISSUED_ON = 17
export const LOCATOR_COLUMN_INDEX = 1
export const PASSENGER_COLUMN_INDEX = 3
export const ORIGIN_COLUMN_INDEX = 4
export const POINTS_COLUMN_INDEX = 6
export const VALUE_OF_THE_THOUSAND_COLUMN_INDEX = 7
// The boarding taxes: the one cell a cancelled issuance still lets the operator correct.
export const TAXES_COLUMN_INDEX = 8
export const CALCULATED_COLUMN_INDEX = 9
export const ACCOUNT_COLUMN_INDEX = 10
export const OBSERVATION_COLUMN_INDEX = 14
export const REDEMPTION_FEE_COLUMN_INDEX = 15

function isSkippableRow(row: NavigableRow): boolean {
  return row.status === "cancelled"
}

export interface ResolvedCell {
  r: number
  c: number
  // true when r === rows.length: the caller must create a new draft row before
  // selecting it (this resolver stays pure and never mutates `rows` itself).
  appendRow: boolean
}

// Shared "next valid cell" resolver used by Enter, arrows-while-editing, and
// Tab/Shift+Tab (in any state). Recurses over cancelled rows and steps around
// the calculated column so callers never have to special-case either skip.
export function resolveNextCell<T extends NavigableRow>(
  rows: T[],
  sel: { r: number; c: number },
  direction: Direction,
  depth = 0,
  columnCount: number = COLUMN_COUNT,
): ResolvedCell {
  // Guards against infinite recursion if every row happens to be cancelled.
  if (depth > rows.length + columnCount + 5) {
    return { r: sel.r, c: sel.c, appendRow: false }
  }

  if (direction === "down" || direction === "up") {
    const step = direction === "down" ? 1 : -1
    const r = sel.r + step

    if (r < 0) return { r: 0, c: sel.c, appendRow: false }
    if (r >= rows.length) {
      if (direction === "down") return { r: rows.length, c: sel.c, appendRow: true }
      return { r: rows.length - 1, c: sel.c, appendRow: false }
    }
    if (isSkippableRow(rows[r])) {
      return resolveNextCell(rows, { r, c: sel.c }, direction, depth + 1, columnCount)
    }
    return { r, c: sel.c, appendRow: false }
  }

  // tab-right / tab-left
  const step = direction === "tab-right" ? 1 : -1
  let r = sel.r
  let c = sel.c + step
  let crossedIntoNewRow = false

  if (c === CALCULATED_COLUMN_INDEX) c += step

  if (c < 0) {
    r -= 1
    c = columnCount - 1
    if (c === CALCULATED_COLUMN_INDEX) c -= 1
    crossedIntoNewRow = true
  } else if (c >= columnCount) {
    r += 1
    c = 0
    if (c === CALCULATED_COLUMN_INDEX) c += 1
    crossedIntoNewRow = true
  }

  if (r < 0) return { r: 0, c: 0, appendRow: false }
  if (r >= rows.length) {
    if (direction === "tab-right") return { r: rows.length, c: 0, appendRow: true }
    return { r: rows.length - 1, c: columnCount - 1, appendRow: false }
  }
  // Only treat landing on a cancelled row as "skippable" when this step
  // actually crossed a row boundary - Tab/Shift+Tab moving column-to-column
  // within a row the user clicked into directly (allowed, for inspection)
  // must never eject them to a different row.
  if (crossedIntoNewRow && isSkippableRow(rows[r])) {
    // Skip the whole cancelled row in one jump - recurse from its far edge
    // so the next step crosses into another row again, instead of stepping
    // column-by-column through cells that must never be landed on.
    const jumpSel = direction === "tab-right" ? { r, c: columnCount - 1 } : { r, c: 0 }
    return resolveNextCell(rows, jumpSel, direction, depth + 1, columnCount)
  }
  return { r, c, appendRow: false }
}

// Plain arrow-key navigation outside edit mode: a trivial clamp, deliberately
// NOT skip-aware (neither cancelled rows nor the calculated column) so the
// "skip only applies to Enter/arrows-while-editing/Tab" rule can't regress by
// sharing code with resolveNextCell.
export function clampMove(
  rowCount: number,
  sel: { r: number; c: number },
  dr: number,
  dc: number,
  columnCount: number = COLUMN_COUNT,
): { r: number; c: number } {
  const r = Math.min(Math.max(sel.r + dr, 0), Math.max(rowCount - 1, 0))
  const c = Math.min(Math.max(sel.c + dc, 0), columnCount - 1)
  return { r, c }
}
