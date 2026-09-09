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
  // The airline's own order number, which some airlines (LATAM) key their booking on
  // instead of the locator. Edited through the Localizador cell rather than a column
  // of its own - see locator-cell.tsx.
  latam_purchase_number: string | null
  flight_date: string | null // ISO YYYY-MM-DD
  // The day of the issuance, ISO YYYY-MM-DD. Never null: an issuance always happened on
  // some day, and a draft carries the one the toolbar is recording on.
  issued_on: string
  main_passenger_name: string | null
  origin_iata_code: string | null
  destination_iata_code: string | null
  points: number | null
  value_of_the_thousand: number | null // centavos
  taxes: number | null // centavos
  // The airline's redemption fee this issuance did not pay, because the account it was made
  // on is exempt. Centavos. Null on an ordinary account and whenever the rule could not be
  // applied; zero means it ran and there was no fee to avoid. Does not take part in the
  // row's "Valor total".
  redemption_fee_saved_cents: number | null
  // An operator typed the amount by hand, which stops the next Traveos sync recomputing
  // over the correction. Read-only here: it is set by the server from the patch itself.
  redemption_fee_manual: boolean
  account_id: string | null
  // Which of the account's aliases was picked. Null for accounts with no registered
  // alias, which are still selectable - see grid-cell.tsx.
  account_nickname_id: string | null
  issuer_id: string | null
  agency_id: string | null
  status: IssuanceStatus
  // Free text on the issuance. The bot writes it by resolving the fifth line of an
  // EMISSAO against observation_presets; the grid edits the resolved text, not the
  // code, because the code is a shorthand for typing, not what the issuance carries.
  observation: string | null
  // When the itinerary was last pulled from Traveos. Null means it never was, which
  // is not the same as incomplete: a row can be marked complete by hand and still
  // have no Traveos data, and only rows that have it can render a ticket.
  traveos_synced_at: string | null
  // Set while a pull from Traveos is in flight, by whoever started it - another
  // operator, or the bot. It comes from the row rather than from this browser, which
  // is what makes the lock and the spinner survive leaving the screen and show up
  // for everybody else.
  traveos_syncing_at: string | null
  // When the automatic Traveos lookup last ran on this row, whether or not it found
  // the booking. Persisted rather than kept in this browser: for an imported day the
  // in-memory "one attempt per page load" is one attempt per refresh, times hundreds
  // of rows. See isAutocompleteCandidate.
  traveos_attempted_at: string | null
  // Came from the pre-panel spreadsheet (backend internal/legacyimport). Read-only.
  imported: boolean
  // Read-only cancellation metadata, surfaced in the status pill's hover card. Never
  // edited through the grid - only set when the row loads cancelled or is cancelled here.
  cancelled_at: string | null
  cancelled_by_id: string | null
  refunded_points: number | null
  refunded_taxes: number | null // centavos
  cancellation_fee: number | null // centavos
  // The row stands for an estorno alone: the booking was made outside the panel, so
  // the supplier balance counts the refund and not the points. Set at cancellation.
  cancellation_only: boolean
  saveStatus: SaveStatus
  // Sticks until the row next saves successfully, unlike saveStatus which flashes
  // for 900ms and resets. Drives the persistent red row highlight so a failed save
  // isn't lost the moment the toast and the dot fade.
  hasError: boolean
}

// Whether a Traveos pull is in flight for this row. Two sources, and both are needed:
// the local map covers the gap between the click and the first refresh from the
// server, and the row's own marker is what makes the lock outlive this browser -
// another operator's pull, or one that started before this screen was opened.
export function isRowSyncing(
  row: IssuanceRow,
  syncingRowKeys: Record<string, true>,
): boolean {
  return Boolean(syncingRowKeys[row.key]) || Boolean(row.traveos_syncing_at)
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
    latam_purchase_number: issuance.latam_purchase_number,
    flight_date: outboundFlight ? outboundFlight.departure_at.slice(0, 10) : null,
    issued_on: issuance.issued_on,
    main_passenger_name: issuance.main_passenger_name,
    origin_iata_code: issuance.origin_iata_code,
    destination_iata_code: issuance.destination_iata_code,
    points: issuance.points,
    value_of_the_thousand: issuance.value_of_the_thousand,
    taxes: issuance.taxes,
    redemption_fee_saved_cents: issuance.redemption_fee_saved_cents ?? null,
    redemption_fee_manual: issuance.redemption_fee_manual,
    account_id: issuance.account_id,
    account_nickname_id: issuance.account_nickname_id,
    issuer_id: issuance.issuer_id,
    agency_id: issuance.agency_id,
    status: issuance.status,
    observation: issuance.observation,
    traveos_synced_at: issuance.traveos_synced_at ?? null,
    traveos_syncing_at: issuance.traveos_syncing_at ?? null,
    traveos_attempted_at: issuance.traveos_attempted_at ?? null,
    imported: issuance.imported,
    cancelled_at: issuance.cancelled_at,
    cancelled_by_id: issuance.cancelled_by_id,
    refunded_points: issuance.refunded_points,
    refunded_taxes: issuance.refunded_taxes,
    cancellation_fee: issuance.cancellation_fee,
    cancellation_only: issuance.cancellation_only,
    saveStatus: "idle",
    hasError: false,
  }
}

// issuerId defaults a new draft row's "Emissor" to the logged-in user, and agencyId its
// "Agência" to the configured default agency - both still freely editable before the row is
// first created (see issuances-grid.tsx).
export function makeDraftRow(
  issuerId?: string | null,
  agencyId?: string | null,
  issuedOn?: string,
): IssuanceRow {
  return {
    key: `draft-${Math.random().toString(36).slice(2)}`,
    id: null,
    isDraft: true,
    loyalty_program_id: null,
    airline_id: null,
    program_locator: "",
    airline_locator: null,
    latam_purchase_number: null,
    flight_date: null,
    issued_on: issuedOn ?? new Date().toISOString().slice(0, 10),
    main_passenger_name: null,
    origin_iata_code: null,
    destination_iata_code: null,
    points: null,
    value_of_the_thousand: null,
    taxes: null,
    // A draft has no itinerary, so there is nothing to compute yet - the amount arrives from
    // the server once the row exists and the Traveos sync has run.
    redemption_fee_saved_cents: null,
    redemption_fee_manual: false,
    account_id: null,
    account_nickname_id: null,
    issuer_id: issuerId ?? null,
    agency_id: agencyId ?? null,
    status: "incomplete",
    observation: null,
    traveos_synced_at: null,
    traveos_syncing_at: null,
    traveos_attempted_at: null,
    // A row typed into the grid is never imported - the flag only ever arrives from
    // the server, on rows the spreadsheet importer wrote.
    imported: false,
    cancelled_at: null,
    cancelled_by_id: null,
    refunded_points: null,
    refunded_taxes: null,
    cancellation_fee: null,
    cancellation_only: false,
    saveStatus: "idle",
    hasError: false,
  }
}

// The 5 fields CreateIssuanceRequest requires up front - a draft row only
// becomes a real POSTed issuance once all of these are filled.
//
// autocompleteHintFor (autocomplete.ts) names three of them to the operator, because the
// Traveos autocomplete only ever looks at saved rows. Adding a field here means adding its
// explanation there too.
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
