import { Issuance, IssuanceStatus } from "@/schemas/issuance"

export type SaveStatus = "idle" | "saving" | "success" | "error"

// Client-side row shape the grid edits. A row with id === null is a draft that
// only lives in React state until the required fields for POST /issuances are
// filled - see issuances-grid.tsx.
export interface IssuanceRow {
  // Stable client-side identity, distinct from the backend id (which a draft
  // row doesn't have yet) - used to address a row across async save calls
  // without relying on its array index, which can shift.
  key: string
  id: string | null
  isDraft: boolean
  loyalty_program_id: string | null
  airline_id: string | null
  program_locator: string
  airline_locator: string | null
  flight_date: string | null // ISO YYYY-MM-DD
  main_passenger_name: string | null
  origin_iata_code: string | null
  destination_iata_code: string | null
  points: number | null
  value_of_the_thousand: number | null // centavos
  taxes: number | null // centavos
  account_id: string | null
  // Which of the account's aliases was picked. Null for accounts with no registered
  // alias, which are still selectable - see grid-cell.tsx.
  account_nickname_id: string | null
  issuer_id: string | null
  agency_id: string | null
  status: IssuanceStatus
  saveStatus: SaveStatus
}

export function issuanceToRow(issuance: Issuance): IssuanceRow {
  const outboundFlight = issuance.flights?.[0]
  return {
    key: issuance.id,
    id: issuance.id,
    isDraft: false,
    loyalty_program_id: issuance.loyalty_program_id,
    airline_id: issuance.airline_id,
    program_locator: issuance.program_locator,
    airline_locator: issuance.airline_locator,
    flight_date: outboundFlight ? outboundFlight.departure_at.slice(0, 10) : null,
    main_passenger_name: issuance.main_passenger_name,
    origin_iata_code: issuance.origin_iata_code,
    destination_iata_code: issuance.destination_iata_code,
    points: issuance.points,
    value_of_the_thousand: issuance.value_of_the_thousand,
    taxes: issuance.taxes,
    account_id: issuance.account_id,
    account_nickname_id: issuance.account_nickname_id,
    issuer_id: issuance.issuer_id,
    agency_id: issuance.agency_id,
    status: issuance.status,
    saveStatus: "idle",
  }
}

// issuerId defaults a new draft row's "Emissor" to the logged-in user - still
// freely editable before the row is first created (see issuances-grid.tsx).
export function makeDraftRow(issuerId?: string | null): IssuanceRow {
  return {
    key: `draft-${Math.random().toString(36).slice(2)}`,
    id: null,
    isDraft: true,
    loyalty_program_id: null,
    airline_id: null,
    program_locator: "",
    airline_locator: null,
    flight_date: null,
    main_passenger_name: null,
    origin_iata_code: null,
    destination_iata_code: null,
    points: null,
    value_of_the_thousand: null,
    taxes: null,
    account_id: null,
    account_nickname_id: null,
    issuer_id: issuerId ?? null,
    agency_id: null,
    status: "incomplete",
    saveStatus: "idle",
  }
}

// The 5 fields CreateIssuanceRequest requires up front - a draft row only
// becomes a real POSTed issuance once all of these are filled.
export function draftIsReadyToCreate(row: IssuanceRow): boolean {
  return (
    row.program_locator.trim() !== "" &&
    row.loyalty_program_id !== null &&
    row.account_id !== null &&
    row.points !== null &&
    row.points > 0 &&
    row.value_of_the_thousand !== null &&
    row.value_of_the_thousand > 0
  )
}
