import { IssuanceStatus } from "@/schemas/issuance"

export interface NavigableRow {
  status: IssuanceStatus
}

export type Direction = "down" | "up" | "tab-right" | "tab-left"

// "Cia aérea", "Localizador", "Data do voo", "Passageiro principal", "Origem",
// "Destino", "Pontos", "Valor milheiro", "Taxas", "Valor total" (calculated),
// "Conta", "Emissor", "Agência", "Status", "Observação", "Taxa de resgate".
export const COLUMN_COUNT = 16
// "Data da emissão" is numbered after all of them, and is the one column the operator can
// hide - see columnOrder for where it is shown.
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

// The order the columns are shown in, as column indices. Kept apart from the indices
// themselves so a column can move on screen without renumbering every cell, clear patch
// and copy rule keyed on them. Status is shown last; "Data da emissão", when visible,
// sits just before it.
export const STATUS_COLUMN_INDEX = 13

export function columnOrder(issuedOnVisible: boolean): number[] {
  const order = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, OBSERVATION_COLUMN_INDEX, REDEMPTION_FEE_COLUMN_INDEX]
  if (issuedOnVisible) order.push(ISSUED_ON_COLUMN_INDEX)
  order.push(STATUS_COLUMN_INDEX)
  return order
}

// Shared "next valid cell" resolver used by Enter, arrows-while-editing, and
// Tab/Shift+Tab (in any state). Recurses over cancelled rows and steps around
// the calculated column so callers never have to special-case either skip.
//
// sel.c is a column index; moving left and right walks `order`, the on-screen order.
export function resolveNextCell<T extends NavigableRow>(
  rows: T[],
  sel: { r: number; c: number },
  direction: Direction,
  order: readonly number[],
  depth = 0,
): ResolvedCell {
  const last = order.length - 1
  // Guards against infinite recursion if every row happens to be cancelled.
  if (depth > rows.length + order.length + 5) {
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
      return resolveNextCell(rows, { r, c: sel.c }, direction, order, depth + 1)
    }
    return { r, c: sel.c, appendRow: false }
  }

  // tab-right / tab-left
  const step = direction === "tab-right" ? 1 : -1
  let r = sel.r
  let position = Math.max(order.indexOf(sel.c), 0) + step
  let crossedIntoNewRow = false

  if (order[position] === CALCULATED_COLUMN_INDEX) position += step

  if (position < 0) {
    r -= 1
    position = last
    if (order[position] === CALCULATED_COLUMN_INDEX) position -= 1
    crossedIntoNewRow = true
  } else if (position > last) {
    r += 1
    position = 0
    if (order[position] === CALCULATED_COLUMN_INDEX) position += 1
    crossedIntoNewRow = true
  }

  if (r < 0) return { r: 0, c: order[0], appendRow: false }
  if (r >= rows.length) {
    if (direction === "tab-right") return { r: rows.length, c: order[0], appendRow: true }
    return { r: rows.length - 1, c: order[last], appendRow: false }
  }
  // Only treat landing on a cancelled row as "skippable" when this step
  // actually crossed a row boundary - Tab/Shift+Tab moving column-to-column
  // within a row the user clicked into directly (allowed, for inspection)
  // must never eject them to a different row.
  if (crossedIntoNewRow && isSkippableRow(rows[r])) {
    // Skip the whole cancelled row in one jump - recurse from its far edge
    // so the next step crosses into another row again, instead of stepping
    // column-by-column through cells that must never be landed on.
    const jumpSel = direction === "tab-right" ? { r, c: order[last] } : { r, c: order[0] }
    return resolveNextCell(rows, jumpSel, direction, order, depth + 1)
  }
  return { r, c: order[position], appendRow: false }
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
  order: readonly number[],
): { r: number; c: number } {
  const r = Math.min(Math.max(sel.r + dr, 0), Math.max(rowCount - 1, 0))
  const position = Math.min(Math.max(Math.max(order.indexOf(sel.c), 0) + dc, 0), order.length - 1)
  return { r, c: order[position] }
}
